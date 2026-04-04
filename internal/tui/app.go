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
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/blacktop/go-termimg"
	"github.com/charmbracelet/x/ansi"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/cache"
	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/models"
	"rptui-bubbletea/internal/mpv"
	"rptui-bubbletea/internal/tui/modals"
	"rptui-bubbletea/internal/tui/visualizer"
	"rptui-bubbletea/internal/tui/widgets"
)

// Logger for TUI
var logger *log.Logger

func init() {
	logger = loginit.InitLogger("[TUI] ")
}

// artistArtCacheEntry stores a cached artist thumbnail render
type artistArtCacheEntry struct {
	rendered string
	width    int
	height   int
}

// Bottom view mode constants
const (
	ViewPlaylist = iota
	ViewLyrics
	ViewSyncedLyrics
	ViewArtist
	ViewVisualizer
	ViewOff
	ViewModeCount
)

// Scrobble flash states
const (
	flashOff      = 0
	flashSolid    = 1 // success — accent for 5s
	flashBlinkOn  = 2 // failure — accent visible
	flashBlinkOff = 3 // failure — muted
	flashDuration = 5 * time.Second
)

var bottomViewNames = []string{
	"Playlist",
	"Lyrics",
	"Synced Lyrics",
	"Artist",
	"Visualizer",
	"Off",
}

// Modal types
const (
	ModalNone = iota
	ModalOptions
	ModalSkipWarning
	ModalFavorites
	ModalGallery
)

// Connection retry constants
const (
	connStateConnected    = "connected"
	connStateDisconnected = "disconnected"

	retryInitialInterval = 5 * time.Second
	retryMaxInterval     = 60 * time.Second
	retryMultiplier      = 2
)

// Model represents the main TUI application model
type Model struct {
	// Configuration
	config       *config.Config
	theme        *config.ColorTheme
	styles       *config.ThemeStyles
	themeWatcher *config.ThemeWatcher

	// API Clients
	rpAPI             *api.RadioParadiseAPI
	lyricsClient      *api.LRCLibClient
	wikipediaClient   *api.WikipediaClient
	discogsClient     *api.DiscogsClient
	musicbrainzClient *api.MusicBrainzClient
	theaudiodbClient  *api.TheAudioDBClient
	mpvBackend        *mpv.MPVBackend
	cacheManager      *cache.CacheManager

	// State
	songs            []*models.Song
	currentSongIndex int
	playlistStartIdx int
	isPlaying        bool
	isPaused         bool
	bottomViewMode   int
	imageBase        string
	imageCounter     int  // for unique image IDs
	skipWarningShown bool // track if skip warning has been shown this session
	mutedForBlocked  bool // MPV muted to silence a blocklisted last song

	// Current song info
	currentSong *models.Song

	// Playback position (cached from MPV, updated every tick - avoid IPC in View)
	playbackPos mpv.PlaybackPosition

	// Connected time (set once when playback starts)
	connectedAt time.Time

	// Next block polling
	pollingNextBlock bool
	lastBlockID      int // track last block ID to detect new vs cached response

	// Auto-favorite playback
	favoritesQueue       []cache.CachedSong
	lastFavoriteQueuedAt time.Time

	// Bottom view content (displayed to user)
	lyrics       string
	syncedLyrics []api.SyncedLyric
	artistInfo   *models.ArtistInfo
	artistStatus string                        // current loading status for artist view
	artistCache  map[string]*models.ArtistInfo // keyed by lowercase artist name

	// Pending content (fetched for current song, not yet shown).
	// User presses 'u' to update displayed content from pending.
	// Synced lyrics bypass this — they always auto-update.
	pendingLyrics          string
	pendingArtistInfo      *models.ArtistInfo
	pendingEventID         int64  // eventID the pending data belongs to
	pendingArtistArtStr    string // pending rendered artist thumbnail
	pendingArtistArtLoaded bool
	pendingArtistArtWidth  int
	pendingArtistArtHeight int

	// Bubbles components
	viewport    viewport.Model
	albumArtStr string // cached rendered escape sequence

	// Cached album art render string (only re-render when image changes)
	albumArtLoaded bool

	// Artist thumbnail image (rendered beside artist info viewport)
	artistArtStr     string // cached rendered escape sequence
	artistArtLoaded  bool
	artistArtEventID int64                          // eventID of the song that triggered this image
	artistArtWidth   int                            // width in terminal columns
	artistArtHeight  int                            // height in terminal rows
	artistArtCache   map[string]artistArtCacheEntry // keyed by lowercase artist name

	// Custom Widgets
	headerWidget     *widgets.Header
	footerWidget     *widgets.Footer
	nowPlayingWidget *widgets.NowPlaying
	playlistWidget   *widgets.Playlist

	// Modal Widgets
	optionsModal     *modals.Options
	skipWarningModal *modals.SkipWarning
	favoritesModal   *modals.Favorites
	galleryModal     *modals.Gallery

	// UI dimensions
	width  int
	height int

	// Status
	statusMsg     string
	statusIsError bool
	statusSeq     int

	// Scrobble support
	scrobbler          *api.Scrobbler
	songStartTime      time.Time
	scrobbleEligible   bool
	scrobbleFlashAt    time.Time // when scrobble flash started
	scrobbleFlashState int       // 0=off, 1=solid accent (success), 2=blink-on, 3=blink-off

	// Connection monitoring
	connState           string // "connected", "disconnected", "reconnecting"
	consecutiveFailures int
	retryInterval       time.Duration // current backoff interval
	connErrorMsg        string        // persistent error message (no auto-clear)

	// Error state
	err error

	// Modal state
	activeModal int

	// Favorite download tracking
	downloadResults chan favoriteDownloadMsg
	downloadingFav  int64 // eventID of song currently being downloaded (0 = none)

	// Initialization complete
	initialized bool

	// Light/dark mode
	isDark bool

	// Help
	help help.Model

	// Spinner for "awaiting new songs" animation
	spinner spinner.Model

	// Terminal cell ratio for album art aspect ratio correction
	cellRatio float64

	// Notification tracking
	notifSentForSong bool // true once desktop notification fired for current song

	// Visualizer state
	vis            *visualizer.Visualizer // visualizer engine
	visFullscreen  bool                   // visualizer is in fullscreen mode
	visInfoShownAt time.Time              // when song info overlay was last shown
	visInfoVisible bool                   // whether info overlay is currently visible
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config, theme *config.ColorTheme) *Model {
	styles := config.NewThemeStyles(theme)

	themeWatcher := config.NewThemeWatcher(cfg.ColorsFile)
	if err := themeWatcher.Start(); err != nil {
		logger.Printf("Warning: failed to start theme watcher: %v", err)
	} else {
		logger.Printf("Theme watcher started successfully")
	}

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
	discogsClient := api.NewDiscogsClient(cfg.DiscogsToken, cfg.DiscogsKey, cfg.DiscogsSecret)
	if discogsClient.HasAuth() {
		logger.Printf("Discogs API: authenticated (images enabled)")
	} else {
		logger.Printf("Discogs API: unauthenticated (no images, limited rate)")
	}
	musicbrainzClient := api.NewMusicBrainzClient()
	theaudiodbClient := api.NewTheAudioDBClient()
	mpvBackend := mpv.NewMPVBackend()
	cacheManager := cache.NewCacheManager(
		cfg.GetFavoritesDir(),
		cfg.GetBlocklistDir(),
		cfg.MaxFavorites,
	)
	scrobbler := api.NewScrobbler(cfg)
	if scrobbler.Enabled() {
		logger.Printf("Scrobble enabled: %v", scrobbler.ServiceNames())
	}
	if err := cacheManager.EnsureDirectories(); err != nil {
		logger.Printf("Warning: failed to create cache directories: %v", err)
	}

	// Initialize custom widgets
	headerWidget := widgets.NewHeader(styles.Header, "rptui - Radio Paradise")
	footerWidget := widgets.NewFooter(styles.AccentStyle, styles.MutedStyle)
	footerWidget.SetScrobbleServices(scrobbler.ServiceNames())
	nowPlayingWidget := widgets.NewNowPlaying(styles.ForegroundStyle, styles.AccentStyle, styles.MutedStyle, theme.Accent, theme.Cursor, theme.Background)
	playlistWidget := widgets.NewPlaylist(styles)

	// Initialize modal widgets
	optionsModal := modals.NewOptions(styles, cfg.Channel, cfg.Bitrate, cfg.ShowAlbumArt, cfg.ShowSkipWarning, cfg.CopyAlbumArt, cfg.NotificationsEnabled, cfg.NotificationsShowArt, cfg.Visualizer.Mode)
	skipWarningModal := modals.NewSkipWarning(styles, cfg.MinFavorites)

	// Initialize viewport for bottom views
	viewport := viewport.New(
		viewport.WithWidth(100),
		viewport.WithHeight(15),
	)
	viewport.SoftWrap = true
	viewport.Style = lipgloss.NewStyle().
		Background(lipgloss.Color(theme.Background)).
		Foreground(lipgloss.Color(theme.Foreground))

	// Initialize help
	help := help.New()

	// Initialize spinner
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent))

	return &Model{
		config:            cfg,
		theme:             theme,
		styles:            styles,
		themeWatcher:      themeWatcher,
		rpAPI:             rpAPI,
		lyricsClient:      lyricsClient,
		wikipediaClient:   wikipediaClient,
		discogsClient:     discogsClient,
		musicbrainzClient: musicbrainzClient,
		theaudiodbClient:  theaudiodbClient,
		mpvBackend:        mpvBackend,
		cacheManager:      cacheManager,
		scrobbler:         scrobbler,
		bottomViewMode:    ViewPlaylist,
		headerWidget:      headerWidget,
		footerWidget:      footerWidget,
		nowPlayingWidget:  nowPlayingWidget,
		playlistWidget:    playlistWidget,
		optionsModal:      optionsModal,
		skipWarningModal:  skipWarningModal,
		viewport:          viewport,
		help:              help,
		spinner:           sp,
		cellRatio:         cellRatio,
		downloadResults:   make(chan favoriteDownloadMsg, 1),
	}
}

// Init initializes the TUI model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.fetchBlockCmd,
		tickProgressCmd(),
		tickPollCmd(),
		tea.RequestBackgroundColor,
		m.downloadResultsCmd(),
	}
	if m.themeWatcher != nil {
		cmds = append(cmds, watchThemeCmd(m.themeWatcher))
	}
	return tea.Batch(cmds...)
}

// fetchBlockCmd fetches a block from the API.
// Returns data only — handleBlockFetched decides whether to start/append MPV.
func (m Model) fetchBlockCmd() tea.Msg {
	block, err := m.rpAPI.GetBlock(context.Background())
	if err != nil {
		return blockFetchedMsg{err: fmt.Errorf("GetBlock error: %w", err)}
	}

	songs, imageBase := m.rpAPI.ParseBlockSongs(block)
	if len(songs) == 0 {
		return blockFetchedMsg{err: fmt.Errorf("no songs in block")}
	}

	blockID := 0
	if block.BlockID != "" {
		fmt.Sscanf(block.BlockID, "%d", &blockID)
	}
	return blockFetchedMsg{songs: songs, imageBase: imageBase, blockID: blockID}
}

// downloadResultsCmd returns a persistent command that reads from the download results channel.
func (m Model) downloadResultsCmd() tea.Cmd {
	return func() tea.Msg {
		return <-m.downloadResults
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

	// Always update spinner (for animation)
	var spinnerCmd tea.Cmd
	m.spinner, spinnerCmd = m.spinner.Update(msg)
	if spinnerCmd != nil {
		cmds = append(cmds, spinnerCmd)
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

		contentWidth := msg.Width
		contentHeight := max(5, msg.Height-15)

		m.viewport.SetWidth(contentWidth)
		m.viewport.SetHeight(contentHeight)
		m.playlistWidget.SetSize(contentWidth, contentHeight)

		return handle(m, renderAlbumArtAfterDelay())

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
			case ModalGallery:
				if m.galleryModal != nil {
					cmd = m.galleryModal.Update(msg)
				}
			}
			return handle(m, cmd)
		}
		newModel, cmd := m.handleKeyPress(msg)
		return handle(newModel, cmd)

	case modals.OptionsMsg:
		m.activeModal = ModalNone
		if msg.Closed {
			return handle(m, renderAlbumArtAfterDelay())
		}

		// Apply toggle config changes (these don't require restart)
		if msg.ShowAlbumArt != nil {
			m.config.ShowAlbumArt = *msg.ShowAlbumArt
		}
		if msg.ShowSkipWarn != nil {
			m.config.ShowSkipWarning = *msg.ShowSkipWarn
		}
		if msg.CopyAlbumArt != nil {
			m.config.CopyAlbumArt = *msg.CopyAlbumArt
		}
		if msg.NotificationsEnabled != nil {
			m.config.NotificationsEnabled = *msg.NotificationsEnabled
		}
		if msg.NotificationsShowArt != nil {
			m.config.NotificationsShowArt = *msg.NotificationsShowArt
		}
		if msg.VisualizerMode != nil {
			m.config.Visualizer.Mode = *msg.VisualizerMode
			if m.vis != nil {
				mode := visualizer.ModeFromString(*msg.VisualizerMode)
				m.vis.SetMode(mode)
				m.vis.RequestRefresh()
			}
		}

		needsRestart := msg.Station != nil || msg.Bitrate != nil

		// Apply station/bitrate changes
		if msg.Station != nil {
			m.config.Channel = *msg.Station
			m.rpAPI.SetChannel(*msg.Station)
		}
		if msg.Bitrate != nil {
			m.config.Bitrate = *msg.Bitrate
			m.rpAPI.SetBitrate(*msg.Bitrate)
		}
		m.config.Save()

		if !needsRestart {
			return handle(m, renderAlbumArtAfterDelay())
		}

		// Full restart: stop MPV, clear state, re-fetch fresh block
		m.mpvBackend.Stop()
		m.songs = nil
		m.currentSongIndex = 0
		m.playlistStartIdx = 0
		m.currentSong = nil
		m.isPlaying = false
		m.isPaused = false
		m.pollingNextBlock = false
		m.lastBlockID = 0
		m.lyrics = ""
		m.syncedLyrics = nil
		m.artistInfo = nil
		m.artistStatus = ""
		m.pendingLyrics = ""
		m.pendingArtistInfo = nil
		m.pendingEventID = 0
		m.pendingArtistArtStr = ""
		m.pendingArtistArtLoaded = false
		m.pendingArtistArtWidth = 0
		m.pendingArtistArtHeight = 0
		m.albumArtLoaded = false
		m.albumArtStr = ""
		m.artistArtStr = ""
		m.artistArtLoaded = false
		m.artistArtEventID = 0
		m.initialized = false
		m.connectedAt = time.Time{}

		return handle(m, tea.Batch(setStatus(&m, "Restarting...", false), m.fetchBlockCmd))

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
		return handle(m, renderAlbumArtAfterDelay())

	case modals.FavoritesMsg:
		m.activeModal = ModalNone
		if msg.PlayEventID != nil {
			fav, err := m.cacheManager.GetFavoriteByEventID(*msg.PlayEventID)
			if err == nil && fav != nil {
				song := fav.ToSong()
				song.IsFromFavorite = true
				m.songs = []*models.Song{song}
				m.currentSongIndex = 0
				m.playlistStartIdx = 0

				if err := m.mpvBackend.Start([]string{song.GaplessURL}); err == nil {
					return handle(m, tea.Batch(setStatus(&m, "Playing favorite", false), m.songChangedCmds()))
				}
			}
		}
		return handle(m, renderAlbumArtAfterDelay())

	case modals.GalleryMsg:
		m.activeModal = ModalNone
		m.galleryModal = nil
		return handle(m, renderAlbumArtAfterDelay())

	case modals.GalleryImageLoadedMsg:
		if m.galleryModal != nil && m.activeModal == ModalGallery {
			cmd := m.galleryModal.HandleImageLoaded(msg)
			return m, cmd
		}
		return m, nil

	case modals.GalleryRenderImageMsg:
		if m.galleryModal != nil && m.activeModal == ModalGallery {
			return m, tea.Raw(msg.ImageStr)
		}
		return m, nil

	case blockFetchedMsg:
		return handle(m.handleBlockFetched(msg))

	case progressTickMsg:
		return handle(m.handleProgressTick(msg))

	case pollTickMsg:
		return handle(m.handlePollTick(msg))

	case connRetryTickMsg:
		return handle(m.handleConnRetryTick(msg))

	case imageLoadedMsg:
		return handle(m.handleImageLoaded(msg))

	case renderAlbumArtMsg:
		return handle(m, m.renderAlbumArtCmd())

	case lyricsFetchedMsg:
		return handle(m.handleLyricsFetched(msg))

	case artistFetchedMsg:
		return handle(m.handleArtistFetched(msg))

	case artistImageLoadedMsg:
		return handle(m.handleArtistImageLoaded(msg))

	case renderArtistArtMsg:
		return handle(m, m.renderArtistArtCmd())

	case artistStatusMsg:
		if m.currentSong != nil && msg.eventID == m.currentSong.EventID {
			m.artistStatus = msg.status
			if m.bottomViewMode == ViewArtist {
				m.updateBottomView()
			}
		}
		return handle(m, nil)

	case statusClearMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
			m.statusIsError = false
		}
		return handle(m, nil)

	case scrobbleResultMsg:
		m.scrobbleFlashAt = time.Now()
		m.scrobbleFlashState = flashSolid // default to success
		for _, r := range msg.results {
			if !r.Success {
				m.scrobbleFlashState = flashBlinkOn // any failure triggers blink
				break
			}
		}

	case favoriteDownloadMsg:
		m.downloadingFav = 0
		m.updatePlaylist()
		if msg.success {
			return m, setStatus(&m, "Added to favorites", false)
		}
		return m, setStatus(&m, "Failed to download favorite", true)

	case notificationSentMsg:
		m.notifSentForSong = true
		return m, nil

	case visTickMsg:
		if m.vis != nil && m.bottomViewMode == ViewVisualizer {
			m.vis.Tick(m.isPlaying, m.isPaused)
			// Update song info overlay visibility in fullscreen
			if m.visFullscreen && m.currentSong != nil {
				showInfo := m.config.Visualizer.ShowInfo
				if showInfo == "on" {
					m.visInfoVisible = true
				} else if showInfo == "fade" && m.visInfoVisible {
					if time.Since(m.visInfoShownAt) >= time.Duration(m.config.Visualizer.InfoDuration)*time.Second {
						m.visInfoVisible = false
					}
				}
			}
		}
		// Re-schedule visualizer tick
		if m.vis != nil && m.bottomViewMode == ViewVisualizer {
			cmds = append(cmds, tickVisCmd())
		}

	case themeChangedMsg:
		newTheme, err := config.LoadTheme(m.config.ColorsFile, m.config.Theme)
		if err != nil {
			return handle(m, watchThemeCmd(m.themeWatcher))
		}
		m.theme = newTheme
		m.styles = config.NewThemeStyles(newTheme)

		m.nowPlayingWidget.UpdateStyles(
			m.styles.ForegroundStyle,
			m.styles.AccentStyle,
			m.styles.MutedStyle,
			m.theme.Accent,
			m.theme.Cursor,
			m.theme.Background,
		)
		m.playlistWidget.UpdateStyles(m.styles)
		m.headerWidget.UpdateStyles(m.styles.Header)
		m.footerWidget.UpdateStyles(m.styles.AccentStyle, m.styles.MutedStyle)

		m.viewport.Style = lipgloss.NewStyle().
			Background(lipgloss.Color(m.theme.Background)).
			Foreground(lipgloss.Color(m.theme.Foreground))
		m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent))

		// Update visualizer theme colors
		if m.vis != nil {
			m.vis.SetColors(m.theme.Accent, m.theme.Cursor, m.theme.Muted)
		}

		return m, watchThemeCmd(m.themeWatcher)
	}

	cmds = append(cmds, m.downloadResultsCmd())
	return m, tea.Batch(cmds...)
}

// handleKeyPress handles keyboard input
func (m Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		// Wait for in-progress favorite downloads
		m.cacheManager.WaitForDownloads(60 * time.Second)
		// Scrobble current song if eligible before quitting
		if m.currentSong != nil && m.scrobbleEligible && m.scrobbler.Enabled() {
			go m.scrobbler.Scrobble(context.Background(), *m.currentSong, m.songStartTime)
		}
		// Cleanup MPV before quitting
		m.mpvBackend.Stop()
		if m.themeWatcher != nil {
			m.themeWatcher.Close()
		}
		if m.vis != nil {
			m.vis.Close()
		}
		return m, tea.Quit

	case "space":
		// Play/Pause
		if err := m.mpvBackend.TogglePause(); err == nil {
			m.isPaused = !m.isPaused
		}
		return m, nil

	case "n":
		// Skip next - show warning only once per session (suppressed if favorites mode enabled)
		favCount, _ := m.cacheManager.GetFavoriteCount()
		if m.config.ShowSkipWarning && !m.skipWarningShown && favCount < m.config.MinFavorites {
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
			return m, setStatus(&m, "No more songs in block", false)
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
				return m, setStatus(&m, "Restarting song", false)
			}
		}
		return m, nil

	case "r":
		// Restart song
		if err := m.mpvBackend.SeekToStart(); err == nil {
			return m, setStatus(&m, "Restarting song", false)
		}
		return m, nil

	case "left", "right":
		// Seek: left=-10s, right=+10s
		if m.currentSong == nil || !m.isPlaying || m.isPaused {
			return m, nil
		}
		var delta float64
		if msg.String() == "left" {
			delta = -10
		} else {
			delta = 10
		}
		if delta > 0 {
			remaining := m.currentSong.GetDurationSeconds() - m.playbackPos.TimePos
			maxDelta := remaining - 0.5
			if maxDelta < 0 {
				maxDelta = 0
			}
			if delta > maxDelta {
				delta = maxDelta
			}
		}
		if delta == 0 {
			return m, nil
		}
		if err := m.mpvBackend.SeekRelative(delta); err == nil {
			if delta > 0 {
				return m, setStatus(&m, fmt.Sprintf("Seek +%ds", int(delta)), false)
			} else {
				return m, setStatus(&m, fmt.Sprintf("Seek %ds", int(delta)), false)
			}
		}
		return m, nil

	case "v":
		// Toggle bottom view — suppressed in fullscreen visualizer
		if m.visFullscreen {
			return m, nil
		}
		prevMode := m.bottomViewMode
		m.bottomViewMode = (m.bottomViewMode + 1) % ViewModeCount
		statusCmd := setStatus(&m, fmt.Sprintf("View: %s", bottomViewNames[m.bottomViewMode]), false)

		var cmds []tea.Cmd
		cmds = append(cmds, statusCmd)

		// When leaving lyrics or artist view, apply any pending updates
		// so both views are current next time they're entered.
		if prevMode == ViewLyrics || prevMode == ViewArtist {
			if m.hasPendingUpdate() {
				m.applyPendingUpdate()
				if m.pendingArtistArtLoaded && m.bottomViewMode == ViewArtist {
					cmds = append(cmds, renderArtistArtAfterDelay())
				}
			}
		}

		// Clear artist image when leaving artist view
		if prevMode == ViewArtist && m.bottomViewMode != ViewArtist {
			cmds = append(cmds, clearKittyImagesCmd(), renderAlbumArtAfterDelay())
		}

		// Initialize visualizer when entering the view
		if m.bottomViewMode == ViewVisualizer {
			if m.vis == nil {
				seed := uint64(0)
				if m.currentSong != nil {
					seed = uint64(m.currentSong.EventID)
				}
				m.vis = visualizer.New(seed)
				m.vis.SetColors(m.theme.Accent, m.theme.Cursor, m.theme.Muted)
				mode := visualizer.ModeFromString(m.config.Visualizer.Mode)
				m.vis.SetMode(mode)
			} else {
				m.vis.SetColors(m.theme.Accent, m.theme.Cursor, m.theme.Muted)
			}
			// Start audio tap if real audio is enabled
			source := m.vis.EnableRealAudio(m.config.Visualizer.RealAudio)
			m.vis.RequestRefresh()
			cmds = append(cmds, tickVisCmd())
			cmds = append(cmds, setStatus(&m, "Visualizer: "+source, false))
		} else {
			// Stop audio tap when leaving visualizer view
			if m.vis != nil {
				m.vis.Close()
			}
		}

		// Fetch lyrics/artist if needed when entering those views
		if m.bottomViewMode == ViewLyrics || m.bottomViewMode == ViewSyncedLyrics {
			if m.lyrics == "" && m.currentSong != nil {
				cmds = append(cmds, m.fetchLyricsCmd())
			}
		} else if m.bottomViewMode == ViewArtist {
			if m.artistInfo == nil && m.currentSong != nil {
				cmds = append(cmds, m.fetchArtistCmd())
			}
			if m.artistArtLoaded && m.artistArtStr != "" {
				cmds = append(cmds, renderArtistArtAfterDelay())
			}
		}

		m.updateBottomView()
		return m, tea.Batch(cmds...)

	case "u":
		// Update displayed lyrics/artist info from pending
		if m.hasPendingUpdate() {
			hadPendingArt := m.pendingArtistArtLoaded
			m.applyPendingUpdate()
			var cmd tea.Cmd
			if hadPendingArt && m.bottomViewMode == ViewArtist {
				cmd = renderArtistArtAfterDelay()
			}
			return m, tea.Batch(cmd, setStatus(&m, "Updated to current song", false))
		}
		return m, nil

	case "f":
		// Toggle favorite
		if m.currentSong != nil {
			if m.cacheManager.IsFavorite(m.currentSong) {
				// Remove favorite
				if err := m.cacheManager.RemoveFavorite(m.currentSong.EventID); err == nil {
					m.updatePlaylist()
					return m, setStatus(&m, "Removed from favorites", false)
				}
			} else if m.downloadingFav == m.currentSong.EventID {
				return m, setStatus(&m, "Already downloading favorite", false)
			} else {
				// Start download
				m.downloadingFav = m.currentSong.EventID
				m.updatePlaylist()
				return m, tea.Batch(
					setStatus(&m, "Downloading favorite...", false),
					favoriteDownloadCmd(m.cacheManager, m.currentSong, m.rpAPI.GetFileExtension(), m.downloadResults),
				)
			}
		}
		return m, nil

	case "b":
		// Toggle blocklist
		var statusCmd tea.Cmd
		if m.currentSong != nil {
			wasBlocked := m.cacheManager.IsBlocked(m.currentSong)
			if _, err := m.cacheManager.ToggleBlocklist(m.currentSong); err == nil {
				if wasBlocked {
					statusCmd = setStatus(&m, "Removed from blocklist", false)
				} else {
					statusCmd = setStatus(&m, "Added to blocklist", false)
				}
				m.updatePlaylist()
			}
		}
		return m, statusCmd

	case "0", "1", "2", "3", "5":
		// Station hotkeys
		station := int(msg.String()[0] - '0')
		return m.switchStation(station)

	case "o":
		// Options modal — suppressed in fullscreen visualizer
		if m.visFullscreen {
			return m, nil
		}
		m.optionsModal = modals.NewOptions(m.styles, m.config.Channel, m.config.Bitrate, m.config.ShowAlbumArt, m.config.ShowSkipWarning, m.config.CopyAlbumArt, m.config.NotificationsEnabled, m.config.NotificationsShowArt, m.config.Visualizer.Mode)
		m.activeModal = ModalOptions
		return m, clearKittyImagesCmd()

	case "m":
		// Manage favorites modal — suppressed in fullscreen visualizer
		if m.visFullscreen {
			return m, nil
		}
		m.favoritesModal = modals.NewFavorites(m.styles, m.cacheManager, m.width, m.height)
		m.activeModal = ModalFavorites
		return m, clearKittyImagesCmd()

	case "i":
		// Gallery modal (only in artist view with images)
		if m.bottomViewMode == ViewArtist && m.artistInfo != nil && len(m.artistInfo.GalleryURLs) > 0 {
			m.galleryModal = modals.NewGallery(
				m.styles,
				m.artistInfo.GalleryURLs,
				m.artistInfo.GallerySource,
				m.width, m.height,
				m.cellRatio,
			)
			m.activeModal = ModalGallery
			return m, tea.Batch(
				clearKittyImagesCmd(),
				m.galleryModal.PrefetchImages(),
			)
		}

	case "$":
		// Open RP donate page
		return m, tea.Batch(setStatus(&m, "Opening RP donate page...", false), openDonatePageCmd)

	case "F":
		// Toggle fullscreen visualizer — only works when visualizer view is active
		if m.bottomViewMode == ViewVisualizer {
			m.visFullscreen = !m.visFullscreen
			if m.visFullscreen {
				return m, tea.Batch(
					setStatus(&m, "Visualizer: fullscreen", false),
					clearKittyImagesCmd(),
				)
			}
			// Exiting fullscreen: restore album art
			return m, tea.Batch(
				setStatus(&m, "Visualizer: windowed", false),
				clearKittyImagesCmd(),
				renderAlbumArtAfterDelay(),
			)
		}
		return m, nil

	case "up", "k":
		// Cycle visualizer modes when in visualizer view, otherwise scroll viewport
		if m.bottomViewMode == ViewVisualizer {
			if m.vis != nil {
				m.vis.CycleMode()
				m.vis.RequestRefresh()
				return m, setStatus(&m, fmt.Sprintf("Visualizer: %s", m.vis.ModeName()), false)
			}
		}
		m.viewport.ScrollUp(1)
		return m, nil

	case "down", "j":
		// Cycle visualizer modes (reverse) when in visualizer view, otherwise scroll viewport
		if m.bottomViewMode == ViewVisualizer {
			if m.vis != nil {
				m.vis.CycleModeReverse()
				m.vis.RequestRefresh()
				return m, setStatus(&m, fmt.Sprintf("Visualizer: %s", m.vis.ModeName()), false)
			}
		}
		m.viewport.ScrollDown(1)
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

	case "c":
		// Copy current song info to clipboard
		if m.currentSong != nil {
			m.statusMsg = "Copied to clipboard"
			m.statusIsError = false
			m.statusSeq++
			seq := m.statusSeq
			return m, tea.Batch(
				copyToClipboardCmd(m.currentSong),
				tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
					return statusClearMsg{seq: seq}
				}),
			)
		}
		return m, nil
	}

	return m, nil
}

// switchStation changes the station and restarts playback
func (m Model) switchStation(channel int) (tea.Model, tea.Cmd) {
	if channel == m.config.Channel {
		return m, nil
	}

	stationName := config.StationNames[channel]
	if stationName == "" {
		return m, nil
	}

	m.config.Channel = channel
	m.rpAPI.SetChannel(channel)
	m.config.Save()

	// Full restart: stop MPV, clear state, re-fetch fresh block
	m.mpvBackend.Stop()
	m.songs = nil
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.currentSong = nil
	m.isPlaying = false
	m.isPaused = false
	m.pollingNextBlock = false
	m.lastBlockID = 0
	m.lyrics = ""
	m.syncedLyrics = nil
	m.artistInfo = nil
	m.artistStatus = ""
	m.pendingLyrics = ""
	m.pendingArtistInfo = nil
	m.pendingEventID = 0
	m.pendingArtistArtStr = ""
	m.pendingArtistArtLoaded = false
	m.pendingArtistArtWidth = 0
	m.pendingArtistArtHeight = 0
	m.albumArtLoaded = false
	m.albumArtStr = ""
	m.artistArtStr = ""
	m.artistArtLoaded = false
	m.artistArtEventID = 0
	m.initialized = false
	m.connectedAt = time.Time{}
	m.connState = ""
	m.consecutiveFailures = 0
	m.retryInterval = 0
	m.connErrorMsg = ""

	return m, tea.Batch(setStatus(&m, fmt.Sprintf("Switching to %s...", stationName), false), m.fetchBlockCmd)
}

// handleBlockFetched handles the block fetched message
func (m Model) handleBlockFetched(msg blockFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if !m.initialized {
			// Fatal on startup — can't play without a block
			m.err = msg.err
			return m, setStatus(&m, fmt.Sprintf("Error: %v", msg.err), true)
		}

		// Already playing — track failure and use exponential backoff
		m.consecutiveFailures++

		// Classify the error for user-facing message
		errType := api.ClassifyConnError(msg.err)
		var errLabel string
		switch errType {
		case api.ConnErrorNetwork:
			errLabel = "No internet"
		case api.ConnErrorTimeout:
			errLabel = "RP timed out"
		case api.ConnErrorServer:
			errLabel = "RP unavailable"
		default:
			errLabel = "Connection error"
		}

		// Calculate backoff interval
		if m.retryInterval == 0 {
			m.retryInterval = retryInitialInterval
		} else {
			m.retryInterval = time.Duration(float64(m.retryInterval) * retryMultiplier)
			if m.retryInterval > retryMaxInterval {
				m.retryInterval = retryMaxInterval
			}
		}

		m.connState = connStateDisconnected
		m.connErrorMsg = fmt.Sprintf("⚠ %s • retrying in %ds...", errLabel, int(m.retryInterval.Seconds()))
		m.pollingNextBlock = true

		logger.Printf("Block fetch error #%d (%s), retry in %v: %v",
			m.consecutiveFailures, errLabel, m.retryInterval, msg.err)

		return m, tickConnRetryCmd(m.retryInterval)
	}

	// Success — was previously disconnected?
	wasDisconnected := m.connState == connStateDisconnected
	prevFailures := m.consecutiveFailures

	// Reset connection state
	m.consecutiveFailures = 0
	m.retryInterval = 0
	m.connState = connStateConnected
	m.connErrorMsg = ""

	// If was disconnected, prepare "Reconnected" message (auto-clears)
	var reconnectedCmd tea.Cmd
	if wasDisconnected {
		logger.Printf("Connection restored after %d failed attempt(s)", prevFailures)
		reconnectedCmd = setStatus(&m, "✓ Reconnected", false)
	}

	// Check for promo block (blockID == 0) — skip in all cases
	if msg.blockID == 0 {
		logger.Printf("Skipping promo block (blockID=0)")
		if !m.initialized {
			return m, tea.Batch(setStatus(&m, "Waiting for stream...", false), m.fetchBlockCmd)
		}
		return m, m.fetchBlockCmd
	}

	// If already initialized, we're appending to existing playlist
	if m.initialized {
		// Check for cached response (block ID decreased)
		if msg.blockID > 0 && msg.blockID < m.lastBlockID {
			logger.Printf("Block ID decreased (%d -> %d), likely cached response, skipping", m.lastBlockID, msg.blockID)
			return m, nil
		}

		// Update last block ID
		if msg.blockID > 0 {
			m.lastBlockID = msg.blockID
		}

		// Check if last API song (skip favorites) is still in response
		// Python walks backward to find last non-favorite song's event ID
		if len(m.songs) > 0 {
			var lastAPIEventID int64
			for i := len(m.songs) - 1; i >= 0; i-- {
				if !m.songs[i].IsFromFavorite {
					lastAPIEventID = m.songs[i].EventID
					break
				}
			}

			if lastAPIEventID > 0 {
				for _, song := range msg.songs {
					if song.EventID == lastAPIEventID {
						logger.Printf("Last API song still in response, continuing to poll")
						return m, nil
					}
				}
			}
		}

		// Guard: if fewer than 2 songs, likely partial/early response
		if len(msg.songs) < 2 {
			logger.Printf("Only %d song(s) in new block, continuing to poll", len(msg.songs))
			return m, nil
		}

		// New block received - append or restart
		logger.Printf("New block received (blockID=%d), %d songs", msg.blockID, len(msg.songs))

		// Build URLs for new songs
		urls := make([]string, len(msg.songs))
		for i, song := range msg.songs {
			urls[i] = song.GaplessURL
		}

		mpvWasStopped := !m.mpvBackend.IsRunning()

		if mpvWasStopped {
			// MPV has stopped - restart with new block
			logger.Printf("MPV stopped, restarting playback with new block")
			m.currentSongIndex = len(m.songs) // will point to first new song
			m.songs = append(m.songs, msg.songs...)
			m.playlistStartIdx = m.currentSongIndex
			if err := m.mpvBackend.Start(urls); err != nil {
				logger.Printf("Failed to restart MPV: %v", err)
				return m, nil
			}
			m.isPlaying = true
		} else {
			// MPV still running - append to playlist
			if err := m.mpvBackend.AppendToPlaylist(urls); err != nil {
				logger.Printf("Failed to append to playlist: %v", err)
				return m, nil
			}
			m.songs = append(m.songs, msg.songs...)

			// Unmute if we muted for a blocked song, and skip past it
			if m.mutedForBlocked {
				m.mpvBackend.SetMute(false)
				m.mpvBackend.SkipNext()
				m.mutedForBlocked = false
				logger.Printf("Unmuted and skipped past blocklisted song")
			}
		}

		m.imageBase = msg.imageBase
		m.pollingNextBlock = false

		m.updatePlaylist()

		logger.Printf("Total songs: %d, currentIdx: %d", len(m.songs), m.currentSongIndex)

		// If MPV was restarted, trigger song change UI update
		if mpvWasStopped {
			return m, tea.Batch(reconnectedCmd, m.songChangedCmds())
		}
		return m, reconnectedCmd
	}

	// First load (not initialized yet) — start MPV
	urls := make([]string, len(msg.songs))
	for i, song := range msg.songs {
		urls[i] = song.GaplessURL
	}
	if err := m.mpvBackend.Start(urls); err != nil {
		m.err = fmt.Errorf("MPV error: %w", err)
		return m, setStatus(&m, fmt.Sprintf("Error: %v", err), true)
	}

	m.songs = msg.songs
	m.imageBase = msg.imageBase
	m.lastBlockID = msg.blockID
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.isPlaying = true
	m.pollingNextBlock = false
	m.connectedAt = time.Now()
	m.initialized = true

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
		// MPV has stopped - if on last song, enable polling
		if m.currentSongIndex >= len(m.songs)-1 {
			if !m.pollingNextBlock {
				logger.Printf("Playback stopped at end of playlist, enabling polling")
				m.pollingNextBlock = true
				cmds = append(cmds, m.spinner.Tick)
			}
		}
	} else {
		m.playbackPos = pos
	}

	// Sync pause state with MPV (detect external changes e.g. media keys via mpris)
	m.isPaused = m.mpvBackend.QueryPauseState()

	// Update progress bar (0.0 to 1.0) - only if we got a valid position
	if err == nil {
		if cmd := m.nowPlayingWidget.UpdateProgress(pos.PercentPos / 100.0); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Check for natural song transition (like Python's _detect_song_transition)
	// Account for playlistStartIdx offset: MPV playlist position is relative
	// to when MPV was started, but our songs[] index is absolute
	mpvPos, mpvErr := m.mpvBackend.GetPlaylistPosition()
	if mpvErr == nil && mpvPos >= 0 {
		absoluteIdx := m.playlistStartIdx + mpvPos
		if absoluteIdx != m.currentSongIndex && absoluteIdx >= 0 && absoluteIdx < len(m.songs) {
			m.currentSongIndex = absoluteIdx
			// Auto-skip blocklisted songs
			if m.cacheManager.IsBlocked(m.songs[m.currentSongIndex]) {
				logger.Printf("Auto-skipping blocklisted: %s", m.songs[m.currentSongIndex].Title)
				if m.currentSongIndex < len(m.songs)-1 {
					m.mpvBackend.SkipNext()
				} else {
					// Last song is blocked
					favCount, _ := m.cacheManager.GetFavoriteCount()
					if favCount >= m.config.MinFavorites {
						// Queue a favorite and skip to it
						if cmd := m.queueNextFavorite(); cmd != nil {
							cmds = append(cmds, cmd)
						}
						m.mpvBackend.SkipNext()
					} else {
						// No favorites: mute, seek to 2 min remaining to trigger polling
						m.mpvBackend.SetMute(true)
						m.mutedForBlocked = true
						songDur := float64(m.songs[m.currentSongIndex].Duration) / 1000.0
						target := songDur - 120
						if target > 0 {
							m.mpvBackend.SeekRelative(target - m.playbackPos.TimePos)
						}
						logger.Printf("Muted and seeked blocklisted song to trigger polling")
					}
				}
				return m, tea.Batch(cmds...)
			}
			cmds = append(cmds, m.songChangedCmds())
		}
	}

	// Start polling when on last song with ≤2 min remaining
	if !m.pollingNextBlock && m.currentSongIndex >= len(m.songs)-1 && m.currentSong != nil && err == nil {
		songDuration := float64(m.currentSong.Duration) / 1000.0
		timeRemaining := songDuration - m.playbackPos.TimePos
		if timeRemaining <= 120 && timeRemaining > 0 {
			logger.Printf("Last song with %.0fs remaining, starting to poll for next block", timeRemaining)
			m.pollingNextBlock = true
		}
	}

	// MPV stopped on last song - ensure polling is active and start spinner
	if !m.mpvBackend.IsRunning() && !m.mpvBackend.IsPaused() && m.currentSongIndex >= len(m.songs)-1 {
		if !m.pollingNextBlock {
			logger.Printf("MPV stopped on last song, enabling polling")
			m.pollingNextBlock = true
			cmds = append(cmds, m.spinner.Tick)
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

	// Check scrobble eligibility (once per song, when listen threshold is met)
	if !m.scrobbleEligible && m.scrobbler.Enabled() && m.currentSong != nil {
		elapsed := time.Since(m.songStartTime)
		durSecs := float64(m.currentSong.Duration) / 1000.0
		threshold := time.Duration(min(durSecs/2, 240) * float64(time.Second))
		if elapsed >= threshold && durSecs > 30 {
			m.scrobbleEligible = true
		}
	}

	// Toggle scrobble blink state for failure indication (every 1 second)
	if m.scrobbleFlashState == flashBlinkOn || m.scrobbleFlashState == flashBlinkOff {
		if time.Since(m.scrobbleFlashAt) >= flashDuration {
			m.scrobbleFlashState = flashOff
		} else {
			elapsed := time.Since(m.scrobbleFlashAt)
			if int(elapsed.Seconds())%2 == 0 {
				m.scrobbleFlashState = flashBlinkOn
			} else {
				m.scrobbleFlashState = flashBlinkOff
			}
		}
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

	// Update clock display every 5 seconds (like Python's do_poll)
	m.connectedAt = time.Now()

	// If polling for next block AND not in backoff, fetch it
	if m.pollingNextBlock && !m.mpvBackend.IsPaused() && m.connState != connStateDisconnected {
		cmds = append(cmds, m.fetchBlockCmd)
	}

	return m, tea.Batch(cmds...)
}

// handleConnRetryTick fires when the backoff interval expires — time to retry
func (m Model) handleConnRetryTick(msg connRetryTickMsg) (tea.Model, tea.Cmd) {
	if m.connState != connStateDisconnected {
		return m, nil
	}

	logger.Printf("Connection retry #%d triggered", m.consecutiveFailures+1)
	return m, m.fetchBlockCmd
}

// sendNotificationCmd returns a tea.Cmd that sends a desktop notification.
// withImage controls whether to include the album art thumbnail.
func (m *Model) sendNotificationCmd(withImage bool) tea.Cmd {
	song := m.currentSong
	stationName := config.StationNames[m.config.Channel]
	cfg := m.config
	return func() tea.Msg {
		api.SendDesktopNotification(song, stationName, cfg, withImage)
		return notificationSentMsg{}
	}
}

// handleImageLoaded handles image loading completion
func (m Model) handleImageLoaded(msg imageLoadedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}
	if msg.err != nil {
		logger.Printf("Image load error: %v", msg.err)
		// Send notification without image if not already sent
		if m.config.NotificationsEnabled && !m.notifSentForSong {
			return m, m.sendNotificationCmd(false)
		}
		return m, nil
	}

	// Decode image (jpeg/png decoders registered via blank imports)
	img, format, err := image.Decode(bytes.NewReader(msg.imageData))
	if err != nil {
		logger.Printf("Image decode error: %v", err)
		// Send notification without image if not already sent
		if m.config.NotificationsEnabled && !m.notifSentForSong {
			return m, m.sendNotificationCmd(false)
		}
		return m, nil
	}

	logger.Printf("Image decoded: %s, format=%s, bounds=%v, dataLen=%d", m.currentSong.Title, format, img.Bounds(), len(msg.imageData))

	// Copy album art to file if configured
	if m.config.CopyAlbumArt && m.config.AlbumArtPath != "" {
		if err := os.WriteFile(m.config.AlbumArtPath, msg.imageData, 0644); err != nil {
			logger.Printf("Warning: failed to copy album art to %s: %v", m.config.AlbumArtPath, err)
		}
	}

	// Save album art for desktop notifications
	if m.config.NotificationsEnabled && m.config.NotificationsShowArt {
		api.SaveNotifyArt(msg.imageData)
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
		Protocol(termimg.Auto)

	rendered, err := tiImg.Render()
	if err != nil {
		logger.Printf("Album art render error: %v", err)
		// Send notification without image if not already sent
		if m.config.NotificationsEnabled && !m.notifSentForSong {
			return m, m.sendNotificationCmd(false)
		}
		return m, nil
	}

	logger.Printf("Album art loaded for: %s (len=%d)", m.currentSong.Title, len(rendered))
	m.albumArtStr = rendered
	m.albumArtLoaded = true

	// Send desktop notification with album art
	var cmds []tea.Cmd
	if m.config.NotificationsEnabled && !m.notifSentForSong {
		cmds = append(cmds, m.sendNotificationCmd(true))
	}
	cmds = append(cmds, renderAlbumArtAfterDelay())
	return m, tea.Batch(cmds...)
}

// handleLyricsFetched handles lyrics fetch completion.
// Synced lyrics always auto-update.  Plain lyrics go to a pending slot
// so the user can finish reading before pressing 'u' to update.
func (m Model) handleLyricsFetched(msg lyricsFetchedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}

	// Synced lyrics always go straight to display (time-based)
	m.syncedLyrics = msg.synced

	if msg.err != nil {
		m.pendingLyrics = "Lyrics not found"
		m.pendingEventID = msg.eventID
		// Auto-display if nothing showing yet
		if m.lyrics == "" {
			m.lyrics = m.pendingLyrics
		}
		if m.bottomViewMode == ViewLyrics {
			m.updateBottomView()
		}
		return m, nil
	}

	// Store plain lyrics in pending
	m.pendingLyrics = msg.plain
	m.pendingEventID = msg.eventID

	// Auto-display if nothing showing yet
	if m.lyrics == "" {
		m.lyrics = m.pendingLyrics
	}

	if m.bottomViewMode == ViewLyrics || m.bottomViewMode == ViewSyncedLyrics {
		m.updateBottomView()
	}

	return m, nil
}

// handleArtistFetched handles artist info fetch completion.
// Artist info goes to a pending slot so the user can finish reading
// before pressing 'u' to update.
func (m Model) handleArtistFetched(msg artistFetchedMsg) (tea.Model, tea.Cmd) {
	// Discard stale result from a previous song
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}

	m.artistStatus = ""

	if msg.err != nil {
		m.pendingArtistInfo = &models.ArtistInfo{Bio: "Artist info not found"}
		m.pendingEventID = msg.eventID
		// Auto-display if nothing showing yet
		if m.artistInfo == nil {
			m.artistInfo = m.pendingArtistInfo
		}
		if m.bottomViewMode == ViewArtist {
			m.updateBottomView()
		}
		return m, nil
	}

	// Store in pending
	m.pendingArtistInfo = msg.info
	m.pendingEventID = msg.eventID

	// Always update the artist cache (for next time this artist plays)
	if msg.info != nil && m.currentSong != nil {
		if m.artistCache == nil {
			m.artistCache = make(map[string]*models.ArtistInfo)
		}
		m.artistCache[strings.ToLower(m.currentSong.Artist)] = msg.info
	}

	// Auto-display if nothing showing yet
	if m.artistInfo == nil {
		m.artistInfo = m.pendingArtistInfo
	}

	if m.bottomViewMode == ViewArtist {
		m.updateBottomView()
	}

	// Start downloading artist thumbnail if available
	var cmd tea.Cmd
	if msg.info != nil && msg.info.ThumbnailURL != "" {
		cmd = m.loadArtistImageCmd(msg.info.ThumbnailURL)
	}

	return m, cmd
}

// prunePlaylist removes old songs from previous blocks, keeping up to 3 before
// the currently playing song for prev-song functionality.
// Only prunes songs that belong to an older block than the current song.
func (m *Model) prunePlaylist() {
	if m.currentSongIndex < 0 || m.currentSongIndex >= len(m.songs) {
		return
	}

	currentBlockID := m.songs[m.currentSongIndex].BlockID

	// Find how many old-block songs precede the current song
	keepStart := m.currentSongIndex - 3
	if keepStart <= 0 {
		return
	}

	// Only prune if the song at keepStart-1 is from an older block
	// Walk forward from 0 to find the prune boundary: stop at the first
	// song that is either from the current block or within 3 of current
	pruneEnd := 0
	for i := 0; i < keepStart; i++ {
		if m.songs[i].BlockID >= currentBlockID {
			break
		}
		pruneEnd = i + 1
	}

	if pruneEnd <= 0 {
		return
	}

	m.songs = m.songs[pruneEnd:]
	m.currentSongIndex -= pruneEnd
	m.playlistStartIdx -= pruneEnd
	logger.Printf("Pruned %d old-block songs, currentIdx=%d, playlistStartIdx=%d", pruneEnd, m.currentSongIndex, m.playlistStartIdx)
}

// updatePlaylist updates the playlist table
func (m *Model) updatePlaylist() {
	if len(m.songs) == 0 {
		return
	}

	rows := make([]table.Row, len(m.songs))
	for i, song := range m.songs {
		prefix := ""
		if song.EventID == m.downloadingFav {
			prefix = "⏳ "
		} else if m.cacheManager.IsFavorite(song) {
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

// hasPendingUpdate returns true if there is pending data for the current
// song that differs from what is currently displayed.
func (m *Model) hasPendingUpdate() bool {
	if m.currentSong == nil || m.pendingEventID != m.currentSong.EventID {
		return false
	}
	if m.bottomViewMode == ViewLyrics {
		return m.pendingLyrics != "" && m.pendingLyrics != m.lyrics
	}
	if m.bottomViewMode == ViewArtist {
		hasInfo := m.pendingArtistInfo != nil && m.pendingArtistInfo != m.artistInfo
		hasArt := m.pendingArtistArtLoaded
		return hasInfo || hasArt
	}
	return false
}

// applyPendingUpdate copies pending data into the displayed fields and
// refreshes the current view.  Called when the user presses 'u'.
func (m *Model) applyPendingUpdate() {
	if m.currentSong == nil || m.pendingEventID != m.currentSong.EventID {
		return
	}
	if m.pendingLyrics != "" {
		m.lyrics = m.pendingLyrics
	}
	if m.pendingArtistInfo != nil {
		m.artistInfo = m.pendingArtistInfo
		// Cache by artist name
		if m.artistCache == nil {
			m.artistCache = make(map[string]*models.ArtistInfo)
		}
		m.artistCache[strings.ToLower(m.currentSong.Artist)] = m.artistInfo
	}
	if m.pendingArtistArtLoaded {
		m.artistArtStr = m.pendingArtistArtStr
		m.artistArtLoaded = true
		m.artistArtEventID = m.pendingEventID
		m.artistArtWidth = m.pendingArtistArtWidth
		m.artistArtHeight = m.pendingArtistArtHeight
		// Clear pending art after applying
		m.pendingArtistArtStr = ""
		m.pendingArtistArtLoaded = false
	}
	m.updateBottomView()
}

// updateBottomView updates the viewport content based on current mode
func (m *Model) updateBottomView() {
	var content string

	// Reset viewport width for non-artist views
	if m.bottomViewMode != ViewArtist && m.width > 0 {
		m.viewport.SetWidth(m.width)
	}

	switch m.bottomViewMode {
	case ViewLyrics:
		if m.lyrics == "" {
			content = "  Loading lyrics..."
		} else {
			content = indentLines(m.lyrics, "  ") + strings.Repeat("\n", 10)
			if m.hasPendingUpdate() {
				content += "\n  \x1b[3m(press 'u' to update)\x1b[0m"
			}
		}

	case ViewSyncedLyrics:
		if len(m.syncedLyrics) == 0 {
			content = "  No synced lyrics available\n  Synced lyrics require duration matching ±2s. Radio edits reduce match chances."
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

			cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Cursor)).Bold(true)
			var lines []string
			for i := startIdx; i < endIdx; i++ {
				if i == currentLineIdx {
					lines = append(lines, cursorStyle.Render("▶ "+m.syncedLyrics[i].Content))
				} else {
					lines = append(lines, "  "+m.syncedLyrics[i].Content)
				}
			}
			// Pad with many newlines so viewport scrolls to blank space at bottom
			content = strings.Join(lines, "\n") + strings.Repeat("\n", 10)
		}

	case ViewArtist:
		// Narrow viewport width when artist image is displayed beside it
		if m.artistArtLoaded && m.artistArtStr != "" {
			imgGap := m.artistArtWidth + 5 // image width + margin
			newWidth := m.width - imgGap
			if newWidth < 30 {
				newWidth = 30
			}
			m.viewport.SetWidth(newWidth)
		} else {
			m.viewport.SetWidth(m.width)
		}

		if m.artistInfo == nil {
			// Indent when no artist image (viewport not right-shifted)
			indent := ""
			if !(m.artistArtLoaded && m.artistArtStr != "") {
				indent = "  "
			}
			if m.artistStatus != "" {
				content = indent + m.spinner.View() + " " + m.artistStatus
			} else {
				content = indent + "Loading artist info..."
			}
		} else {
			// Indent when no artist image (viewport not right-shifted)
			indent := ""
			if !(m.artistArtLoaded && m.artistArtStr != "") {
				indent = "  "
			}
			var lines []string
			if m.artistInfo.Bio != "" {
				lines = append(lines, indent+m.artistInfo.Bio)
				if m.artistInfo.BioSource != "" {
					lines = append(lines, indent+"Source: "+m.artistInfo.BioSource)
				}
			} else {
				lines = append(lines, indent+"No biography available.")
			}
			if m.artistInfo.Discography != "" {
				lines = append(lines, "")
				lines = append(lines, indent+"Studio Albums:")
				discoLines := strings.Split(m.artistInfo.Discography, "\n")
				for _, line := range discoLines {
					lines = append(lines, indent+"  "+line)
				}
				if m.artistInfo.DiscoSource != "" {
					lines = append(lines, indent+"Source: "+m.artistInfo.DiscoSource)
				}
			}
			if m.artistInfo.ThumbSource != "" {
				lines = append(lines, "")
				lines = append(lines, indent+"thumb: "+m.artistInfo.ThumbSource)
			}
			if len(m.artistInfo.GalleryURLs) > 0 {
				lines = append(lines, "")
				count := len(m.artistInfo.GalleryURLs)
				lines = append(lines, indent+fmt.Sprintf("(press 'i' for %d additional artist images)", count))
			}
			if m.hasPendingUpdate() {
				lines = append(lines, "")
				lines = append(lines, "(press 'u' to update)")
			}
			// Pad with many newlines so viewport scrolls to blank space at bottom
			content = strings.Join(lines, "\n") + strings.Repeat("\n", 10)
		}

	case ViewPlaylist:
		// Playlist is rendered separately via table component
		return

	case ViewVisualizer:
		// Visualizer is rendered directly in View()
		return

	case ViewOff:
		content = ""
	}

	// Word-wrap content to viewport width before setting
	viewWidth := m.viewport.Width()
	if content != "" && viewWidth > 0 {
		content = ansi.Wordwrap(content, viewWidth, "")
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

	// Scrobble previous song if it met the listen threshold
	prevSong := m.currentSong
	scrobbleStartTime := m.songStartTime
	scrobblePrev := prevSong != nil && m.scrobbleEligible && m.scrobbler.Enabled()

	m.currentSong = m.songs[m.currentSongIndex]
	m.playlistWidget.SetCursor(m.currentSongIndex)

	// Reset scrobble state for new song
	m.songStartTime = time.Now()
	m.scrobbleEligible = false

	// Send now-playing notification to scrobble services
	if m.scrobbler.Enabled() && m.currentSong != nil {
		song := *m.currentSong
		go m.scrobbler.SendNowPlaying(context.Background(), song)
	}

	// Clear synced lyrics (time-based, always refresh)
	m.syncedLyrics = nil
	m.albumArtStr = ""
	m.albumArtLoaded = false
	m.notifSentForSong = false
	// Don't clear m.artistArtStr — keep displayed artist thumbnail
	m.playbackPos = mpv.PlaybackPosition{}

	// Clear pending data for the new song
	m.pendingLyrics = ""
	m.pendingArtistInfo = nil
	m.pendingEventID = 0
	m.pendingArtistArtStr = ""
	m.pendingArtistArtLoaded = false
	m.pendingArtistArtWidth = 0
	m.pendingArtistArtHeight = 0

	// Only hold displayed content for the view the user is currently in.
	// If we're not in that view, clear stale data so the view shows
	// current song info when the user next enters it.
	if m.bottomViewMode != ViewLyrics {
		m.lyrics = ""
	}
	if m.bottomViewMode != ViewArtist {
		m.artistInfo = nil
		m.artistArtStr = ""
		m.artistArtLoaded = false
		m.artistArtEventID = 0
	}

	// Check artist cache — put in pending, not display
	m.artistStatus = ""
	artistKey := strings.ToLower(m.currentSong.Artist)
	if m.artistCache != nil {
		if cached, ok := m.artistCache[artistKey]; ok {
			m.pendingArtistInfo = cached
			m.pendingEventID = m.currentSong.EventID
		}
	}

	// Restore cached artist thumbnail to pending
	if m.artistArtCache != nil {
		if cached, ok := m.artistArtCache[artistKey]; ok {
			m.pendingArtistArtStr = cached.rendered
			m.pendingArtistArtLoaded = true
			m.pendingArtistArtWidth = cached.width
			m.pendingArtistArtHeight = cached.height
			if m.pendingEventID == 0 && m.currentSong != nil {
				m.pendingEventID = m.currentSong.EventID
			}
		}
	}

	// Prune old-block songs, then update playlist and bottom view
	m.prunePlaylist()
	m.updatePlaylist()
	m.updateBottomView()

	// Update visualizer seed on song change
	if m.vis != nil && m.bottomViewMode == ViewVisualizer {
		m.vis.SetSeed(uint64(m.currentSong.EventID))
		// Show song info overlay in fullscreen
		if m.visFullscreen {
			showInfo := m.config.Visualizer.ShowInfo
			if showInfo == "fade" || showInfo == "on" {
				m.visInfoVisible = true
				m.visInfoShownAt = time.Now()
			}
		}
	}

	var cmds []tea.Cmd

	// Scrobble previous song if eligible
	if scrobblePrev {
		cmds = append(cmds, scrobbleCmd(m.scrobbler, *prevSong, scrobbleStartTime))
	}

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
		// No art URL — send notification without image if enabled
		if m.config.NotificationsEnabled && !m.notifSentForSong {
			cmds = append(cmds, m.sendNotificationCmd(false))
		}
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
	favCount, _ := m.cacheManager.GetFavoriteCount()
	if favCount < m.config.MinFavorites {
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
		return setStatus(m, "No favorites available...", false)
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
	song.IsFromFavorite = true

	// Append to MPV playlist
	if err := m.mpvBackend.AppendToPlaylist([]string{song.GaplessURL}); err != nil {
		logger.Printf("Failed to append favorite to playlist: %v", err)
		return setStatus(m, "Error queueing favorite", true)
	}

	// Append to song list (don't change currentSongIndex — let natural
	// transition detection in handleProgressTick pick it up when MPV
	// actually starts playing the new track)
	m.songs = append(m.songs, song)

	logger.Printf("Appended favorite to playlist: %s", song.Title)

	// Update playlist display to show the queued song
	m.updatePlaylist()
	return setStatus(m, "★ Queued favorite", false)
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

// fetchArtistCmd fetches artist info with per-item fallback:
//
//	bio:        tadb > discogs > wikipedia
//	discography: musicbrainz > wikipedia
//	thumb:      discogs > tadb > wikipedia
//	gallery:    discogs > tadb
//
// TADB and MusicBrainz run in parallel first (fast, no ID resolution).
// Discogs runs whenever auth is configured (for thumb/gallery priority),
// or if bio is still missing.
// Wikipedia is the final fallback for bio/thumb/discography.
func (m Model) fetchArtistCmd() tea.Cmd {
	if m.currentSong == nil {
		return nil
	}

	// Check cache first
	if m.artistCache != nil {
		if cached, ok := m.artistCache[strings.ToLower(m.currentSong.Artist)]; ok {
			return func() tea.Msg {
				return artistFetchedMsg{eventID: m.currentSong.EventID, info: cached}
			}
		}
	}

	song := m.currentSong // capture for closure
	discogsClient := m.discogsClient
	mbClient := m.musicbrainzClient
	tadbClient := m.theaudiodbClient
	wikiClient := m.wikipediaClient

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		info := &models.ArtistInfo{}

		// --- Phase 1: TADB + MusicBrainz in parallel ---
		logger.Printf("Artist fetch: phase 1 (TADB + MB) for %s", song.Artist)

		type tadbResult struct {
			artist *api.TADBArtist
			err    error
		}
		type mbResult struct {
			albums []api.MBAlbum
			err    error
		}

		tadbCh := make(chan tadbResult, 1)
		mbCh := make(chan mbResult, 1)

		go func() {
			a, e := tadbClient.SearchArtist(ctx, song.Artist)
			tadbCh <- tadbResult{a, e}
		}()
		go func() {
			a, e := mbClient.GetDiscography(ctx, song.Artist, song.Album)
			mbCh <- mbResult{a, e}
		}()

		tadb := <-tadbCh
		mb := <-mbCh

		// Process TADB — bio, thumb, gallery
		if tadb.err != nil {
			logger.Printf("TheAudioDB error for %s: %v", song.Artist, tadb.err)
		}
		if tadb.artist != nil {
			if tadb.artist.Bio != "" {
				info.Bio = tadb.artist.Bio
				info.BioSource = "theaudiodb"
			}
			if tadb.artist.Thumb != "" {
				info.ThumbnailURL = tadb.artist.Thumb
				info.ThumbSource = "theaudiodb"
			}
			if len(tadb.artist.FanArts) > 0 {
				info.GalleryURLs = tadb.artist.FanArts
				info.GallerySource = "theaudiodb"
			}
		}

		// Process MusicBrainz — discography
		if mb.err != nil {
			logger.Printf("MusicBrainz error for %s: %v", song.Artist, mb.err)
		}
		if len(mb.albums) > 0 {
			var discoBuilder strings.Builder
			for _, a := range mb.albums {
				if a.Year != "" {
					discoBuilder.WriteString(fmt.Sprintf("%s (%s)\n", a.Title, a.Year))
				} else {
					discoBuilder.WriteString(a.Title + "\n")
				}
			}
			info.Discography = strings.TrimSpace(discoBuilder.String())
			info.DiscoSource = "musicbrainz"
		}

		// --- Phase 2: Discogs ---
		// Always run if auth configured (for thumb/gallery: discogs > tadb),
		// or if bio still needed (bio: tadb > discogs > wikipedia)
		if info.Bio == "" || discogsClient.HasAuth() {
			logger.Printf("Artist fetch: phase 2 (Discogs) for %s", song.Artist)
			discogsArtist, err := discogsClient.SearchArtist(ctx, song.Artist)
			if err != nil {
				logger.Printf("Discogs error for %s: %v", song.Artist, err)
			}
			if discogsArtist != nil {
				// Bio: only fill if TADB didn't provide one
				if info.Bio == "" && discogsArtist.Profile != "" {
					info.Bio = discogsArtist.Profile
					info.BioSource = "discogs"
				}
				// Thumb: discogs > tadb (always override if discogs has image)
				if discogsArtist.PrimaryImage != "" {
					info.ThumbnailURL = discogsArtist.PrimaryImage
					info.ThumbSource = "discogs"
				}
				// Gallery: discogs > tadb (always override if discogs has images)
				if len(discogsArtist.GalleryURLs) > 0 {
					info.GalleryURLs = discogsArtist.GalleryURLs
					info.GallerySource = "discogs"
				}
			}
		}

		// --- Phase 3: Wikipedia (final fallback for remaining fields) ---
		if info.Bio == "" || info.ThumbnailURL == "" || info.Discography == "" {
			logger.Printf("Artist fetch: phase 3 (Wikipedia) for %s", song.Artist)
			wikiInfo, err := wikiClient.FindArtist(ctx, song.Artist)
			if err != nil {
				logger.Printf("Wikipedia error for %s: %v", song.Artist, err)
			}
			if wikiInfo != nil {
				if info.Bio == "" && wikiInfo.Summary != "" {
					info.Bio = wikiInfo.Summary
					info.BioSource = "wikipedia"
					info.PageURL = wikiInfo.PageURL
				}
				if info.ThumbnailURL == "" && wikiInfo.ThumbnailURL != "" {
					info.ThumbnailURL = wikiInfo.ThumbnailURL
					info.ThumbSource = "wikipedia"
				}
				if info.Discography == "" && wikiInfo.Discography != "" {
					info.Discography = wikiInfo.Discography
					info.DiscoSource = "wikipedia"
				}
			}
		}

		// --- Final fallback ---
		if info.Bio == "" {
			info.Bio = "No biography found."
		}

		hasData := info.BioSource != "" || info.DiscoSource != "" || info.ThumbSource != ""
		if !hasData {
			logger.Printf("No artist info found for: %s", song.Artist)
			return artistFetchedMsg{eventID: song.EventID, err: fmt.Errorf("artist not found")}
		}

		logger.Printf("Artist info for %s: bio=%s disco=%s thumb=%s gallery=%s",
			song.Artist, info.BioSource, info.DiscoSource, info.ThumbSource, info.GallerySource)
		return artistFetchedMsg{eventID: song.EventID, info: info}
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

// loadArtistImageCmd fetches the artist thumbnail image from a URL
func (m Model) loadArtistImageCmd(url string) tea.Cmd {
	if m.currentSong == nil {
		return nil
	}
	eventID := m.currentSong.EventID
	artistName := m.currentSong.Artist
	return func() tea.Msg {
		logger.Printf("Fetching artist thumbnail: %s for %s", url, artistName)
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return artistImageLoadedMsg{eventID: eventID, err: err}
		}
		req.Header.Set("User-Agent", "rptui/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return artistImageLoadedMsg{eventID: eventID, err: err}
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return artistImageLoadedMsg{eventID: eventID, err: err}
		}
		logger.Printf("Artist thumbnail fetched: %s, %d bytes", artistName, len(data))
		return artistImageLoadedMsg{eventID: eventID, imageData: data}
	}
}

// handleArtistImageLoaded handles artist thumbnail image loading completion
func (m Model) handleArtistImageLoaded(msg artistImageLoadedMsg) (tea.Model, tea.Cmd) {
	if m.currentSong == nil || msg.eventID != m.currentSong.EventID {
		return m, nil
	}
	if msg.err != nil {
		logger.Printf("Artist image load error: %v", msg.err)
		return m, nil
	}

	img, format, err := image.Decode(bytes.NewReader(msg.imageData))
	if err != nil {
		logger.Printf("Artist image decode error: %v", err)
		return m, nil
	}

	logger.Printf("Artist image decoded: format=%s, bounds=%v", format, img.Bounds())

	termimg.ClearResizeCache()

	// Set display width in columns, calculate height from source aspect ratio
	// accounting for terminal cell ratio (cellRatio = cellHeight/cellWidth, ~2.0)
	const displayWidth = 30
	imgBounds := img.Bounds()
	imgW := float64(imgBounds.Dx())
	imgH := float64(imgBounds.Dy())
	// height_rows = width_cols * (imgH/imgW) / cellRatio
	displayHeight := int(float64(displayWidth) * (imgH / imgW) / m.cellRatio)
	if displayHeight < 4 {
		displayHeight = 4
	}
	if displayHeight > 20 {
		displayHeight = 20
	}

	logger.Printf("Artist thumbnail sizing: src=%dx%d, display=%dx%d cells, cellRatio=%.2f",
		imgBounds.Dx(), imgBounds.Dy(), displayWidth, displayHeight, m.cellRatio)

	tiImg := termimg.New(img).
		Size(displayWidth, displayHeight).
		Scale(termimg.ScaleFit).
		Protocol(termimg.Auto).
		ZIndex(1)

	rendered, err := tiImg.Render()
	if err != nil {
		logger.Printf("Artist thumbnail render error: %v", err)
		return m, nil
	}

	logger.Printf("Artist thumbnail loaded (len=%d)", len(rendered))

	// Always cache by artist name
	if m.currentSong != nil {
		if m.artistArtCache == nil {
			m.artistArtCache = make(map[string]artistArtCacheEntry)
		}
		m.artistArtCache[strings.ToLower(m.currentSong.Artist)] = artistArtCacheEntry{
			rendered: rendered,
			width:    displayWidth,
			height:   displayHeight,
		}
	}

	// Route to pending or display based on what's currently shown
	if m.artistArtStr != "" && m.artistArtEventID != msg.eventID {
		// Display already has art for a different song — stash in pending
		m.pendingArtistArtStr = rendered
		m.pendingArtistArtLoaded = true
		m.pendingArtistArtWidth = displayWidth
		m.pendingArtistArtHeight = displayHeight
		m.pendingEventID = msg.eventID
		if m.bottomViewMode == ViewArtist {
			m.updateBottomView() // show "press 'u' to update" hint
		}
		return m, nil
	}

	// Nothing displayed yet (or same event) — auto-show
	m.artistArtStr = rendered
	m.artistArtLoaded = true
	m.artistArtEventID = msg.eventID
	m.artistArtWidth = displayWidth
	m.artistArtHeight = displayHeight

	// Update the bottom view (viewport width changes when image loads)
	if m.bottomViewMode == ViewArtist {
		m.updateBottomView()
	}

	return m, renderArtistArtAfterDelay()
}

// renderArtistArtCmd delegates to renderImagesCmd which draws all images together.
func (m Model) renderArtistArtCmd() tea.Cmd {
	return m.renderImagesCmd()
}

// altView wraps content in a tea.View with AltScreen enabled
func (m Model) altView(s string) tea.View {
	v := tea.NewView(s)
	v.AltScreen = true
	v.BackgroundColor = lipgloss.Color(m.theme.Background)
	v.ForegroundColor = lipgloss.Color(m.theme.Foreground)
	return v
}

// View renders the TUI
func (m Model) View() tea.View {
	if m.err != nil {
		return m.altView(fmt.Sprintf("Error: %v\n\nPress q to quit", m.err))
	}

	if !m.initialized {
		return m.altView("Loading...\n\nPress q to quit")
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
		case ModalGallery:
			if m.galleryModal != nil {
				modalView = m.galleryModal.View()
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

			return m.altView(b.String())
		}
	}

	// Fullscreen visualizer — replaces entire screen
	if m.visFullscreen && m.bottomViewMode == ViewVisualizer && m.vis != nil {
		return m.renderFullscreenVisualizer()
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
		m.statusMsg,
		m.statusIsError,
		m.connErrorMsg,
		m.config.MinFavorites,
		len(m.favoritesQueue),
	)

	var b strings.Builder
	b.WriteString(header + "\n\n")

	// Now-playing info
	b.WriteString(nowPlayingView + "\n\n")

	// Show spinner animation when waiting for new songs AND MPV has stopped (Python line 3242-3244)
	// Only show when truly at end of content, not while still playing and polling for next block
	if m.pollingNextBlock && !m.mpvBackend.IsRunning() {
		b.WriteString(m.styles.MutedStyle.Render("Ahead of livestream. Awaiting new songs"+m.spinner.View()+" ") + "\n")
	}

	// The bottom section should fill the remaining space except for the footer
	currentHeight := lipgloss.Height(b.String())
	remainingHeight := m.height - currentHeight - 1 // 2 footer lines

	// Sync viewport height to actual available space so scroll math is correct
	if m.bottomViewMode != ViewPlaylist && m.bottomViewMode != ViewOff && remainingHeight > 0 {
		m.viewport.SetHeight(remainingHeight)
	}

	// 3. Bottom Section (Playlist, Visualizer, or other)
	// Visualizer renders now that we know the available height
	var bottomSection string
	if m.bottomViewMode == ViewPlaylist {
		bottomSection = m.playlistWidget.View()
	} else if m.bottomViewMode == ViewVisualizer && m.vis != nil {
		m.vis.SetRows(max(3, remainingHeight))
		if m.vis.AudioReady() {
			bottomSection = m.vis.Render(m.width)
		} else {
			// Show loading message while audio tap connects
			modeName := m.vis.ModeName()
			source := m.vis.AudioSource()
			lines := []string{
				"",
				fmt.Sprintf("Loading %s visualization...", modeName),
				fmt.Sprintf("Connecting to %s audio...", source),
				"",
			}
			bottomSection = strings.Join(lines, "\n")
		}
	} else if m.bottomViewMode != ViewOff {
		viewportContent := m.viewport.View()
		// Offset viewport to the right when artist image is beside it
		if m.bottomViewMode == ViewArtist && m.artistArtLoaded && m.artistArtStr != "" {
			leftPad := strings.Repeat(" ", m.artistArtWidth+5)
			vpLines := strings.Split(viewportContent, "\n")
			for i, line := range vpLines {
				vpLines[i] = leftPad + line
			}
			viewportContent = strings.Join(vpLines, "\n")
		}
		bottomSection = viewportContent
	}

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

	// 4. Footer
	if m.scrobbleFlashState != flashOff && time.Since(m.scrobbleFlashAt) >= flashDuration {
		m.scrobbleFlashState = flashOff
	}
	m.footerWidget.SetFlashState(m.scrobbleFlashState)
	footer := m.footerWidget.View()

	b.WriteString(footer)

	return m.altView(b.String())
}

// renderImagesCmd returns a tea.Cmd that sends all terminal images (album art
// and artist thumbnail) via tea.Raw. Both images must be drawn in one call
// because ClearAll removes all kitty placements.
func (m Model) renderImagesCmd() tea.Cmd {
	if m.activeModal != ModalNone {
		return nil
	}

	// Suppress all images when in fullscreen visualizer
	if m.visFullscreen && m.bottomViewMode == ViewVisualizer {
		return nil
	}

	hasAlbumArt := m.config.ShowAlbumArt && m.albumArtLoaded && m.albumArtStr != ""
	hasArtistArt := m.artistArtLoaded && m.artistArtStr != "" && m.bottomViewMode == ViewArtist

	if !hasAlbumArt && !hasArtistArt {
		return nil
	}

	clearStr := termimg.ClearAllString()
	raw := clearStr

	if hasAlbumArt {
		artHeight := 16
		artWidth := int(float64(artHeight) * m.cellRatio)
		if artWidth < 10 {
			artWidth = 10
		}
		artCol := m.width - artWidth - 2
		if artCol < 1 {
			artCol = 1
		}
		raw += fmt.Sprintf("\x1b[s\x1b[3;%dH%s\x1b[u", artCol, m.albumArtStr)
	}

	if hasArtistArt {
		// Bottom section starts after: header(1) + gap(2) + nowPlaying(15 lines) + gap(2) = row 20
		raw += fmt.Sprintf("\x1b[s\x1b[%d;%dH%s\x1b[u", 20, 2, m.artistArtStr)
	}

	return tea.Raw(raw)
}

// renderAlbumArtCmd is kept as an alias for renderImagesCmd for backward compatibility
// with all the existing renderAlbumArtAfterDelay() call sites.
func (m Model) renderAlbumArtCmd() tea.Cmd {
	return m.renderImagesCmd()
}

// renderFullscreenVisualizer renders the visualizer filling the entire terminal,
// with an optional song info overlay.
func (m Model) renderFullscreenVisualizer() tea.View {
	if m.vis == nil {
		return m.altView("Visualizer not initialized")
	}

	// Reserve top rows for song info overlay when visible
	infoReserve := 0
	if m.visInfoVisible && m.currentSong != nil {
		infoReserve = 6 // title + artist + album + station + 2 padding
	}

	rows := max(3, m.height-infoReserve)
	m.vis.SetRows(rows)

	var b strings.Builder

	if !m.vis.AudioReady() {
		// Show loading message while audio tap connects
		modeName := m.vis.ModeName()
		source := m.vis.AudioSource()
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Loading %s visualization...\n", modeName))
		b.WriteString(fmt.Sprintf("Connecting to %s audio...\n", source))
		return m.altView(b.String())
	}

	vizContent := m.vis.Render(m.width)

	// Song info overlay at top with padding
	if m.visInfoVisible && m.currentSong != nil {
		infoLines := m.buildVisInfoOverlay()
		padLeft := max(2, (m.width-lipgloss.Width(infoLines[0]))/2)
		leftPad := strings.Repeat(" ", padLeft)

		// Two blank lines of padding above song info
		b.WriteString("\n\n")
		for _, line := range infoLines {
			b.WriteString(leftPad + line + "\n")
		}
	}

	b.WriteString(vizContent)

	return m.altView(b.String())
}

// buildVisInfoOverlay creates styled lines for the song info overlay.
func (m Model) buildVisInfoOverlay() []string {
	song := m.currentSong
	if song == nil {
		return nil
	}

	// Build info lines
	var lines []string

	// Title
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Foreground)).
		Bold(true)
	lines = append(lines, titleStyle.Render(song.Title))

	// Artist
	artistStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Accent))
	lines = append(lines, artistStyle.Render(song.Artist))

	// Album (year)
	if song.Album != "" {
		albumStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(m.theme.Muted))
		album := song.Album
		if song.Year != "" {
			album = fmt.Sprintf("%s (%s)", album, song.Year)
		}
		lines = append(lines, albumStyle.Render(album))
	}

	// Station info
	stationStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.theme.Cursor))
	stationName := config.StationNames[m.config.Channel]
	lines = append(lines, stationStyle.Render(stationName))

	return lines
}

// getFavoriteCount returns the number of favorites
func (m Model) getFavoriteCount() int {
	count, _ := m.cacheManager.GetFavoriteCount()
	return count
}

func (m Model) getSkipsAvailable() int {
	return max(0, len(m.songs)-1-m.currentSongIndex)
}

func (m Model) getPrevAvailable() int {
	return m.currentSongIndex
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
