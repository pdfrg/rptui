// Package tui provides the terminal user interface
package tui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/blacktop/go-termimg"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/cache"
	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/models"
	"rptui-bubbletea/internal/mpv"
	"rptui-bubbletea/internal/tui/modals"
	"rptui-bubbletea/internal/tui/widgets"
)

// Logger for TUI
var logger *log.Logger

func init() {
	f, err := os.OpenFile("rptui-go.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logger = log.New(f, "[TUI] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "[TUI] ", log.LstdFlags|log.Lshortfile)
	}
}

// Bottom view mode constants
const (
	ViewPlaylist = iota
	ViewLyrics
	ViewSyncedLyrics
	ViewArtist
	ViewOff
	ViewModeCount
)

var bottomViewNames = []string{
	"Playlist",
	"Lyrics",
	"Synced Lyrics",
	"Artist",
	"Off",
}

// Modal types
const (
	ModalNone = iota
	ModalOptions
	ModalSkipWarning
	ModalFavorites
)

// Model represents the main TUI application model
type Model struct {
	// Configuration
	config *config.Config
	theme  *config.ColorTheme
	styles *config.ThemeStyles

	// API Clients
	rpAPI           *api.RadioParadiseAPI
	lyricsClient    *api.LRCLibClient
	wikipediaClient *api.WikipediaClient
	mpvBackend      *mpv.MPVBackend
	cacheManager    *cache.CacheManager

	// State
	songs            []*models.Song
	currentSongIndex int
	playlistStartIdx int
	isPlaying        bool
	isPaused         bool
	bottomViewMode   int
	imageBase        string
	favoriteCount    int  // cached count, updated on song change
	imageCounter     int  // for unique image IDs
	skipWarningShown bool // track if skip warning has been shown this session

	// Current song info
	currentSong *models.Song

	// Playback position (cached from MPV, updated every tick - avoid IPC in View)
	playbackPos mpv.PlaybackPosition

	// Connected time (set once when playback starts)
	connectedAt time.Time

	// Next block polling
	pollingNextBlock bool

	// Auto-favorite playback
	favoritesQueue       []cache.CachedSong
	lastFavoriteQueuedAt time.Time

	// Bottom view content
	lyrics       string
	syncedLyrics []api.SyncedLyric
	artistInfo   *api.ArtistInfo

	// Bubbles components
	viewport    viewport.Model
	albumArtStr string // cached rendered escape sequence

	// Cached album art render string (only re-render when image changes)
	albumArtLoaded bool

	// Custom Widgets
	headerWidget     *widgets.Header
	footerWidget     *widgets.Footer
	nowPlayingWidget *widgets.NowPlaying
	playlistWidget   *widgets.Playlist

	// Modal Widgets
	optionsModal     *modals.Options
	skipWarningModal *modals.SkipWarning
	favoritesModal   *modals.Favorites

	// UI dimensions
	width  int
	height int

	// Status
	statusMsg string

	// Error state
	err error

	// Modal state
	activeModal int

	// Initialization complete
	initialized bool

	// Light/dark mode
	isDark bool

	// Help
	help help.Model

	// Terminal cell ratio for album art aspect ratio correction
	cellRatio float64
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config, theme *config.ColorTheme) *Model {
	styles := config.NewThemeStyles(theme)

	// Initialize terminal cell ratio for album art
	features := termimg.QueryTerminalFeatures()
	cellRatio := float64(features.FontHeight) / float64(features.FontWidth)
	if cellRatio <= 0 {
		cellRatio = 2.0 // fallback (typical terminal: ~2x taller than wide)
	}

	// Initialize API clients
	rpAPI := api.NewRadioParadiseAPI(cfg.Channel, cfg.Bitrate)
	lyricsClient := api.NewLRCLibClient()
	wikipediaClient := api.NewWikipediaClient()
	mpvBackend := mpv.NewMPVBackend()
	cacheManager := cache.NewCacheManager(
		cfg.GetFavoritesDir(),
		cfg.GetBlocklistDir(),
		cfg.MaxFavorites,
	)
	if err := cacheManager.EnsureDirectories(); err != nil {
		logger.Printf("Warning: failed to create cache directories: %v", err)
	}

	// Initialize custom widgets
	headerWidget := widgets.NewHeader(styles.Header, "rptui - Radio Paradise")
	footerWidget := widgets.NewFooter(styles.AccentStyle, styles.MutedStyle)
	nowPlayingWidget := widgets.NewNowPlaying(styles.ForegroundStyle, styles.AccentStyle, styles.MutedStyle, theme.Accent, theme.Cursor, theme.Background)
	playlistWidget := widgets.NewPlaylist(styles)

	// Initialize modal widgets
	optionsModal := modals.NewOptions(styles, cfg.Channel, cfg.Bitrate)
	skipWarningModal := modals.NewSkipWarning(styles, cfg.MinFavorites)

	// Initialize viewport for bottom views
	viewport := viewport.New(
		viewport.WithWidth(100),
		viewport.WithHeight(15),
	)

	// Initialize help
	help := help.New()

	return &Model{
		config:           cfg,
		theme:            theme,
		styles:           styles,
		rpAPI:            rpAPI,
		lyricsClient:     lyricsClient,
		wikipediaClient:  wikipediaClient,
		mpvBackend:       mpvBackend,
		cacheManager:     cacheManager,
		bottomViewMode:   ViewPlaylist,
		headerWidget:     headerWidget,
		footerWidget:     footerWidget,
		nowPlayingWidget: nowPlayingWidget,
		playlistWidget:   playlistWidget,
		optionsModal:     optionsModal,
		skipWarningModal: skipWarningModal,
		viewport:         viewport,
		help:             help,
		cellRatio:        cellRatio,
	}
}

// Init initializes the TUI model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchBlockCmd,
		tickProgressCmd(),
		tickPollCmd(),
		tea.RequestBackgroundColor,
	)
}

// fetchBlockCmd fetches the initial block from the API
func (m Model) fetchBlockCmd() tea.Msg {
	block, err := m.rpAPI.GetBlock(context.Background())
	if err != nil {
		return blockFetchedMsg{err: fmt.Errorf("GetBlock error: %w", err)}
	}

	// Parse songs
	songs, imageBase := m.rpAPI.ParseBlockSongs(block)
	if len(songs) == 0 {
		return blockFetchedMsg{err: fmt.Errorf("no songs in block")}
	}

	// Build playlist URLs
	urls := make([]string, len(songs))
	for i, song := range songs {
		urls[i] = song.GaplessURL
	}

	// Start MPV
	if err := m.mpvBackend.Start(urls); err != nil {
		return blockFetchedMsg{songs: songs, imageBase: imageBase, err: fmt.Errorf("MPV error: %w", err)}
	}

	return blockFetchedMsg{
		songs:     songs,
		imageBase: imageBase,
		err:       nil,
	}
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Always update animations
	if cmd := m.nowPlayingWidget.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Always update playlist (handles table scrolling)
	if cmd := m.playlistWidget.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Local helper to handle return values
	handle := func(newModel tea.Model, newCmd tea.Cmd) (tea.Model, tea.Cmd) {
		if newCmd != nil {
			cmds = append(cmds, newCmd)
		}
		return newModel, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		return handle(m, nil)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update component sizes
		m.headerWidget.SetWidth(msg.Width)
		m.footerWidget.SetWidth(msg.Width)
		m.nowPlayingWidget.SetWidth(40)

		contentWidth := max(20, msg.Width-4)
		contentHeight := max(5, msg.Height-14)

		m.viewport.SetWidth(contentWidth)
		m.viewport.SetHeight(contentHeight)
		m.playlistWidget.SetSize(contentWidth, contentHeight)

		return handle(m, nil)

	case tea.KeyPressMsg:
		if m.activeModal != ModalNone {
			var cmd tea.Cmd
			switch m.activeModal {
			case ModalOptions:
				cmd = m.optionsModal.Update(msg)
			case ModalSkipWarning:
				cmd = m.skipWarningModal.Update(msg)
			case ModalFavorites:
				if m.favoritesModal != nil {
					cmd = m.favoritesModal.Update(msg)
				}
			}
			return handle(m, cmd)
		}
		newModel, cmd := m.handleKeyPress(msg)
		return handle(newModel, cmd)

	case modals.OptionsMsg:
		m.activeModal = ModalNone
		if msg.Station != nil {
			m.config.Channel = *msg.Station
			m.config.Save()
			m.rpAPI.SetChannel(*msg.Station)
			return handle(m, m.fetchBlockCmd)
		}
		if msg.Bitrate != nil {
			m.config.Bitrate = *msg.Bitrate
			m.config.Save()
			m.rpAPI.SetBitrate(*msg.Bitrate)
			return handle(m, m.fetchBlockCmd)
		}
		return handle(m, nil)

	case modals.SkipWarningMsg:
		m.activeModal = ModalNone
		if msg.Confirmed {
			if m.currentSongIndex < len(m.songs)-1 {
				if err := m.mpvBackend.SkipNext(); err == nil {
					m.currentSongIndex++
					return handle(m, m.songChangedCmds())
				}
			}
		}
		return handle(m, nil)

	case modals.FavoritesMsg:
		m.activeModal = ModalNone
		if msg.PlayEventID != nil {
			fav, err := m.cacheManager.GetFavoriteByEventID(*msg.PlayEventID)
			if err == nil && fav != nil {
				song := fav.ToSong()
				m.songs = []*models.Song{song}
				m.currentSongIndex = 0
				m.playlistStartIdx = 0

				if err := m.mpvBackend.Start([]string{song.GaplessURL}); err == nil {
					m.statusMsg = "Playing from favorites: " + song.Title
					return handle(m, m.songChangedCmds())
				}
			}
		}
		return handle(m, nil)

	case blockFetchedMsg:
		return handle(m.handleBlockFetched(msg))

	case progressTickMsg:
		return handle(m.handleProgressTick(msg))

	case pollTickMsg:
		return handle(m.handlePollTick(msg))

	case imageLoadedMsg:
		return handle(m.handleImageLoaded(msg))

	case lyricsFetchedMsg:
		return handle(m.handleLyricsFetched(msg))

	case artistFetchedMsg:
		return handle(m.handleArtistFetched(msg))
	}

	return m, tea.Batch(cmds...)
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// Cleanup MPV before quitting
		m.mpvBackend.Stop()
		return m, tea.Quit

	case "space":
		// Play/Pause
		if err := m.mpvBackend.TogglePause(); err == nil {
			m.isPaused = !m.isPaused
			if m.isPaused {
				m.statusMsg = "Paused"
			} else {
				m.statusMsg = "Playing"
			}
		}
		return m, nil

	case "n":
		// Skip next - show warning only once per session
		if m.config.ShowSkipWarning && !m.skipWarningShown {
			m.skipWarningShown = true
			m.activeModal = ModalSkipWarning
			return m, clearKittyImagesCmd()
		}
		if m.currentSongIndex < len(m.songs)-1 {
			if err := m.mpvBackend.SkipNext(); err == nil {
				m.currentSongIndex++
				return m, m.songChangedCmds()
			}
		} else {
			m.statusMsg = "No more songs in block"
		}
		return m, nil

	case "p":
		// Previous song
		if m.currentSongIndex > 0 {
			if err := m.mpvBackend.SkipPrev(); err == nil {
				m.currentSongIndex--
				return m, m.songChangedCmds()
			}
		} else {
			// Restart current song if at beginning
			if err := m.mpvBackend.SeekToStart(); err == nil {
				m.statusMsg = "Restarting song"
			}
		}
		return m, nil

	case "r":
		// Restart song
		if err := m.mpvBackend.SeekToStart(); err == nil {
			m.statusMsg = "Restarting song"
		}
		return m, nil

	case "s":
		// Stop
		if err := m.mpvBackend.Stop(); err == nil {
			m.isPlaying = false
			m.isPaused = false
			m.statusMsg = "Stopped"
		}
		return m, nil

	case "v":
		// Toggle bottom view
		m.bottomViewMode = (m.bottomViewMode + 1) % ViewModeCount
		m.statusMsg = fmt.Sprintf("View: %s", bottomViewNames[m.bottomViewMode])
		m.updateBottomView()

		// Fetch lyrics/artist if needed
		if m.bottomViewMode == ViewLyrics || m.bottomViewMode == ViewSyncedLyrics {
			if m.lyrics == "" && m.currentSong != nil {
				return m, m.fetchLyricsCmd()
			}
		} else if m.bottomViewMode == ViewArtist {
			if m.artistInfo == nil && m.currentSong != nil {
				return m, m.fetchArtistCmd()
			}
		}
		return m, nil

	case "f":
		// Toggle favorite
		if m.currentSong != nil {
			wasFavorite := m.cacheManager.IsFavorite(m.currentSong)
			if _, err := m.cacheManager.ToggleFavorite(m.currentSong); err == nil {
				if wasFavorite {
					m.favoriteCount--
					m.statusMsg = "Removed from favorites"
				} else {
					m.favoriteCount++
					m.statusMsg = "Added to favorites"
				}
				m.updatePlaylist()
			}
		}
		return m, nil

	case "b":
		// Toggle blocklist
		if m.currentSong != nil {
			wasBlocked := m.cacheManager.IsBlocked(m.currentSong)
			if _, err := m.cacheManager.ToggleBlocklist(m.currentSong); err == nil {
				if wasBlocked {
					m.statusMsg = "Removed from blocklist"
				} else {
					m.statusMsg = "Added to blocklist"
				}
				m.updatePlaylist()
			}
		}
		return m, nil

	case "o":
		// Options modal
		m.activeModal = ModalOptions
		return m, clearKittyImagesCmd()

	case "m":
		// Manage favorites modal
		m.favoritesModal = modals.NewFavorites(m.styles, m.cacheManager)
		m.activeModal = ModalFavorites
		return m, clearKittyImagesCmd()

	case "$":
		// Open RP donate page
		m.statusMsg = "Opening RP donate page..."
		return m, openDonatePageCmd

	case "up", "k":
		// Scroll up in viewport
		if m.bottomViewMode != ViewPlaylist {
			m.viewport.ScrollUp(1)
		}
		return m, nil

	case "down", "j":
		// Scroll down in viewport
		if m.bottomViewMode != ViewPlaylist {
			m.viewport.ScrollDown(1)
		}
		return m, nil

	case "g":
		// Scroll to top
		if m.bottomViewMode != ViewPlaylist {
			m.viewport.GotoTop()
		}
		return m, nil

	case "G":
		// Scroll to bottom
		if m.bottomViewMode != ViewPlaylist {
			m.viewport.GotoBottom()
		}
		return m, nil
	}

	return m, nil
}

// handleBlockFetched handles the block fetched message
func (m Model) handleBlockFetched(msg blockFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if !m.initialized {
			// Fatal on startup — can't play without a block
			m.err = msg.err
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
			return m, nil
		}
		// Already playing — log the error and keep polling for next block
		logger.Printf("Block fetch error (retrying): %v", msg.err)
		m.statusMsg = "Retrying next block..."
		m.pollingNextBlock = true
		return m, nil
	}

	m.songs = msg.songs
	m.imageBase = msg.imageBase
	m.currentSongIndex = 0
	m.isPlaying = true
	m.pollingNextBlock = false

	// Set connected time on first init
	if !m.initialized {
		m.connectedAt = time.Now()
	}

	m.statusMsg = "Playing"
	m.initialized = true

	// Use centralized song-change helper (sets currentSong, fetches art/lyrics/artist)
	return m, m.songChangedCmds()
}

// handleProgressTick handles progress updates (every 1 second)
func (m Model) handleProgressTick(msg progressTickMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{tickProgressCmd()} // Always re-arm

	if !m.initialized || m.currentSong == nil {
		return m, tea.Batch(cmds...)
	}

	// Get playback position from MPV and store in model (avoid IPC in View)
	pos, err := m.mpvBackend.GetPlaybackPosition()
	if err != nil {
		return m, tea.Batch(cmds...)
	}
	m.playbackPos = pos

	// Update progress bar (0.0 to 1.0)
	if cmd := m.nowPlayingWidget.UpdateProgress(pos.PercentPos / 100.0); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Check for natural song transition (like Python's _detect_song_transition)
	mpvPos, err := m.mpvBackend.GetPlaylistPosition()
	if err == nil && mpvPos >= 0 && mpvPos != m.currentSongIndex {
		m.currentSongIndex = mpvPos
		if m.currentSongIndex >= 0 && m.currentSongIndex < len(m.songs) {
			// Auto-skip blocklisted songs
			if m.cacheManager.IsBlocked(m.songs[m.currentSongIndex]) {
				logger.Printf("Auto-skipping blocklisted: %s", m.songs[m.currentSongIndex].Title)
				if m.currentSongIndex < len(m.songs)-1 {
					m.mpvBackend.SkipNext()
				}
				return m, tea.Batch(cmds...)
			}
			cmds = append(cmds, m.songChangedCmds())

			// If on last song of block, trigger next block polling
			if m.currentSongIndex >= len(m.songs)-1 {
				m.pollingNextBlock = true
			}
		}
	}

	// Check if we should auto-queue a favorite
	if cmd := m.checkAndQueueFavorite(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Update synced lyrics position if in that view
	if m.bottomViewMode == ViewSyncedLyrics && len(m.syncedLyrics) > 0 {
		m.updateBottomView()
	}

	return m, tea.Batch(cmds...)
}

// handlePollTick handles poll timer updates (every 5 seconds)
// Matches Python's poll_wrapper: fetches next block when needed
func (m Model) handlePollTick(msg pollTickMsg) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{tickPollCmd()} // Always re-arm

	if !m.initialized {
		return m, tea.Batch(cmds...)
	}

	// If we're on the last song and need next block, fetch it
	if m.pollingNextBlock {
		cmds = append(cmds, m.fetchBlockCmd)
	}

	return m, tea.Batch(cmds...)
}

// handleImageLoaded handles image loading completion
func (m Model) handleImageLoaded(msg imageLoadedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}
	if msg.err != nil {
		logger.Printf("Image load error: %v", msg.err)
		return m, nil
	}

	// Decode image (jpeg/png decoders registered via blank imports)
	img, format, err := image.Decode(bytes.NewReader(msg.imageData))
	if err != nil {
		logger.Printf("Image decode error: %v", err)
		return m, nil
	}

	logger.Printf("Image decoded: %s, format=%s, bounds=%v, dataLen=%d", m.currentSong.Title, format, img.Bounds(), len(msg.imageData))

	// Copy album art to file if configured
	if m.config.CopyAlbumArt && m.config.AlbumArtPath != "" {
		if err := os.WriteFile(m.config.AlbumArtPath, msg.imageData, 0644); err != nil {
			logger.Printf("Warning: failed to copy album art to %s: %v", m.config.AlbumArtPath, err)
		}
	}

	// Clear go-termimg's global resize cache before rendering. The cache keys
	// on (targetSize, path, srcBounds) — since path is empty for in-memory images,
	// all 500x500 sources map to the same key and return stale pixel data.
	termimg.ClearResizeCache()

	// Calculate album art dimensions based on terminal cell ratio
	// Space is height constrained: album art is to the right of now playing (~9 lines)
	// and above the playlist (~starts at row 11). Available: ~8-9 rows.
	// Use 16 rows to fit (with 2 row gap above playlist), width based on cell ratio.
	// cellRatio = cellHeight / cellWidth (typical ~2.0, e.g., 7x14)
	height := 16
	width := int(float64(height) * m.cellRatio)
	if width < 10 {
		width = 10 // minimum width
	}

	tiImg := termimg.New(img).
		Size(width, height).
		Scale(termimg.ScaleFit).
		Protocol(termimg.Kitty)

	rendered, err := tiImg.Render()
	if err != nil {
		logger.Printf("Album art render error: %v", err)
		return m, nil
	}

	logger.Printf("Album art loaded for: %s (len=%d)", m.currentSong.Title, len(rendered))
	m.albumArtStr = rendered
	m.albumArtLoaded = true
	return m, nil
}

// handleLyricsFetched handles lyrics fetch completion
func (m Model) handleLyricsFetched(msg lyricsFetchedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}
	if msg.err != nil {
		m.lyrics = "Lyrics not found"
		return m, nil
	}

	m.lyrics = msg.plain
	m.syncedLyrics = msg.synced

	if m.bottomViewMode == ViewLyrics || m.bottomViewMode == ViewSyncedLyrics {
		m.updateBottomView()
	}

	return m, nil
}

// handleArtistFetched handles artist info fetch completion
func (m Model) handleArtistFetched(msg artistFetchedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}
	if msg.err != nil {
		return m, nil
	}

	m.artistInfo = msg.info

	if m.bottomViewMode == ViewArtist {
		m.updateBottomView()
	}

	return m, nil
}

// updatePlaylist updates the playlist table
func (m *Model) updatePlaylist() {
	if len(m.songs) == 0 {
		return
	}

	rows := make([]table.Row, len(m.songs))
	for i, song := range m.songs {
		prefix := ""
		if m.cacheManager.IsFavorite(song) {
			prefix = "⭐ "
		} else if m.cacheManager.IsBlocked(song) {
			prefix = "🚫 "
		}

		rows[i] = table.Row{
			fmt.Sprintf("%d", i+1),
			prefix + song.Title,
			song.Artist,
			song.GetDurationFormatted(),
			song.Album,
			song.Year,
		}
	}

	m.playlistWidget.SetRows(rows)
	m.playlistWidget.SetCursor(m.currentSongIndex)
}

// updateBottomView updates the viewport content based on current mode
func (m *Model) updateBottomView() {
	var content string

	switch m.bottomViewMode {
	case ViewLyrics:
		if m.lyrics == "" {
			content = "Loading lyrics..."
		} else {
			content = m.lyrics
		}

	case ViewSyncedLyrics:
		if len(m.syncedLyrics) == 0 {
			content = "No synced lyrics available"
		} else {
			// Use cached playback position (updated every tick)
			// Find current line
			currentLineIdx := -1
			for i, line := range m.syncedLyrics {
				if line.Time <= m.playbackPos.TimePos {
					currentLineIdx = i
				}
			}

			// Display lyrics around current position
			startIdx := currentLineIdx - 3
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx := currentLineIdx + 4
			if endIdx > len(m.syncedLyrics) {
				endIdx = len(m.syncedLyrics)
			}

			var lines []string
			for i := startIdx; i < endIdx; i++ {
				if i == currentLineIdx {
					lines = append(lines, "▶ "+m.syncedLyrics[i].Content)
				} else {
					lines = append(lines, "  "+m.syncedLyrics[i].Content)
				}
			}
			content = strings.Join(lines, "\n")
		}

	case ViewArtist:
		if m.artistInfo == nil {
			content = "Loading artist info..."
		} else {
			var lines []string
			lines = append(lines, fmt.Sprintf("=== %s ===", m.artistInfo.PageTitle))
			lines = append(lines, "")
			lines = append(lines, m.artistInfo.Summary)
			if m.artistInfo.Discography != "" {
				lines = append(lines, "")
				lines = append(lines, "Studio Albums:")
				discoLines := strings.Split(m.artistInfo.Discography, "\n")
				for _, line := range discoLines {
					lines = append(lines, "  "+line)
				}
			}
			content = strings.Join(lines, "\n")
		}

	case ViewPlaylist:
		// Playlist is rendered separately via table component
		return

	case ViewOff:
		content = ""
	}

	m.viewport.SetContent(content)
}

// songChangedCmds is the centralized handler for all song transitions
// (initial load, manual skip/prev, natural transition).
// It updates model state and returns Cmds for all async fetches.
func (m *Model) songChangedCmds() tea.Cmd {
	if m.currentSongIndex < 0 || m.currentSongIndex >= len(m.songs) {
		return nil
	}

	m.currentSong = m.songs[m.currentSongIndex]
	m.playlistWidget.SetCursor(m.currentSongIndex)
	m.statusMsg = "Now playing: " + m.currentSong.Title

	// Clear stale data
	m.lyrics = ""
	m.syncedLyrics = nil
	m.artistInfo = nil
	m.albumArtStr = ""
	m.albumArtLoaded = false
	m.playbackPos = mpv.PlaybackPosition{}

	// Update cached favorite count
	if count, err := m.cacheManager.GetFavoriteCount(); err == nil {
		m.favoriteCount = count
	}

	// Update playlist and bottom view
	m.updatePlaylist()
	m.updateBottomView()

	var cmds []tea.Cmd

	// Reset progress bar
	if cmd := m.nowPlayingWidget.UpdateProgress(0); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Load album art (if available)
	if m.currentSong.CoverLarge != "" {
		logger.Printf("Loading album art: %s", m.currentSong.CoverLarge)
		cmds = append(cmds, m.loadImageCmd(m.currentSong.CoverLarge))
	} else {
		logger.Printf("No album art URL for song: %s", m.currentSong.Title)
	}

	// Always fetch lyrics and artist info (not gated on album art)
	cmds = append(cmds, m.fetchLyricsCmd())
	cmds = append(cmds, m.fetchArtistCmd())

	return tea.Batch(cmds...)
}

// checkAndQueueFavorite checks if we should queue a favorite for auto-playback.
// Called every progress tick (1s). Mirrors Python's _check_and_queue_favorite_if_needed.
func (m *Model) checkAndQueueFavorite() tea.Cmd {
	if !m.pollingNextBlock {
		return nil
	}
	if m.favoriteCount < m.config.MinFavorites {
		return nil
	}
	if !m.mpvBackend.IsRunning() {
		return nil
	}
	if time.Since(m.lastFavoriteQueuedAt) < 30*time.Second {
		return nil
	}
	if m.currentSong == nil || m.playbackPos.TimePos < 0 {
		return nil
	}

	songDuration := float64(m.currentSong.Duration) / 1000.0
	timeRemaining := songDuration - m.playbackPos.TimePos
	if timeRemaining > 10 {
		return nil
	}

	logger.Printf("About to stop (%.1fs remaining), queueing favorite", timeRemaining)
	m.lastFavoriteQueuedAt = time.Now()
	return m.queueNextFavorite()
}

// queueNextFavorite picks a random favorite and appends it to the MPV playlist.
// Mirrors Python's _queue_next_favorite.
func (m *Model) queueNextFavorite() tea.Cmd {
	favorites, err := m.cacheManager.GetFavorites()
	if err != nil || len(favorites) == 0 {
		m.statusMsg = "No favorites available..."
		return nil
	}

	// Refill shuffled queue if empty
	if len(m.favoritesQueue) == 0 {
		m.favoritesQueue = make([]cache.CachedSong, len(favorites))
		copy(m.favoritesQueue, favorites)
		rand.Shuffle(len(m.favoritesQueue), func(i, j int) {
			m.favoritesQueue[i], m.favoritesQueue[j] = m.favoritesQueue[j], m.favoritesQueue[i]
		})
		logger.Printf("Shuffled favorites queue with %d songs", len(m.favoritesQueue))
	}

	// Pop first item
	fav := m.favoritesQueue[0]
	m.favoritesQueue = m.favoritesQueue[1:]

	song := fav.ToSong()

	// Append to MPV playlist
	if err := m.mpvBackend.AppendToPlaylist([]string{song.GaplessURL}); err != nil {
		logger.Printf("Failed to append favorite to playlist: %v", err)
		m.statusMsg = fmt.Sprintf("Error queueing favorite: %v", err)
		return nil
	}

	// Append to song list (don't change currentSongIndex — let natural
	// transition detection in handleProgressTick pick it up when MPV
	// actually starts playing the new track)
	m.songs = append(m.songs, song)

	m.statusMsg = fmt.Sprintf("★ Queued favorite: %s", song.Title)
	logger.Printf("Appended favorite to playlist: %s", song.Title)

	// Update playlist display to show the queued song
	m.updatePlaylist()
	return nil
}

// fetchLyricsCmd fetches lyrics for the current song
func (m Model) fetchLyricsCmd() tea.Cmd {
	if m.currentSong == nil {
		return nil
	}

	song := m.currentSong // capture for closure
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		result, err := m.lyricsClient.GetLyricsByDuration(
			ctx,
			song.Artist,
			song.Title,
			song.Album,
			float64(song.Duration),
		)

		if err != nil {
			return lyricsFetchedMsg{eventID: song.EventID, err: err}
		}

		if result == nil {
			return lyricsFetchedMsg{eventID: song.EventID, err: fmt.Errorf("lyrics not found")}
		}

		var syncedLyrics []api.SyncedLyric
		if result.SyncedLyrics != "" {
			syncedLyrics = api.ParseSyncedLyrics(result.SyncedLyrics)
		}

		return lyricsFetchedMsg{
			eventID: song.EventID,
			plain:   result.PlainLyrics,
			synced:  syncedLyrics,
		}
	}
}

// fetchArtistCmd fetches artist info for the current song
func (m Model) fetchArtistCmd() tea.Cmd {
	if m.currentSong == nil {
		return nil
	}

	song := m.currentSong // capture for closure
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		info, err := m.wikipediaClient.FindArtist(ctx, song.Artist)
		if err != nil {
			return artistFetchedMsg{eventID: song.EventID, err: err}
		}

		return artistFetchedMsg{
			eventID: song.EventID,
			info:    info,
		}
	}
}

// loadImageCmd loads an image from URL
func (m Model) loadImageCmd(url string) tea.Cmd {
	if m.currentSong == nil {
		return nil
	}

	eventID := m.currentSong.EventID // capture for closure
	songTitle := m.currentSong.Title // capture for logging
	return func() tea.Msg {
		logger.Printf("Fetching album art: %s for %s", url, songTitle)
		resp, err := http.Get(url)
		if err != nil {
			logger.Printf("Album art fetch error: %v", err)
			return imageLoadedMsg{eventID: eventID, err: err}
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Printf("Album art read error: %v", err)
			return imageLoadedMsg{eventID: eventID, err: err}
		}

		logger.Printf("Album art fetched: %s, %d bytes", songTitle, len(data))
		return imageLoadedMsg{
			eventID:   eventID,
			imageData: data,
		}
	}
}

// altView wraps content in a tea.View with AltScreen enabled
func altView(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

// View renders the TUI
func (m Model) View() tea.View {
	if m.err != nil {
		return altView(fmt.Sprintf("Error: %v\n\nPress q to quit", m.err))
	}

	if !m.initialized {
		return altView("Loading...\n\nPress q to quit")
	}

	// If a modal is active, render it full-screen (like the Python app).
	// This avoids z-order issues with Kitty album art graphics.
	if m.activeModal != ModalNone {
		var modalView string
		switch m.activeModal {
		case ModalOptions:
			modalView = m.optionsModal.View()
		case ModalSkipWarning:
			modalView = m.skipWarningModal.View()
		case ModalFavorites:
			if m.favoritesModal != nil {
				modalView = m.favoritesModal.View()
			}
		}

		if modalView != "" {
			// Kitty images are cleared via tea.Raw when the modal opens
			// (see clearKittyImagesCmd). We don't embed the clear sequence
			// in the view content because the cell-based renderer strips it.

			// Center the modal vertically and horizontally
			modalLines := strings.Split(modalView, "\n")
			modalHeight := len(modalLines)
			modalWidth := 0
			for _, line := range modalLines {
				if w := lipgloss.Width(line); w > modalWidth {
					modalWidth = w
				}
			}

			padTop := max(0, (m.height-modalHeight)/2)
			padLeft := max(0, (m.width-modalWidth)/2)
			leftPad := strings.Repeat(" ", padLeft)

			var b strings.Builder
			for i := 0; i < padTop; i++ {
				b.WriteString("\n")
			}
			for _, line := range modalLines {
				b.WriteString(leftPad + line + "\n")
			}

			return altView(b.String())
		}
	}

	// 1. Header
	header := m.headerWidget.View()

	// 2. Main Content (Now Playing + Album Art)
	// Use cached playback position (updated every tick, no IPC here)

	// Now-playing always gets full width
	m.nowPlayingWidget.SetWidth(m.width - 4)

	nowPlayingView := m.nowPlayingWidget.View(
		m.currentSong,
		m.isPaused,
		m.playbackPos.TimePos,
		m.connectedAt,
		m.config.GetDisplayInfo(),
		m.getFavoriteCount(),
		m.cacheManager.IsFavorite(m.currentSong),
		m.getSkipsAvailable(),
		m.getPrevAvailable(),
	)

	// 3. Bottom Section (Playlist or other)
	var bottomSection string
	if m.bottomViewMode == ViewPlaylist {
		bottomSection = m.playlistWidget.View()
	} else if m.bottomViewMode != ViewOff {
		bottomSection = m.viewport.View()
	}

	// 4. Footer
	footer := m.footerWidget.View()

	var b strings.Builder
	b.WriteString(header + "\n\n")

	// Now-playing info
	b.WriteString(nowPlayingView + "\n")

	// The bottom section should fill the remaining space except for the footer
	currentHeight := lipgloss.Height(b.String())
	remainingHeight := m.height - currentHeight - 1 // -1 for footer

	if remainingHeight > 0 {
		// Crop or pad bottom section
		bottomLines := strings.Split(bottomSection, "\n")
		for i := 0; i < remainingHeight; i++ {
			if i < len(bottomLines) {
				b.WriteString(bottomLines[i] + "\n")
			} else {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString(footer)

	// 5. Album art positioned at upper right using cursor escape sequences.
	//    Clear all previous Kitty images first to prevent stale art when
	//    the new image has the same pixel dimensions as the previous one.
	var artSuffix string
	if m.config.ShowAlbumArt && m.albumArtLoaded && m.albumArtStr != "" {
		// Recalculate dimensions for positioning (same formula as in handleImageLoaded)
		artHeight := 16
		artWidth := int(float64(artHeight) * m.cellRatio)
		if artWidth < 10 {
			artWidth = 10
		}
		// Position with 2-cell right padding
		artCol := m.width - artWidth - 2
		if artCol < 1 {
			artCol = 1
		}
		clearStr := termimg.ClearAllString()
		artSuffix = fmt.Sprintf("%s\x1b[s\x1b[2;%dH%s\x1b[u", clearStr, artCol, m.albumArtStr)
	}

	return altView(b.String() + artSuffix)
}

// getFavoriteCount returns the number of favorites
func (m Model) getFavoriteCount() int {
	return m.favoriteCount
}

func (m Model) getSkipsAvailable() int {
	return max(0, len(m.songs)-1-m.currentSongIndex)
}

func (m Model) getPrevAvailable() int {
	return m.currentSongIndex
}
