// Package tui provides the terminal user interface
package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
	"github.com/blacktop/go-termimg"
	"github.com/charmbracelet/x/ansi"
	"github.com/pdfrg/rptui/internal/api"
	"github.com/pdfrg/rptui/internal/cache"
	"github.com/pdfrg/rptui/internal/config"
	"github.com/pdfrg/rptui/internal/loginit"
	"github.com/pdfrg/rptui/internal/models"
	"github.com/pdfrg/rptui/internal/mpv"
	"github.com/pdfrg/rptui/internal/tui/modals"
	"github.com/pdfrg/rptui/internal/tui/visualizer"
	"github.com/pdfrg/rptui/internal/tui/widgets"
)

// Logger for TUI
var logger *log.Logger

func init() {
	logger = loginit.InitLogger("[TUI] ")
	api.SetDiscogsLogger(logger)
	visualizer.SetLogger(logger)
	visualizer.SetAudioLogger(logger)
	visualizer.SetFFTLogger(logger)
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
	ViewComments
	ViewVisualizer
	ViewOff
	ViewModeCount
)

var bottomViewNames = []string{
	"Playlist",
	"Lyrics",
	"Synced Lyrics",
	"Artist",
	"Comments",
	"Visualizer",
	"Off",
}

// Scrobble flash states
const (
	flashOff      = 0
	flashSolid    = 1 // success — accent for 5s
	flashBlinkOn  = 2 // failure — accent visible
	flashBlinkOff = 3 // failure — muted
	flashDuration = 5 * time.Second
)

// Modal types
const (
	ModalNone = iota
	ModalOptions
	ModalSkipWarning
	ModalFavorites
	ModalGallery
	ModalStationWarning
	ModalRating
	ModalNetworkTransition
	ModalSleepTimer
)

// Network transition variants
const (
	NetworkGoingOffline = iota
	NetworkGoingOnline
)

// Layout mode constants
const (
	LayoutLarge = iota
	LayoutMedium
	LayoutCompact
	LayoutNarrow
)

var layoutNames = map[int]string{
	LayoutLarge:   "large",
	LayoutMedium:  "medium",
	LayoutCompact: "compact",
	LayoutNarrow:  "narrow",
}

// Package-level state for terminal size prompt (survives across View() calls)
// These are set during the first render when terminal size is known
var (
	layoutPromptActive bool
	layoutCheckDone    bool
	layoutPromptWidth  int
	layoutPromptHeight int
	layoutPromptLayout string
)

// ResetLayoutPrompt resets the package-level layout prompt state
func ResetLayoutPrompt() {
	layoutPromptActive = false
	layoutCheckDone = false
	layoutPromptWidth = 0
	layoutPromptHeight = 0
	layoutPromptLayout = ""
}

// Layout requirements (width x height in terminal cells)
type layoutRequirements struct {
	minCols int
	minRows int
	recCols int
}

var layoutReqs = map[string]layoutRequirements{
	"large":   {minCols: 75, minRows: 32, recCols: 113},
	"medium":  {minCols: 75, minRows: 21, recCols: 113},
	"compact": {minCols: 36, minRows: 18, recCols: 36},
	"narrow":  {minCols: 36, minRows: 35, recCols: 36},
}

// checkTerminalSize checks if terminal is large enough for the given layout
func checkTerminalSize(width, height int, layout string) (fits bool, suboptimal bool, warning string, options []string) {
	req, ok := layoutReqs[layout]
	if !ok {
		return true, false, "", nil
	}

	// Check minimum requirements
	if width < req.minCols || height < req.minRows {
		return false, false, "terminal too narrow or too short", nil
	}

	// Check suboptimal zone (large/medium only)
	if (layout == "large" || layout == "medium") && width >= req.minCols && width < req.recCols {
		return false, true, "Some hotkeys and scrobble indicators (if configured) may not be visible at this terminal width.", nil
	}

	// Fits well
	return true, false, "", nil
}

// getFittingLayouts returns all layouts that fit the given terminal size
func getFittingLayouts(width, height int) []string {
	var fitting []string
	for layout, req := range layoutReqs {
		if width >= req.minCols && height >= req.minRows {
			fitting = append(fitting, layout)
		}
	}
	// Always at least compact should fit if terminal is usable
	if len(fitting) == 0 {
		fitting = append(fitting, "compact")
	}
	// Sort for consistent display order
	sort.Strings(fitting)
	return fitting
}

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
	authClient        *api.RPAuthClient
	commentsClient    *api.RPCommentsClient
	ratingsClient     *api.RPRatingsClient
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
	lyrics          string
	syncedLyrics    []api.SyncedLyric
	artistInfo      *models.ArtistInfo
	artistStatus    string                        // current loading status for artist view
	artistCache     map[string]*models.ArtistInfo // keyed by lowercase artist name
	comments        []*api.Comment                // all loaded comments for current song
	commentsStatus  string                        // loading status for comments view
	commentsTotal   int                           // total number of comments available
	commentsPage    int                           // current page (0-indexed)
	commentsPerPage int                           // comments per page (default 20)
	commentsSongID  int64                         // song ID that the displayed comments belong to
	commentsLoaded  bool                          // whether more comments can be loaded from API

	// Pending content (fetched for current song, not yet shown).
	// User presses 'u' to update displayed content from pending.
	// Synced lyrics bypass this — they always auto-update.
	pendingLyrics          string
	pendingArtistInfo      *models.ArtistInfo
	pendingComments        []*api.Comment // pending comments for new song
	pendingCommentsTotal   int
	pendingCommentsMore    bool
	pendingCommentsOffset  int
	pendingEventID         int64  // eventID the pending data belongs to
	pendingArtistArtStr    string // pending rendered artist thumbnail
	pendingArtistArtLoaded bool
	pendingArtistArtWidth  int
	pendingArtistArtHeight int

	// Bubbles components
	viewport       viewport.Model
	albumArtStr    string // cached rendered escape sequence
	albumArtWidth  int    // width in terminal columns
	albumArtHeight int    // height in terminal rows

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
	optionsModal           *modals.Options
	skipWarningModal       *modals.SkipWarning
	favoritesModal         *modals.Favorites
	galleryModal           *modals.Gallery
	stationWarningModal    *modals.StationWarning
	ratingModal            *modals.Rating
	networkTransitionModal *modals.NetworkTransition
	sleepTimerModal        *modals.SleepTimer

	// UI dimensions
	width  int
	height int

	// Status
	statusMsg     string
	statusIsError bool
	statusSeq     int

	// Scrobble support
	scrobbler        *api.Scrobbler
	songStartTime    time.Time
	scrobbleEligible bool
	scrobbleFlashAt  time.Time      // when scrobble flash started
	scrobbleStates   map[string]int // per-service flash state: "fm" -> 0/1/2/3
	scrobbleServices []string       // list of active services: ["fm", "lb"]

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

	// Detected image protocol (Kitty, Sixel, ITerm2, or Halfblocks)
	imageProtocol termimg.Protocol

	// Notification tracking
	notifSentForSong bool // true once desktop notification fired for current song

	// Visualizer state
	vis            *visualizer.Visualizer // visualizer engine
	visFullscreen  bool                   // visualizer is in fullscreen mode
	visInfoShownAt time.Time              // when song info overlay was last shown
	visInfoVisible bool                   // whether info overlay is currently visible

	// Jukebox mode
	jukeboxMode      bool               // whether jukebox mode is active
	jukeboxQueue     []cache.CachedSong // shuffled queue of favorites
	jukeboxPlayed    int                // number of songs played so far (current song included)
	jukeboxTotal     int                // total songs in this jukebox session
	jukeboxBatchSize int                // how many songs to queue at once
	crossfading      bool               // whether currently doing a crossfade volume ramp

	// Offline mode
	offlineMode                 bool   // whether offline mode is active
	offlineCache                string // name of the offline cache being played
	offlineSongs                []cache.CachedSong
	offlineIndex                int
	offlineStation              int
	offlineBitrate              int
	offlineModeStartedConnected bool // true if --offline had connection on first check

	// Layout mode
	layoutMode    int    // LayoutLarge, LayoutMedium, LayoutCompact, LayoutNarrow
	initialLayout string // user's original choice (config + CLI) for prompt
	// Note: layoutCheckDone and layoutPromptActive are now package-level vars
	// to persist across View() calls (View uses value receiver)

	// Sleep timer
	sleepTimerActive    bool          // whether sleep timer is active
	sleepTimerDuration  time.Duration // how long until sleep
	sleepTimerExpiresAt time.Time     // when the timer expires
	sleepTimerTicker    *time.Ticker  // ticker for countdown updates
	sleepTimerQuitChan  chan struct{} // channel to stop timer

	// Quitting state (after sleep timer fires)
	quittingActive    bool         // whether we're in the 60s countdown to quit
	quittingStartedAt time.Time    // when the 60s countdown started
	quittingTicker    *time.Ticker // ticker for countdown updates
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config, theme *config.ColorTheme, startJukebox bool, layoutOverride string, sleepTimerDuration time.Duration) *Model {
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

	// Detect image protocol (Kitty, Sixel, ITerm2, or Halfblocks)
	imageProtocol := termimg.DetectProtocol()
	logger.Printf("Detected image protocol: %s", imageProtocol)

	// Initialize API clients
	rpAPI := api.NewRadioParadiseAPI(cfg.Channel, cfg.Bitrate)

	// Set up RP authentication if configured
	authClient := api.NewRPAuthClient()
	authPath := filepath.Join(xdg.ConfigHome, "rptui", "auth.toml")
	if err := authClient.LoadState(authPath); err != nil {
		logger.Printf("Warning: failed to load RP auth state: %v", err)
	}
	if cfg.RPAuth.Username != "" && cfg.RPAuth.Password != "" {
		authClient.SetCredentials(cfg.RPAuth.Username, cfg.RPAuth.Password)
		if !authClient.HasAuth() {
			// Have credentials but no valid tokens — attempt login
			if err := authClient.Login(cfg.RPAuth.Username, cfg.RPAuth.Password); err != nil {
				logger.Printf("RP API auth failed: %v", err)
			} else {
				// Save new tokens
				if err := authClient.SaveState(authPath); err != nil {
					logger.Printf("Warning: failed to save RP auth state: %v", err)
				}
			}
		}
	}
	if authClient.HasAuth() {
		rpAPI.WithAuth(authClient)
		logger.Printf("RP API: authenticated as %s (user ID: %s)", authClient.Username(), authClient.UserID())
	} else if cfg.RPAuth.Username != "" {
		logger.Printf("RP API: authentication not available (login failed, no valid session)")
		// If channel 99 is configured but auth failed, fall back to main mix
		if cfg.Channel == 99 {
			logger.Printf("Channel 99 requires authentication, falling back to Main Mix")
			cfg.Channel = 0
			rpAPI.SetChannel(0)
		}
	} else {
		logger.Printf("RP API: unauthenticated (ratings, comments, favorites disabled)")
		// If channel 99 is configured but no auth credentials, fall back to main mix
		if cfg.Channel == 99 {
			logger.Printf("Channel 99 requires authentication, falling back to Main Mix")
			cfg.Channel = 0
			rpAPI.SetChannel(0)
		}
	}

	commentsClient := api.NewRPCommentsClient(authClient)
	ratingsClient := api.NewRPRatingsClient(authClient)

	// Fetch chan_99_cutoff if authenticated (used for RP favorites integration)
	if authClient.HasAuth() {
		if favsCount, err := ratingsClient.GetFavsCount(); err == nil {
			authClient.SetChan99Cutoff(favsCount.Chan99Cutoff)
			logger.Printf("RP API: chan_99_cutoff=%d, favsCount.R7=%s", favsCount.Chan99Cutoff, favsCount.FavsCount.R7)
			if saveErr := authClient.SaveState(authPath); saveErr != nil {
				logger.Printf("Warning: failed to save RP auth state with cutoff: %v", saveErr)
			}
		} else {
			logger.Printf("Warning: failed to fetch RP favs count, using default cutoff=7: %v", err)
		}
	}

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
	cacheManager.SetOfflineDir(filepath.Join(filepath.Dir(cfg.GetFavoritesDir()), "offline"))
	scrobbler := api.NewScrobbler(cfg)
	if scrobbler.Enabled() {
		logger.Printf("Scrobble enabled: %v", scrobbler.ServiceNames())
	}
	if err := cacheManager.EnsureDirectories(); err != nil {
		logger.Printf("Warning: failed to create cache directories: %v", err)
	}

	// Auto-blocklist sync: fetch low-rated songs from RP and sync blocklist
	if cfg.AutoBlocklistRPEnabled && authClient.HasAuth() {
		logger.Printf("Auto-blocklist: syncing songs rated <= %d from RP", cfg.AutoBlocklistRPThreshold)
		songs, err := ratingsClient.GetAllProfileFavorites(authClient.UserID(), "Low", 1, cfg.AutoBlocklistRPThreshold)
		if err != nil {
			logger.Printf("Auto-blocklist: failed to fetch RP ratings: %v", err)
		} else {
			songIDs := make(map[int64]string)
			for _, s := range songs {
				var songID int64
				fmt.Sscanf(s.SongID, "%d", &songID)
				if songID > 0 {
					songIDs[songID] = s.Title
				}
			}
			if err := cacheManager.SyncAutoBlocklist(songIDs); err != nil {
				logger.Printf("Auto-blocklist: failed to sync: %v", err)
			} else {
				logger.Printf("Auto-blocklist: blocked %d songs from RP ratings", len(songIDs))
			}
		}
	}

	// Initialize custom widgets
	headerWidget := widgets.NewHeader(styles.Header, "rptui - Radio Paradise")
	footerWidget := widgets.NewFooter(styles.AccentStyle, styles.MutedStyle)
	footerWidget.SetScrobbleServices(scrobbler.ServiceNames())
	if rpAPI.IsAuthenticated() {
		footerWidget.AddChannel99()
	}
	nowPlayingWidget := widgets.NewNowPlaying(styles.ForegroundStyle, styles.AccentStyle, styles.MutedStyle, theme.Accent, theme.Cursor, theme.Background)
	playlistWidget := widgets.NewPlaylist(styles)

	// Initialize modal widgets
	optionsModal := modals.NewOptions(styles, cfg.Channel, cfg.Bitrate, cfg.ShowAlbumArt, cfg.ShowSkipWarning, cfg.CopyAlbumArt, cfg.NotificationsEnabled, cfg.NotificationsShowArt, cfg.Visualizer.Mode, cfg.ColorsFile, cfg.Theme)
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

	m := &Model{
		config:                 cfg,
		theme:                  theme,
		styles:                 styles,
		themeWatcher:           themeWatcher,
		rpAPI:                  rpAPI,
		authClient:             authClient,
		commentsClient:         commentsClient,
		ratingsClient:          ratingsClient,
		lyricsClient:           lyricsClient,
		wikipediaClient:        wikipediaClient,
		discogsClient:          discogsClient,
		musicbrainzClient:      musicbrainzClient,
		theaudiodbClient:       theaudiodbClient,
		mpvBackend:             mpvBackend,
		cacheManager:           cacheManager,
		scrobbler:              scrobbler,
		bottomViewMode:         ViewPlaylist,
		headerWidget:           headerWidget,
		footerWidget:           footerWidget,
		nowPlayingWidget:       nowPlayingWidget,
		playlistWidget:         playlistWidget,
		optionsModal:           optionsModal,
		skipWarningModal:       skipWarningModal,
		viewport:               viewport,
		help:                   help,
		spinner:                sp,
		cellRatio:              cellRatio,
		imageProtocol:          imageProtocol,
		downloadResults:        make(chan favoriteDownloadMsg, 1),
		jukeboxMode:            startJukebox,
		jukeboxBatchSize:       10,
		commentsPerPage:        20,
		networkTransitionModal: nil,
	}

	// Determine layout mode
	layoutMode := LayoutLarge
	switch cfg.Layout {
	case "medium":
		layoutMode = LayoutMedium
	case "compact":
		layoutMode = LayoutCompact
	case "narrow":
		layoutMode = LayoutNarrow
	}
	if layoutOverride != "" {
		switch layoutOverride {
		case "large":
			layoutMode = LayoutLarge
		case "medium":
			layoutMode = LayoutMedium
		case "compact":
			layoutMode = LayoutCompact
		case "narrow":
			layoutMode = LayoutNarrow
		default:
			logger.Printf("Warning: invalid layout %q, using %q", layoutOverride, layoutNames[layoutMode])
		}
	}
	m.layoutMode = layoutMode
	m.initialLayout = layoutNames[layoutMode] // store user's original choice for prompt

	// Narrow mode forces album art on internally (without it, there'd just be empty space at the top)
	if layoutMode == LayoutNarrow {
		m.config.ShowAlbumArt = true
	}

	// Initialize sleep timer from CLI flag
	if sleepTimerDuration > 0 {
		m.sleepTimerActive = true
		m.sleepTimerDuration = sleepTimerDuration
		m.sleepTimerExpiresAt = time.Now().Add(sleepTimerDuration)
		m.sleepTimerQuitChan = make(chan struct{})
		m.sleepTimerTicker = time.NewTicker(time.Minute)
		logger.Printf("Sleep timer started: %v", sleepTimerDuration)
	}

	return m
}

// NewOfflineModel creates a new TUI model for offline playback
func NewOfflineModel(cfg *config.Config, theme *config.ColorTheme, songs []cache.CachedSong, cacheName string, layoutOverride string) *Model {
	styles := config.NewThemeStyles(theme)

	themeWatcher := config.NewThemeWatcher(cfg.ColorsFile)
	if err := themeWatcher.Start(); err != nil {
		logger.Printf("Warning: failed to start theme watcher: %v", err)
	}

	// Initialize terminal cell ratio for album art
	features := termimg.QueryTerminalFeatures()
	cellRatio := float64(features.FontHeight) / float64(features.FontWidth)
	if cellRatio <= 0 {
		cellRatio = 2.0
	}

	// Detect image protocol (Kitty, Sixel, ITerm2, or Halfblocks)
	imageProtocol := termimg.DetectProtocol()
	logger.Printf("Detected image protocol: %s", imageProtocol)

	// Initialize API clients (lyrics, artist info still available for lookups)
	rpAPI := api.NewRadioParadiseAPI(cfg.Channel, cfg.Bitrate)
	lyricsClient := api.NewLRCLibClient()
	wikipediaClient := api.NewWikipediaClient()
	discogsClient := api.NewDiscogsClient(cfg.DiscogsToken, cfg.DiscogsKey, cfg.DiscogsSecret)
	musicbrainzClient := api.NewMusicBrainzClient()
	theaudiodbClient := api.NewTheAudioDBClient()
	mpvBackend := mpv.NewMPVBackend()
	cacheManager := cache.NewCacheManager(
		cfg.GetFavoritesDir(),
		cfg.GetBlocklistDir(),
		cfg.MaxFavorites,
	)
	cacheManager.SetOfflineDir(filepath.Join(filepath.Dir(cfg.GetFavoritesDir()), "offline"))
	scrobbler := api.NewScrobbler(cfg)
	if err := cacheManager.EnsureDirectories(); err != nil {
		logger.Printf("Warning: failed to create cache directories: %v", err)
	}

	// Initialize custom widgets
	headerWidget := widgets.NewHeader(styles.Header, "rptui - Radio Paradise (Offline)")
	footerWidget := widgets.NewFooter(styles.AccentStyle, styles.MutedStyle)
	footerWidget.SetScrobbleServices(scrobbler.ServiceNames())
	nowPlayingWidget := widgets.NewNowPlaying(styles.ForegroundStyle, styles.AccentStyle, styles.MutedStyle, theme.Accent, theme.Cursor, theme.Background)
	playlistWidget := widgets.NewPlaylist(styles)

	// Initialize modal widgets
	optionsModal := modals.NewOptions(styles, cfg.Channel, cfg.Bitrate, cfg.ShowAlbumArt, cfg.ShowSkipWarning, cfg.CopyAlbumArt, cfg.NotificationsEnabled, cfg.NotificationsShowArt, cfg.Visualizer.Mode, cfg.ColorsFile, cfg.Theme)
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

	// Convert CachedSong to models.Song for playlist display
	var modelSongs []*models.Song
	for _, cs := range songs {
		s := cs.ToSong()
		modelSongs = append(modelSongs, s)
	}

	// Read cache config for station/bitrate display
	offlineStation := cfg.Channel
	offlineBitrate := cfg.Bitrate
	if len(songs) > 0 {
		// Try to read from config.json in cache directory
		offlineDir := filepath.Join(filepath.Dir(cfg.GetFavoritesDir()), "offline", cacheName)
		configPath := filepath.Join(offlineDir, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			var cacheConfig struct {
				Station int `json:"station"`
				Bitrate int `json:"bitrate"`
			}
			if err := json.Unmarshal(data, &cacheConfig); err == nil {
				offlineStation = cacheConfig.Station
				offlineBitrate = cacheConfig.Bitrate
			}
		}
	}

	m := &Model{
		config:                      cfg,
		theme:                       theme,
		styles:                      styles,
		themeWatcher:                themeWatcher,
		rpAPI:                       rpAPI,
		lyricsClient:                lyricsClient,
		wikipediaClient:             wikipediaClient,
		discogsClient:               discogsClient,
		musicbrainzClient:           musicbrainzClient,
		theaudiodbClient:            theaudiodbClient,
		mpvBackend:                  mpvBackend,
		cacheManager:                cacheManager,
		scrobbler:                   scrobbler,
		bottomViewMode:              ViewPlaylist,
		headerWidget:                headerWidget,
		footerWidget:                footerWidget,
		nowPlayingWidget:            nowPlayingWidget,
		playlistWidget:              playlistWidget,
		optionsModal:                optionsModal,
		skipWarningModal:            skipWarningModal,
		viewport:                    viewport,
		help:                        help,
		spinner:                     sp,
		cellRatio:                   cellRatio,
		imageProtocol:               imageProtocol,
		downloadResults:             make(chan favoriteDownloadMsg, 1),
		offlineMode:                 true,
		offlineCache:                cacheName,
		offlineSongs:                songs,
		offlineIndex:                0,
		offlineStation:              offlineStation,
		offlineBitrate:              offlineBitrate,
		offlineModeStartedConnected: false,
		songs:                       modelSongs,
		currentSongIndex:            0,
		networkTransitionModal:      nil,
	}

	// Determine layout mode
	layoutMode := LayoutLarge
	switch cfg.Layout {
	case "medium":
		layoutMode = LayoutMedium
	case "compact":
		layoutMode = LayoutCompact
	case "narrow":
		layoutMode = LayoutNarrow
	}
	if layoutOverride != "" {
		switch layoutOverride {
		case "large":
			layoutMode = LayoutLarge
		case "medium":
			layoutMode = LayoutMedium
		case "compact":
			layoutMode = LayoutCompact
		case "narrow":
			layoutMode = LayoutNarrow
		default:
			logger.Printf("Warning: invalid layout %q, using %q", layoutOverride, layoutNames[layoutMode])
		}
	}
	m.layoutMode = layoutMode

	// Narrow mode forces album art on internally
	if layoutMode == LayoutNarrow {
		m.config.ShowAlbumArt = true
	}

	return m
}

// Init initializes the TUI model
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tickProgressCmd(),
		tickPollCmd(),
		tea.RequestBackgroundColor,
		m.downloadResultsCmd(),
	}
	if m.themeWatcher != nil {
		cmds = append(cmds, watchThemeCmd(m.themeWatcher))
	}

	// In jukebox mode, trigger jukebox start instead of fetching API block
	if m.jukeboxMode {
		cmds = append(cmds, startJukeboxCmd())
	} else if m.offlineMode {
		cmds = append(cmds, startOfflineCmd())
	} else {
		cmds = append(cmds, m.fetchBlockCmd)
	}

	// Start sleep timer ticker if active
	if m.sleepTimerActive {
		cmds = append(cmds, tickSleepTimerCmd())
	}

	// Station validation runs in background (non-blocking, 3s timeout)
	// Skip in offline mode - no network calls needed
	if !m.offlineMode {
		cmds = append(cmds, m.checkStationsCmd)
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

		// For compact/narrow layouts, constrain header/footer to now-playing width
		// so they align with nowplaying widget
		widgetWidth := msg.Width
		if m.layoutMode == LayoutNarrow {
			artHeight := 16
			artWidth := int(float64(artHeight) * m.cellRatio)
			if artWidth < 10 {
				artWidth = 10
			}
			widgetWidth = artWidth
		} else if m.layoutMode == LayoutCompact {
			// Compact: use same width as nowplaying widget
			widgetWidth = msg.Width - 4
		}
		m.headerWidget.SetWidth(widgetWidth)
		m.footerWidget.SetWidth(widgetWidth)
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
			case ModalStationWarning:
				cmd = m.stationWarningModal.Update(msg)
			case ModalNetworkTransition:
				if m.networkTransitionModal != nil {
					cmd = m.networkTransitionModal.Update(msg)
				}
			case ModalRating:
				if m.ratingModal != nil {
					cmd = m.ratingModal.Update(msg)
				}
			case ModalSleepTimer:
				if m.sleepTimerModal != nil {
					cmd = m.sleepTimerModal.Update(msg)
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

		// Apply theme changes
		if msg.Theme != nil {
			themeVal := *msg.Theme
			switch themeVal {
			case "CUSTOM":
				// Keep colors_file as-is, clear theme name
				m.config.Theme = ""
			case "OMARCHY":
				// Clear colors_file and theme to trigger omarchy fallback
				m.config.ColorsFile = ""
				m.config.Theme = ""
			case "DEFAULT":
				// Clear both to use full fallback chain
				m.config.ColorsFile = ""
				m.config.Theme = ""
			default:
				// Built-in theme - set theme name, clear colors_file
				m.config.Theme = themeVal
				m.config.ColorsFile = ""
			}

			// Reload theme
			newTheme, err := config.LoadTheme(m.config.ColorsFile, m.config.Theme)
			if err != nil {
				log.Printf("Failed to reload theme: %v", err)
			} else {
				m.theme = newTheme
				m.styles = config.NewThemeStyles(newTheme)
				m.nowPlayingWidget.UpdateStyles(
					m.styles.ForegroundStyle,
					m.styles.AccentStyle,
					m.styles.MutedStyle,
					newTheme.Accent,
					newTheme.Cursor,
					newTheme.Background,
				)
				m.playlistWidget.UpdateStyles(m.styles)
				m.headerWidget.UpdateStyles(m.styles.Header)
				m.footerWidget.UpdateStyles(m.styles.AccentStyle, m.styles.MutedStyle)
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

	case modals.SleepTimerMsg:
		m.activeModal = ModalNone
		if msg.Closed {
			return handle(m, renderAlbumArtAfterDelay())
		}
		if msg.Cancelled {
			m.stopSleepTimer()
			return handle(m, setStatus(&m, "Sleep timer cancelled", false))
		}
		if msg.Duration > 0 {
			m.startSleepTimer(msg.Duration)
			mins := int(msg.Duration.Minutes())
			// Start the tick
			return handle(m, tea.Batch(setStatus(&m, fmt.Sprintf("Sleep timer set for %d min", mins), false), tickSleepTimerCmd()))
		}
		return handle(m, renderAlbumArtAfterDelay())

	case modals.StationWarningMsg:
		m.activeModal = ModalNone
		return handle(m, renderAlbumArtAfterDelay())

	case modals.NetworkTransitionMsg:
		m.activeModal = ModalNone

		switch msg.Action {
		case "select":
			// User selected an offline cache - switch to it
			return m.switchToOfflineMode(msg.Cache)

		case "dismiss":
			// User dismissed - continue with retry logic
			return m, tickConnRetryCmd(m.retryInterval)

		case "confirm":
			if msg.Response {
				// User confirmed - return to live stream
				return m.exitOfflineMode()
			}
			// User declined - stay in offline mode
			return m, nil
		}
		return m, nil

	case modals.FavoritesMsg:
		if !msg.StayOpen {
			m.activeModal = ModalNone
		}
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
		} else if msg.EnqueueEventID != nil {
			fav, err := m.cacheManager.GetFavoriteByEventID(*msg.EnqueueEventID)
			if err == nil && fav != nil {
				song := fav.ToSong()
				song.IsFromFavorite = true
				url := song.GaplessURL
				if fav.AudioPath != "" {
					url = fav.AudioPath
				}
				if err := m.mpvBackend.AppendToPlaylist([]string{url}); err == nil {
					m.songs = append(m.songs, song)
					m.updatePlaylist()
					if msg.StayOpen {
						m.favoritesModal.SetToastMessage("Favorite enqueued")
					}
					return handle(m, setStatus(&m, "Favorite enqueued", false))
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

	case chan99FetchedMsg:
		return handle(m.handleChan99Fetched(msg))

	case jukeboxStartMsg:
		return handle(m.handleJukeboxStart())

	case offlineStartMsg:
		return handle(m.handleOfflineStart())

	case progressTickMsg:
		return handle(m.handleProgressTick(msg))

	case pollTickMsg:
		return handle(m.handlePollTick(msg))

	case sleepTimerTickMsg:
		return handle(m.handleSleepTimerTick(msg))

	case quitTickMsg:
		return handle(m.handleQuitTick(msg))

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

	case commentsFetchedMsg:
		return handle(m.handleCommentsFetched(msg))

	case statusClearMsg:
		if msg.seq == m.statusSeq {
			m.statusMsg = ""
			m.statusIsError = false
		}
		return handle(m, nil)

	case stationCheckResultMsg:
		if len(msg.issues) > 0 {
			m.stationWarningModal = modals.NewStationWarning(m.styles, msg.issues)
			m.activeModal = ModalStationWarning
			for _, issue := range msg.issues {
				logger.Printf("Station issue [%s]: %s", issue.Kind, issue.Message)
			}
		}
		return handle(m, nil)

	case scrobbleResultMsg:
		m.scrobbleFlashAt = time.Now()
		m.scrobbleStates = make(map[string]int)
		m.scrobbleServices = make([]string, 0, len(msg.results))
		for _, r := range msg.results {
			m.scrobbleServices = append(m.scrobbleServices, r.Service)
			if r.Success {
				m.scrobbleStates[r.Service] = flashSolid
			} else {
				m.scrobbleStates[r.Service] = flashBlinkOn
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

	case modals.RatingMsg:
		m.activeModal = ModalNone
		if msg.Submitted {
			if m.currentSong != nil && m.ratingsClient != nil {
				_, err := m.ratingsClient.SubmitRating(m.currentSong.SongID, msg.Rating)
				if err != nil {
					return m, setStatus(&m, fmt.Sprintf("Rating failed: %v", err), true)
				}
				// Update local user rating
				m.currentSong.UserRating = fmt.Sprintf("%d", msg.Rating)

				// Auto-blocklist if rating is at or below threshold
				if m.config.AutoBlocklistRPEnabled && msg.Rating <= m.config.AutoBlocklistRPThreshold {
					if err := m.cacheManager.AddBlocklist(m.currentSong, false); err == nil {
						return m, setStatus(&m, fmt.Sprintf("Rated %d/10, added to blocklist", msg.Rating), false)
					}
				}
				return m, setStatus(&m, fmt.Sprintf("Rated %d/10", msg.Rating), false)
			}
		}
		return m, renderAlbumArtAfterDelay()

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
	// Handle layout selection prompt (when showing terminal size warning)
	if layoutPromptActive && m.width > 0 && m.height > 0 {
		key := strings.ToLower(msg.String())
		fittingLayouts := getFittingLayouts(m.width, m.height)

		// Check for quit
		if key == "q" || key == "ctrl+c" {
			m.cacheManager.WaitForDownloads(60 * time.Second)
			if m.mpvBackend != nil {
				m.mpvBackend.Stop()
			}
			if m.themeWatcher != nil {
				m.themeWatcher.Close()
			}
			if m.vis != nil {
				m.vis.Close()
			}
			return m, tea.Quit
		}

		// Check if key matches initial layout choice
		if strings.ToLower(m.initialLayout[:1]) == key {
			// User chose their initial layout - clear prompt flag, let normal init proceed
			layoutPromptActive = false
			m.initialized = true
			return m, tea.Batch(m.songChangedCmds())
		}

		// Check if key matches any fitting layout
		for _, l := range fittingLayouts {
			if strings.ToLower(l[:1]) == key && l != m.initialLayout {
				// Switch to that layout
				switch l {
				case "large":
					m.layoutMode = LayoutLarge
				case "medium":
					m.layoutMode = LayoutMedium
				case "compact":
					m.layoutMode = LayoutCompact
				case "narrow":
					m.layoutMode = LayoutNarrow
				}
				// Clear prompt flag, let normal init proceed
				layoutPromptActive = false
				m.initialized = true
				return m, tea.Batch(m.songChangedCmds())
			}
		}

		// Invalid key - ignore but don't return quit
		return m, nil
	}

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
			return m, clearKittyImagesCmdIf(m.imageProtocol)
		}
		if m.currentSongIndex < len(m.songs)-1 {
			if err := m.mpvBackend.SkipNext(); err == nil {
				m.currentSongIndex++
				if m.jukeboxMode {
					m.jukeboxPlayed++
					m.checkJukeboxRefill()
				}
				return m, m.songChangedCmds()
			}
		} else {
			// In jukebox mode, try refilling before giving up
			if m.jukeboxMode {
				m.checkJukeboxRefill()
				if len(m.songs) > m.currentSongIndex+1 {
					if err := m.mpvBackend.SkipNext(); err == nil {
						m.currentSongIndex++
						m.jukeboxPlayed++
						return m, m.songChangedCmds()
					}
				}
			}
			return m, setStatus(&m, "No more songs in block", false)
		}
		return m, nil

	case "P":
		// Previous comments page when in comments view
		if m.bottomViewMode == ViewComments && m.commentsPage > 0 {
			m.commentsPage--
			m.commentsStatus = ""
			m.updateBottomView()
			return m, nil
		}
		return m, nil

	case "p":
		// Previous song
		if m.currentSongIndex > 0 {
			if err := m.mpvBackend.SkipPrev(); err == nil {
				m.currentSongIndex--
				if m.jukeboxMode && m.jukeboxPlayed > 1 {
					m.jukeboxPlayed--
				}
				return m, m.songChangedCmds()
			}
		} else {
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

	case "R":
		// Rate song — only when authenticated
		if !m.rpAPI.IsAuthenticated() {
			return m, setStatus(&m, "Rating requires RP authentication", true)
		}
		if m.visFullscreen {
			return m, nil
		}
		if m.currentSong == nil || m.currentSong.SongID == 0 {
			return m, setStatus(&m, "No song to rate", true)
		}
		userRating := 5
		if m.currentSong.UserRating != "" && m.currentSong.UserRating != "0" {
			fmt.Sscanf(m.currentSong.UserRating, "%d", &userRating)
		}
		m.ratingModal = modals.NewRating(m.styles, m.currentSong.Title, m.currentSong.Artist, m.currentSong.Album, m.currentSong.Year, userRating)
		m.activeModal = ModalRating
		return m, clearKittyImagesCmdIf(m.imageProtocol)

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
		// Toggle bottom view — only in large layout
		if m.layoutMode != LayoutLarge {
			return m, setStatus(&m, "View cycling unavailable in this layout", false)
		}
		// Suppressed in fullscreen visualizer
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
			cmds = append(cmds, clearKittyImagesCmdIf(m.imageProtocol), renderAlbumArtAfterDelay())
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
		} else if m.bottomViewMode == ViewComments {
			// Fetch comments if not already loaded for current song
			if m.currentSong != nil && m.commentsSongID != m.currentSong.SongID {
				m.commentsSongID = m.currentSong.SongID
				cmds = append(cmds, m.fetchCommentsCmd())
			}
		}

		m.updateBottomView()

		// Re-render images after view change to fix any slicing caused by
		// bubbletea's \x1b[K (clear to end of line) during the view transition.
		// This is especially important for non-Kitty protocols and terminals
		// with incomplete Kitty support (like WezTerm).
		if m.config.ShowAlbumArt && m.albumArtLoaded && m.layoutMode != LayoutCompact {
			cmds = append(cmds, renderAlbumArtAfterDelay())
		}

		return m, tea.Batch(cmds...)

	case "l":
		// Load next page or navigate to next loaded page of comments
		if m.bottomViewMode != ViewComments {
			return m, nil
		}
		if !m.rpAPI.IsAuthenticated() {
			return m, setStatus(&m, "Comments require RP authentication", true)
		}
		if m.commentsTotal == 0 || m.currentSong == nil {
			return m, setStatus(&m, "No more comments", false)
		}
		// Calculate the last page that has been loaded
		maxLoadedPage := (len(m.comments) - 1) / m.commentsPerPage
		if m.commentsPage >= maxLoadedPage {
			// We're on the last loaded page — try to load more from API
			if m.commentsLoaded {
				return m, setStatus(&m, "No more comments", false)
			}
			m.commentsPage++
			m.commentsStatus = "Loading comments..."
			return m, tea.Batch(setStatus(&m, "Loading comments...", false), m.fetchCommentsPageCmd(m.commentsPage))
		}
		// Navigate to next already-loaded page
		m.commentsPage++
		m.updateBottomView()
		m.viewport.GotoTop()
		return m, nil

	case "u":
		// Update displayed lyrics/artist info/comments from pending
		applied := false
		var cmd tea.Cmd
		if m.hasPendingUpdate() {
			cmd = m.applyPendingUpdate()
			applied = true
			tea.Batch(cmd, setStatus(&m, "Updated to current song", false))
		}
		if m.pendingComments != nil && m.bottomViewMode == ViewComments {
			m.comments = m.pendingComments
			m.commentsTotal = m.pendingCommentsTotal
			m.commentsPage = 0
			m.commentsStatus = ""
			if m.currentSong != nil {
				m.commentsSongID = m.currentSong.SongID
			}
			m.pendingComments = nil
			m.pendingCommentsTotal = 0
			m.updateBottomView()
			applied = true
			setStatus(&m, "Updated to current song", false)
		}
		if applied {
			var cmds []tea.Cmd
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			cmds = append(cmds, setStatus(&m, "Updated to current song", false))
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case "f":
		// Toggle favorite — disabled in jukebox mode
		if m.jukeboxMode {
			return m, nil
		}
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

				// When authenticated, ensure RP rating matches cutoff if song isn't already rated high enough
				var ratingCmd tea.Cmd
				if m.rpAPI.IsAuthenticated() && m.currentSong.SongID != 0 && m.ratingsClient != nil {
					cutoff := m.authClient.Chan99Cutoff()
					userRating := 0
					if m.currentSong.UserRating != "" && m.currentSong.UserRating != "0" {
						fmt.Sscanf(m.currentSong.UserRating, "%d", &userRating)
					}
					if userRating < cutoff {
						// Song isn't an RP favorite yet — submit rating at cutoff
						ratingCmd = func() tea.Msg {
							_, err := m.ratingsClient.SubmitRating(m.currentSong.SongID, cutoff)
							if err == nil {
								m.currentSong.UserRating = fmt.Sprintf("%d", cutoff)
							} else {
								logger.Printf("Failed to sync RP rating: %v", err)
							}
							return nil
						}
					}
				}

				statusMsg := "Downloading favorite..."
				if m.rpAPI.IsAuthenticated() {
					cutoff := m.authClient.Chan99Cutoff()
					userRating := 0
					if m.currentSong.UserRating != "" && m.currentSong.UserRating != "0" {
						fmt.Sscanf(m.currentSong.UserRating, "%d", &userRating)
					}
					if userRating < cutoff {
						statusMsg = fmt.Sprintf("Downloading favorite, rating %d/10...", cutoff)
					}
				}

				if ratingCmd != nil {
					return m, tea.Batch(
						setStatus(&m, statusMsg, false),
						favoriteDownloadCmd(m.cacheManager, m.currentSong, m.rpAPI.GetFileExtension(), m.downloadResults),
						ratingCmd,
					)
				}
				return m, tea.Batch(
					setStatus(&m, statusMsg, false),
					favoriteDownloadCmd(m.cacheManager, m.currentSong, m.rpAPI.GetFileExtension(), m.downloadResults),
				)
			}
		}
		return m, nil

	case "b":
		// Toggle blocklist — disabled in jukebox mode
		if m.jukeboxMode {
			return m, nil
		}
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

	case "0", "1", "2", "3", "4", "5", "6":
		// Station hotkeys — disabled in offline and jukebox mode
		if m.offlineMode || m.jukeboxMode {
			return m, nil
		}
		stationMap := map[string]int{
			"0": 0,
			"1": 1,
			"2": 2,
			"3": 3,
			"4": 42,
			"5": 5,
			"6": 945,
		}
		station := stationMap[msg.String()]
		return m.switchStation(station)

	case "9":
		// My Paradise channel — only when authenticated
		if m.offlineMode || m.jukeboxMode {
			return m, nil
		}
		if !m.rpAPI.IsAuthenticated() {
			return m, setStatus(&m, "Channel 99 requires RP authentication", true)
		}
		return m.switchStation(99)

	case "o":
		// Options modal — only in large and medium layouts
		if m.layoutMode == LayoutCompact || m.layoutMode == LayoutNarrow {
			return m, setStatus(&m, "Options unavailable in this layout", false)
		}
		if m.visFullscreen {
			return m, nil
		}
		m.optionsModal = modals.NewOptions(m.styles, m.config.Channel, m.config.Bitrate, m.config.ShowAlbumArt, m.config.ShowSkipWarning, m.config.CopyAlbumArt, m.config.NotificationsEnabled, m.config.NotificationsShowArt, m.config.Visualizer.Mode, m.config.ColorsFile, m.config.Theme)
		m.activeModal = ModalOptions
		return m, clearKittyImagesCmdIf(m.imageProtocol)

	case "z":
		// Sleep timer modal — only in large and medium layouts
		if m.layoutMode == LayoutCompact || m.layoutMode == LayoutNarrow {
			return m, setStatus(&m, "Sleep timer unavailable in this layout", false)
		}
		if m.visFullscreen {
			return m, nil
		}
		var remaining time.Duration
		if m.sleepTimerActive {
			remaining = m.sleepTimerExpiresAt.Sub(time.Now())
		}
		m.sleepTimerModal = modals.NewSleepTimer(m.styles, m.sleepTimerActive, remaining)
		m.activeModal = ModalSleepTimer
		return m, clearKittyImagesCmdIf(m.imageProtocol)

	case "m":
		// Manage favorites modal — only in large and medium layouts
		if m.layoutMode == LayoutCompact || m.layoutMode == LayoutNarrow {
			return m, setStatus(&m, "Favorites management unavailable in this layout", false)
		}
		if m.visFullscreen || m.jukeboxMode {
			return m, nil
		}
		m.favoritesModal = modals.NewFavorites(m.styles, m.cacheManager, m.width, m.height)
		m.activeModal = ModalFavorites
		return m, clearKittyImagesCmdIf(m.imageProtocol)

	case "i":
		// Gallery modal — only in large layout
		if m.layoutMode != LayoutLarge {
			return m, nil
		}
		if m.bottomViewMode == ViewArtist && m.artistInfo != nil && len(m.artistInfo.GalleryURLs) > 0 {
			m.galleryModal = modals.NewGallery(
				m.styles,
				m.artistInfo.GalleryURLs,
				m.artistInfo.GallerySource,
				m.width, m.height,
				m.cellRatio,
			)
			m.galleryModal.SetProtocol(m.imageProtocol)
			m.activeModal = ModalGallery
			return m, tea.Batch(
				clearKittyImagesCmdIf(m.imageProtocol),
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
					clearKittyImagesCmdIf(m.imageProtocol),
				)
			}
			// Exiting fullscreen: restore album art
			return m, tea.Batch(
				setStatus(&m, "Visualizer: windowed", false),
				clearKittyImagesCmdIf(m.imageProtocol),
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
		if m.bottomViewMode == ViewVisualizer {
			if m.vis != nil {
				m.vis.CycleModeReverse()
				m.vis.RequestRefresh()
				return m, setStatus(&m, fmt.Sprintf("Visualizer: %s", m.vis.ModeName()), false)
			}
		}
		m.viewport.ScrollDown(1)
		return m, nil

	case "J":
		return m.toggleJukeboxMode()

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

// handleJukeboxStart initializes jukebox mode playback
func (m Model) handleJukeboxStart() (tea.Model, tea.Cmd) {
	favCount, err := m.cacheManager.GetFavoriteCount()
	if err != nil {
		return m, setStatus(&m, "Error reading favorites", true)
	}

	minFaves := m.config.Jukebox.MinFaves
	if favCount < minFaves {
		m.jukeboxMode = false
		return m, setStatus(&m, fmt.Sprintf("Save %d favorites to enable jukebox mode", minFaves), true)
	}

	favorites, err := m.cacheManager.GetFavorites()
	if err != nil {
		m.jukeboxMode = false
		return m, setStatus(&m, "Error loading favorites", true)
	}

	shuffled := make([]cache.CachedSong, len(favorites))
	copy(shuffled, favorites)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	m.jukeboxQueue = shuffled
	m.jukeboxPlayed = 0
	m.jukeboxTotal = len(shuffled)
	m.connectedAt = time.Now()

	logger.Printf("Jukebox mode: %d songs loaded", len(shuffled))

	return m.startJukeboxPlayback()
}

// handleOfflineStart initializes offline playback
func (m Model) handleOfflineStart() (tea.Model, tea.Cmd) {
	if len(m.offlineSongs) == 0 {
		return m, setStatus(&m, "No songs in offline cache", true)
	}

	// Build initial playlist (batch of songs, like jukebox)
	batchSize := 10
	if len(m.offlineSongs) < batchSize {
		batchSize = len(m.offlineSongs)
	}

	var urls []string
	var batchSongs []*models.Song
	for i := 0; i < batchSize; i++ {
		cs := m.offlineSongs[i]
		s := cs.ToSong()
		urls = append(urls, cs.AudioPath)
		batchSongs = append(batchSongs, s)
	}

	m.songs = batchSongs
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.connectedAt = time.Now()
	m.isPlaying = true
	if !layoutPromptActive {
		m.initialized = true
	}

	logger.Printf("Offline mode: %d songs loaded from cache '%s'", len(m.offlineSongs), m.offlineCache)

	m.mpvBackend.Start(urls)
	m.updatePlaylist()

	logger.Printf("Offline mode: %d songs loaded from cache '%s'", len(m.offlineSongs), m.offlineCache)

	return m, tea.Batch(
		setStatus(&m, fmt.Sprintf("Offline: %s", m.offlineCache), false),
		tickProgressCmd(),
		m.songChangedCmds(),
	)
}

// switchToOfflineMode switches from normal mode to an offline cache
func (m Model) switchToOfflineMode(cacheName string) (tea.Model, tea.Cmd) {
	offlineDir := filepath.Join(filepath.Dir(m.config.GetFavoritesDir()), "offline")
	songs, err := cache.LoadCache(offlineDir, cacheName)
	if err != nil {
		return m, setStatus(&m, fmt.Sprintf("Failed to load cache: %v", err), true)
	}

	if len(songs) == 0 {
		return m, setStatus(&m, "No songs in cache", true)
	}

	// Load cache config if available
	configPath := filepath.Join(offlineDir, cacheName, "config.json")
	var cacheConfig struct {
		Station int `json:"station"`
		Bitrate int `json:"bitrate"`
	}
	station := m.config.Channel
	bitrate := m.config.Bitrate
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &cacheConfig); err == nil {
			station = cacheConfig.Station
			bitrate = cacheConfig.Bitrate
		}
	}

	m.offlineMode = true
	m.offlineCache = cacheName
	m.offlineSongs = songs
	m.offlineIndex = 0
	m.offlineStation = station
	m.offlineBitrate = bitrate

	// Initialize with batch
	batchSize := 10
	if len(songs) < batchSize {
		batchSize = len(songs)
	}

	var urls []string
	var batchSongs []*models.Song
	for i := 0; i < batchSize; i++ {
		cs := songs[i]
		s := cs.ToSong()
		urls = append(urls, cs.AudioPath)
		batchSongs = append(batchSongs, s)
	}

	m.songs = batchSongs
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.connectedAt = time.Now()
	m.isPlaying = true
	if !layoutPromptActive {
		m.initialized = true
	}

	logger.Printf("Switched to offline mode: cache '%s' with %d songs", cacheName, len(songs))

	m.mpvBackend.Start(urls)
	m.updatePlaylist()

	return m, tea.Batch(
		setStatus(&m, fmt.Sprintf("Offline: %s", cacheName), false),
		tickProgressCmd(),
		m.songChangedCmds(),
	)
}

// exitOfflineMode exits offline mode and returns to live stream
func (m Model) exitOfflineMode() (tea.Model, tea.Cmd) {
	m.offlineMode = false
	m.offlineCache = ""
	m.offlineSongs = nil
	m.offlineIndex = 0
	m.offlineStation = 0
	m.offlineBitrate = 0
	m.offlineModeStartedConnected = false

	// Clear playlist and reset state for fresh start
	m.songs = nil
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.currentSong = nil
	m.isPlaying = false
	m.isPaused = false
	m.pollingNextBlock = false
	m.lastBlockID = 0

	m.mpvBackend.Stop()

	logger.Printf("Exited offline mode, fetching live stream")

	return m, m.fetchBlockCmd
}

// toggleJukeboxMode enters or exits jukebox mode
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

// toggleJukeboxMode enters or exits jukebox mode
func (m Model) toggleJukeboxMode() (tea.Model, tea.Cmd) {
	if m.jukeboxMode {
		return m.exitJukeboxMode()
	}

	favCount, err := m.cacheManager.GetFavoriteCount()
	if err != nil {
		return m, setStatus(&m, "Error reading favorites", true)
	}

	minFaves := m.config.Jukebox.MinFaves
	if favCount < minFaves {
		return m, setStatus(&m, fmt.Sprintf("Save %d favorites to enable jukebox mode", minFaves), true)
	}

	favorites, err := m.cacheManager.GetFavorites()
	if err != nil {
		return m, setStatus(&m, "Error loading favorites", true)
	}

	// Shuffle favorites
	shuffled := make([]cache.CachedSong, len(favorites))
	copy(shuffled, favorites)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Stop current playback
	m.mpvBackend.Stop()

	// Clear state
	m.songs = nil
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.currentSong = nil
	m.isPlaying = false
	m.isPaused = false
	m.pollingNextBlock = false
	m.initialized = false
	m.connectedAt = time.Now()
	m.lyrics = ""
	m.syncedLyrics = nil
	m.artistInfo = nil
	m.artistStatus = ""
	m.pendingLyrics = ""
	m.pendingArtistInfo = nil
	m.pendingEventID = 0
	m.albumArtLoaded = false
	m.albumArtStr = ""
	m.artistArtStr = ""
	m.artistArtLoaded = false
	m.connErrorMsg = ""
	m.connState = ""
	m.consecutiveFailures = 0
	m.retryInterval = 0

	// Set jukebox state
	m.jukeboxMode = true
	m.jukeboxQueue = shuffled
	m.jukeboxPlayed = 0
	m.jukeboxTotal = len(shuffled)

	logger.Printf("Jukebox mode: %d songs loaded", len(shuffled))

	return m.startJukeboxPlayback()
}

// exitJukeboxMode stops jukebox mode and clears playlist
func (m Model) exitJukeboxMode() (tea.Model, tea.Cmd) {
	logger.Printf("DEBUG: exitJukeboxMode called, setting initialized=false")
	m.mpvBackend.Stop()

	m.songs = nil
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.currentSong = nil
	m.isPlaying = false
	m.isPaused = false
	m.pollingNextBlock = false
	m.initialized = false
	m.connectedAt = time.Time{}
	m.lyrics = ""
	m.syncedLyrics = nil
	m.artistInfo = nil
	m.artistStatus = ""
	m.pendingLyrics = ""
	m.pendingArtistInfo = nil
	m.pendingEventID = 0
	m.albumArtLoaded = false
	m.albumArtStr = ""
	m.artistArtStr = ""
	m.artistArtLoaded = false
	m.connErrorMsg = ""
	m.connState = ""
	m.consecutiveFailures = 0
	m.retryInterval = 0
	m.playbackPos = mpv.PlaybackPosition{}

	m.jukeboxMode = false
	m.jukeboxQueue = nil
	m.jukeboxPlayed = 0
	m.jukeboxTotal = 0
	m.crossfading = false

	logger.Printf("Jukebox mode exited")

	m.mpvBackend.SetVolume(100.0)

	return m, tea.Batch(
		tickProgressCmd(),
		clearKittyImagesCmdIf(m.imageProtocol),
		setStatus(&m, "Jukebox mode off", false),
		m.fetchBlockCmd,
	)
}

// startJukeboxPlayback starts MPV with the first batch of jukebox songs
func (m Model) startJukeboxPlayback() (tea.Model, tea.Cmd) {
	if len(m.jukeboxQueue) == 0 {
		if m.config.Jukebox.Repeat {
			m.reshuffleJukebox()
		} else {
			return m, setStatus(&m, "Jukebox complete", false)
		}
	}

	batchSize := m.jukeboxBatchSize
	if batchSize > len(m.jukeboxQueue) {
		batchSize = len(m.jukeboxQueue)
	}

	urls := make([]string, batchSize)
	songs := make([]*models.Song, batchSize)
	for i := 0; i < batchSize; i++ {
		cs := m.jukeboxQueue[i]
		urls[i] = cs.AudioPath
		songs[i] = cs.ToSong()
	}

	m.jukeboxQueue = m.jukeboxQueue[batchSize:]

	if err := m.mpvBackend.Start(urls); err != nil {
		logger.Printf("Failed to start MPV for jukebox: %v", err)
		return m, setStatus(&m, fmt.Sprintf("Error: %v", err), true)
	}

	m.songs = songs
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.isPlaying = true
	if !layoutPromptActive {
		m.initialized = true
	}
	m.jukeboxPlayed = 1

	logger.Printf("Jukebox: started batch of %d songs, %d remaining", batchSize, len(m.jukeboxQueue))

	return m, tea.Batch(clearKittyImagesCmdIf(m.imageProtocol), setStatus(&m, fmt.Sprintf("🎶 Jukebox: %d songs", m.jukeboxTotal), false), m.songChangedCmds())
}

// startSleepTimer starts a sleep timer with the given duration
func (m *Model) startSleepTimer(duration time.Duration) {
	m.stopSleepTimer()

	m.sleepTimerActive = true
	m.sleepTimerDuration = duration
	m.sleepTimerExpiresAt = time.Now().Add(duration)
	m.sleepTimerQuitChan = make(chan struct{})
	m.sleepTimerTicker = time.NewTicker(time.Minute)

	logger.Printf("Sleep timer started: %v", duration)
}

// stopSleepTimer stops the active sleep timer
func (m *Model) stopSleepTimer() {
	if m.sleepTimerTicker != nil {
		m.sleepTimerTicker.Stop()
		m.sleepTimerTicker = nil
	}
	if m.sleepTimerQuitChan != nil {
		close(m.sleepTimerQuitChan)
		m.sleepTimerQuitChan = nil
	}
	m.sleepTimerActive = false
	m.sleepTimerDuration = 0
	m.sleepTimerExpiresAt = time.Time{}

	logger.Println("Sleep timer stopped")
}

// refillJukeboxBatch appends the next batch of songs to the MPV playlist
// and prunes old songs from the front to keep the playlist manageable
func (m *Model) refillJukeboxBatch() {
	if len(m.jukeboxQueue) == 0 {
		return
	}

	addCount := 5
	if addCount > len(m.jukeboxQueue) {
		addCount = len(m.jukeboxQueue)
	}

	urls := make([]string, addCount)
	songs := make([]*models.Song, addCount)
	for i := 0; i < addCount; i++ {
		cs := m.jukeboxQueue[i]
		urls[i] = cs.AudioPath
		songs[i] = cs.ToSong()
	}

	m.jukeboxQueue = m.jukeboxQueue[addCount:]

	if err := m.mpvBackend.AppendToPlaylist(urls); err != nil {
		logger.Printf("Failed to append jukebox batch: %v", err)
		return
	}

	m.songs = append(m.songs, songs...)

	// Prune played songs from the front to keep playlist at ~10 items
	pruneCount := addCount
	if pruneCount > m.currentSongIndex {
		pruneCount = m.currentSongIndex
	}
	if pruneCount > 0 {
		m.songs = m.songs[pruneCount:]
		m.currentSongIndex -= pruneCount
		m.playlistStartIdx -= pruneCount
		logger.Printf("Jukebox: pruned %d songs from front", pruneCount)
	}

	logger.Printf("Jukebox: added %d songs, playlist now %d, %d remaining in queue", addCount, len(m.songs), len(m.jukeboxQueue))
}

// reshuffleJukebox reshuffles all favorites for repeat mode
func (m *Model) reshuffleJukebox() {
	favorites, err := m.cacheManager.GetFavorites()
	if err != nil {
		logger.Printf("Failed to reload favorites for reshuffle: %v", err)
		return
	}

	m.jukeboxQueue = favorites
	m.jukeboxTotal = len(favorites)
	m.jukeboxPlayed = 0
	rand.Shuffle(len(m.jukeboxQueue), func(i, j int) {
		m.jukeboxQueue[i], m.jukeboxQueue[j] = m.jukeboxQueue[j], m.jukeboxQueue[i]
	})

	logger.Printf("Jukebox: reshuffled %d songs", len(m.jukeboxQueue))
}

// checkJukeboxRefill checks if we need to append more songs to the MPV playlist
func (m *Model) checkJukeboxRefill() tea.Cmd {
	if !m.jukeboxMode || len(m.jukeboxQueue) == 0 {
		return nil
	}

	// Refill when 2 or fewer songs remain in the playlist
	remainingInPlaylist := len(m.songs) - 1 - m.currentSongIndex
	if remainingInPlaylist <= 2 {
		m.refillJukeboxBatch()
		m.updatePlaylist()
	}
	return nil
}

// fadeVolumeIn gradually increases volume from current to target over duration
func (m *Model) fadeVolumeIn(target float64, durationSecs float64) {
	steps := 20
	interval := time.Duration(float64(durationSecs) / float64(steps) * float64(time.Second))
	stepSize := target / float64(steps)

	for i := 1; i <= steps; i++ {
		vol := stepSize * float64(i)
		if vol > target {
			vol = target
		}
		m.mpvBackend.SetVolume(vol)
		time.Sleep(interval)
	}
}

// checkOfflineRefill checks if we need to add more songs from the offline cache
func (m *Model) checkOfflineRefill() tea.Cmd {
	if !m.offlineMode {
		return nil
	}

	// Calculate next index to add
	nextIdx := len(m.songs)
	if nextIdx >= len(m.offlineSongs) {
		return nil // All songs already in playlist
	}

	// Refill when 2 or fewer songs remain in the playlist
	remainingInPlaylist := len(m.songs) - 1 - m.currentSongIndex
	if remainingInPlaylist <= 2 {
		m.refillOfflineBatch()
		m.updatePlaylist()
	}
	return nil
}

// refillOfflineBatch adds the next batch of songs from the offline cache to MPV's playlist
func (m *Model) refillOfflineBatch() {
	batchSize := 5
	nextIdx := len(m.songs)

	// Determine how many songs to add
	addCount := batchSize
	if nextIdx+addCount > len(m.offlineSongs) {
		addCount = len(m.offlineSongs) - nextIdx
	}

	if addCount <= 0 {
		return
	}

	var urls []string
	for i := 0; i < addCount; i++ {
		cs := m.offlineSongs[nextIdx+i]
		urls = append(urls, cs.AudioPath)
		s := cs.ToSong()
		m.songs = append(m.songs, s)
	}

	m.mpvBackend.AppendToPlaylist(urls)
	logger.Printf("Offline refill: added %d songs", addCount)
}

// handleBlockFetched handles the block fetched message
func (m Model) handleBlockFetched(msg blockFetchedMsg) (tea.Model, tea.Cmd) {
	// No-op in jukebox or offline mode
	if m.jukeboxMode || m.offlineMode {
		return m, nil
	}

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

		// Check if we should show network transition modal (going offline)
		// Show after 3 consecutive failures if not in jukebox mode
		if m.consecutiveFailures >= 3 && !m.jukeboxMode {
			offlineDir := filepath.Join(filepath.Dir(m.config.GetFavoritesDir()), "offline")
			caches, _ := cache.ListCaches(offlineDir)
			m.networkTransitionModal = modals.NewNetworkTransition(m.styles, NetworkGoingOffline, caches, errLabel)
			m.activeModal = ModalNetworkTransition
			logger.Printf("Showing network transition modal (going offline) after %d failures", m.consecutiveFailures)
			return m, nil
		}

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

		// In offline mode: check if we should show going-online modal
		// Only if we started offline mode without connection
		if m.offlineMode && !m.offlineModeStartedConnected && !m.jukeboxMode {
			m.networkTransitionModal = modals.NewNetworkTransition(m.styles, NetworkGoingOnline, nil, "")
			m.activeModal = ModalNetworkTransition
			logger.Printf("Showing network transition modal (going online) in offline mode")
		}
	}

	// If in offline mode and this is first successful connection, mark it
	if m.offlineMode && !m.offlineModeStartedConnected {
		m.offlineModeStartedConnected = true
	}

	// Check for promo block (blockID == 0) — skip in all cases
	// Exception: channel 99 (My Paradise) always returns blockID=0 with 1 song
	isChan99 := m.config.Channel == 99
	if msg.blockID == 0 && !isChan99 {
		logger.Printf("Skipping promo block (blockID=0)")
		if !m.initialized {
			return m, tea.Batch(setStatus(&m, "Waiting for stream...", false), m.fetchBlockCmd)
		}
		return m, m.fetchBlockCmd
	}

	// Channel 99 (My Paradise) returns a single-song "block" with no block ID.
	// We buffer 4 songs initially, then refill with 2 when the last song starts.
	if isChan99 {
		// The actual fetching happens asynchronously via fetchChan99Cmd
		// This path is only reached on the initial block fetch or poll tick
		if !m.initialized {
			return m, m.fetchChan99Cmd(4)
		}
		// Already initialized — this is a poll tick, trigger a refill
		return m, m.fetchChan99Cmd(2)
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
			logger.Printf("DEBUG: handleBlockFetched mpvWasStopped path, setting initialized=true")
			m.currentSongIndex = len(m.songs) // will point to first new song
			m.songs = append(m.songs, msg.songs...)
			// Stamp new songs with block ID for pruning
			for i := m.currentSongIndex; i < len(m.songs); i++ {
				m.songs[i].BlockID = int64(msg.blockID)
			}
			m.playlistStartIdx = m.currentSongIndex
			if err := m.mpvBackend.Start(urls); err != nil {
				logger.Printf("Failed to restart MPV: %v", err)
				return m, nil
			}
			m.isPlaying = true
			if !layoutPromptActive {
				m.initialized = true
			}
			m.connectedAt = time.Now()
		} else {
			// MPV still running - append to playlist
			if err := m.mpvBackend.AppendToPlaylist(urls); err != nil {
				logger.Printf("Failed to append to playlist: %v", err)
				return m, nil
			}
			oldLen := len(m.songs)
			m.songs = append(m.songs, msg.songs...)
			// Stamp new songs with block ID for pruning
			for i := oldLen; i < len(m.songs); i++ {
				m.songs[i].BlockID = int64(msg.blockID)
			}

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

	for i := range msg.songs {
		msg.songs[i].BlockID = int64(msg.blockID)
	}
	m.songs = msg.songs
	m.imageBase = msg.imageBase
	m.lastBlockID = msg.blockID
	m.currentSongIndex = 0
	m.playlistStartIdx = 0
	m.isPlaying = true
	m.pollingNextBlock = false
	m.connectedAt = time.Now()
	if !layoutPromptActive {
		m.initialized = true
	}

	return m, m.songChangedCmds()
}

// handleChan99Fetched processes channel 99 song fetch results
func (m Model) handleChan99Fetched(msg chan99FetchedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		if !m.initialized {
			return m, setStatus(&m, fmt.Sprintf("Failed to load My Paradise: %v", msg.err), true)
		}
		logger.Printf("Channel 99 fetch error: %v", msg.err)
		return m, nil
	}

	if !m.initialized {
		// Initial load — start playback with buffered songs
		urls := make([]string, len(msg.songs))
		for i, s := range msg.songs {
			urls[i] = s.GaplessURL
		}

		m.songs = msg.songs
		m.currentSongIndex = 0
		m.playlistStartIdx = 0
		m.lastBlockID = 0

		if err := m.mpvBackend.Start(urls); err != nil {
			logger.Printf("Failed to start MPV for channel 99: %v", err)
			return m, setStatus(&m, fmt.Sprintf("Failed to start playback: %v", err), true)
		}
		m.isPlaying = true
		if !layoutPromptActive {
			m.initialized = true
		}
		m.connectedAt = time.Now()

		m.updatePlaylist()
		return m, tea.Batch(
			setStatus(&m, "My Paradise", false),
			m.songChangedCmds(),
		)
	}

	// Refill — append to existing playlist
	urls := make([]string, len(msg.songs))
	for i, s := range msg.songs {
		urls[i] = s.GaplessURL
	}

	if err := m.mpvBackend.AppendToPlaylist(urls); err != nil {
		logger.Printf("Failed to append to channel 99 playlist: %v", err)
		return m, nil
	}
	m.songs = append(m.songs, msg.songs...)
	m.pollingNextBlock = false
	m.updatePlaylist()

	return m, setStatus(&m, "My Paradise", false)
}

// handleQuitTick handles the 60-second countdown to quit
func (m *Model) handleQuitTick(msg quitTickMsg) (tea.Model, tea.Cmd) {
	if !m.quittingActive {
		return m, nil
	}

	now := time.Now()
	elapsed := now.Sub(m.quittingStartedAt)
	remaining := 60 - int(elapsed.Seconds())

	if remaining <= 0 {
		if m.mpvBackend != nil {
			m.mpvBackend.Stop()
		}
		logger.Println("Quitting app after sleep timer")
		return m, tea.Quit
	}

	m.statusMsg = fmt.Sprintf("Sleep timer expired - quitting in %ds...", remaining)
	m.statusIsError = false
	m.statusSeq++

	return m, tickQuitCmd()
}

// handleSleepTimerTick handles sleep timer countdown updates (every minute)
func (m *Model) handleSleepTimerTick(msg sleepTimerTickMsg) (tea.Model, tea.Cmd) {
	if !m.sleepTimerActive {
		return m, nil
	}

	now := time.Now()

	// Check if timer has expired
	if now.After(m.sleepTimerExpiresAt) || now.Equal(m.sleepTimerExpiresAt) {
		m.sleepTimerActive = false
		if m.sleepTimerTicker != nil {
			m.sleepTimerTicker.Stop()
			m.sleepTimerTicker = nil
		}

		// Pause playback
		if err := m.mpvBackend.Pause(true); err != nil {
			logger.Printf("Error pausing for sleep timer: %v", err)
		}
		m.isPaused = true
		m.isPlaying = false

		// Start 60 second countdown to quit
		m.quittingActive = true
		m.quittingStartedAt = now
		m.quittingTicker = time.NewTicker(time.Second)

		logger.Println("Sleep timer expired, starting 60s countdown to quit")
		// Start the quit tick immediately
		return m, tea.Batch(setStatus(m, "Sleep timer expired - quitting in 60s...", false), tickQuitCmd())
	}

	// Re-arm the ticker
	return m, tickSleepTimerCmd()
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
		// MPV has stopped - if on last song, enable polling (skip in jukebox mode)
		if !m.jukeboxMode && m.currentSongIndex >= len(m.songs)-1 {
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
					if m.offlineMode {
						// In offline mode, just skip to end
						cmds = append(cmds, setStatus(&m, "Last song is blocklisted", false))
						return m, tea.Batch(cmds...)
					}
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
			// In jukebox mode, update played counter on transition
			if m.jukeboxMode {
				m.jukeboxPlayed++
				m.checkJukeboxRefill()
			}
			// In offline mode, refill from sequential queue
			if m.offlineMode {
				m.checkOfflineRefill()
			}
			cmds = append(cmds, m.songChangedCmds())
		}
	}

	// Jukebox mode: handle end of playlist
	if m.jukeboxMode && !m.mpvBackend.IsRunning() && !m.mpvBackend.IsPaused() && m.currentSongIndex >= len(m.songs)-1 {
		if len(m.jukeboxQueue) > 0 || m.config.Jukebox.Repeat {
			// More songs available or repeat enabled - refill and continue
			if len(m.jukeboxQueue) == 0 && m.config.Jukebox.Repeat {
				m.reshuffleJukebox()
			}
			if len(m.jukeboxQueue) > 0 {
				m.refillJukeboxBatch()
				m.updatePlaylist()
			}
		} else {
			// No more songs and repeat disabled
			cmds = append(cmds, setStatus(&m, fmt.Sprintf("Jukebox complete: played %d songs", m.jukeboxTotal), false))
			return m, tea.Batch(cmds...)
		}
	}

	// Offline mode: handle end of playlist
	if m.offlineMode && !m.mpvBackend.IsRunning() && !m.mpvBackend.IsPaused() && m.currentSongIndex >= len(m.songs)-1 {
		if len(m.songs) < len(m.offlineSongs) {
			// More songs in cache - refill
			m.refillOfflineBatch()
			m.updatePlaylist()
		} else {
			// All songs played
			cmds = append(cmds, setStatus(&m, fmt.Sprintf("Offline cache complete: %d songs", len(m.offlineSongs)), false))
			return m, tea.Batch(cmds...)
		}
	}

	// Jukebox mode: pseudo-crossfade volume ramp
	if m.jukeboxMode && m.config.Jukebox.CrossfadeDuration > 0 && !m.isPaused && err == nil {
		crossfadeDur := m.config.Jukebox.CrossfadeDuration
		songDuration := float64(m.currentSong.Duration) / 1000.0
		timeRemaining := songDuration - m.playbackPos.TimePos

		if timeRemaining <= crossfadeDur && timeRemaining > 0 {
			// Fade out: ramp volume down as song approaches end
			fadePercent := timeRemaining / crossfadeDur
			vol := fadePercent * 100.0
			if vol < 5 {
				vol = 5
			}
			m.mpvBackend.SetVolume(vol)
			m.crossfading = true
		} else if m.crossfading && timeRemaining > crossfadeDur {
			// Song just transitioned - fade back in
			m.crossfading = false
			go m.fadeVolumeIn(100.0, m.config.Jukebox.CrossfadeDuration)
		}
	}

	// Start polling when on last song with ≤2 min remaining (skip in jukebox and offline mode)
	if !m.jukeboxMode && !m.offlineMode && !m.pollingNextBlock && m.currentSongIndex >= len(m.songs)-1 && m.currentSong != nil && err == nil {
		songDuration := float64(m.currentSong.Duration) / 1000.0
		timeRemaining := songDuration - m.playbackPos.TimePos
		if timeRemaining <= 120 && timeRemaining > 0 {
			logger.Printf("Last song with %.0fs remaining, starting to poll for next block", timeRemaining)
			m.pollingNextBlock = true
		}
	}

	// MPV stopped on last song - ensure polling is active and start spinner (skip in jukebox and offline mode)
	if !m.jukeboxMode && !m.offlineMode && !m.mpvBackend.IsRunning() && !m.mpvBackend.IsPaused() && m.currentSongIndex >= len(m.songs)-1 {
		if !m.pollingNextBlock {
			logger.Printf("MPV stopped on last song, enabling polling")
			m.pollingNextBlock = true
			cmds = append(cmds, m.spinner.Tick)
		}
	}

	// Check if we should auto-queue a favorite (skip in offline mode)
	if !m.offlineMode {
		if cmd := m.checkAndQueueFavorite(); cmd != nil {
			cmds = append(cmds, cmd)
		}
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

	// Handle per-service scrobble flash states (timeout and blink)
	for service, state := range m.scrobbleStates {
		if state == flashSolid {
			if time.Since(m.scrobbleFlashAt) >= flashDuration {
				m.scrobbleStates[service] = flashOff
			}
		} else if state == flashBlinkOn || state == flashBlinkOff {
			if time.Since(m.scrobbleFlashAt) >= flashDuration {
				m.scrobbleStates[service] = flashOff
			} else {
				elapsed := time.Since(m.scrobbleFlashAt)
				if int(elapsed.Seconds())%2 == 0 {
					m.scrobbleStates[service] = flashBlinkOn
				} else {
					m.scrobbleStates[service] = flashBlinkOff
				}
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
	//
	// For halfblocks protocol: the mosaic library used by go-termimg takes dimensions
	// in PIXELS (not cells). Each character cell is 2x2 pixels, so we must double
	// the target cell dimensions to get the correct pixel dimensions.
	// For other protocols (sixel, iterm2), we apply cellRatio normally.
	const targetHeight = 16
	var width, height int
	if m.imageProtocol == termimg.Halfblocks {
		// Halfblocks: mosaic uses pixels (2 per cell dimension)
		// Double both dimensions: width = target_cols * 2, height = target_rows * 2
		targetWidth := int(float64(targetHeight) * m.cellRatio)
		if targetWidth < 10 {
			targetWidth = 10
		}
		width = targetWidth * 2   // pixels
		height = targetHeight * 2 // pixels
	} else {
		height = targetHeight
		width = int(float64(height) * m.cellRatio)
		if width < 10 {
			width = 10
		}
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

	logger.Printf("Album art loaded for: %s (len=%d, w=%d, h=%d)", m.currentSong.Title, len(rendered), width, height)
	m.albumArtStr = rendered
	m.albumArtLoaded = true
	// Store dimensions in cells for positioning
	if m.imageProtocol == termimg.Halfblocks {
		m.albumArtWidth = width / 2
		m.albumArtHeight = height / 2
	} else {
		m.albumArtWidth = width
		m.albumArtHeight = height
	}

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

// handleCommentsFetched processes the comments fetch result
// When in comments view, goes to pending (like lyrics/artist).
// When NOT in comments view, auto-populates for next time user enters.
func (m Model) handleCommentsFetched(msg commentsFetchedMsg) (tea.Model, tea.Cmd) {
	if m.currentSong == nil || msg.songID != m.currentSong.SongID {
		return m, nil
	}

	if msg.err != nil {
		m.commentsStatus = "Failed to load comments. Check log."
		logger.Printf("Comments fetch failed: %v", msg.err)
		if m.bottomViewMode == ViewComments {
			m.updateBottomView()
		}
		return m, nil
	}

	if m.bottomViewMode == ViewComments {
		// Check if this is for the same song we're currently displaying
		if msg.songID == m.commentsSongID {
			// Same song — either initial or load more
			m.commentsStatus = ""
			if len(msg.comments) == 0 {
				m.commentsStatus = "No comments for this song"
			}
			if msg.loadMore {
				// Appending more comments — keep current page position
				m.comments = msg.comments
				m.commentsLoaded = !msg.more
			} else {
				// Initial fetch — reset page
				m.comments = msg.comments
				m.commentsPage = 0
				m.commentsLoaded = !msg.more
			}
			m.commentsTotal = msg.total
			m.updateBottomView()
			m.viewport.GotoTop()
		} else {
			// Different song (song changed while viewing) — store in pending
			m.pendingComments = msg.comments
			m.pendingCommentsTotal = msg.total
			m.pendingCommentsMore = msg.more
			m.pendingCommentsOffset = msg.offset
			m.updateBottomView()
		}
	} else {
		// Not viewing comments — store for next time user enters
		m.comments = msg.comments
		m.commentsStatus = ""
		m.commentsTotal = msg.total
		m.commentsPage = 0
		m.commentsSongID = msg.songID
		if len(msg.comments) == 0 {
			m.commentsStatus = "No comments for this song"
		}
	}

	return m, nil
}

// prunePlaylist removes old songs from previous blocks, keeping up to 3 before
// the currently playing song for prev-song functionality.
// Only prunes songs that belong to an older block than the current song.
// For channel 99 (no real blocks), prunes by position only.
func (m *Model) prunePlaylist() {
	if m.currentSongIndex < 0 || m.currentSongIndex >= len(m.songs) {
		return
	}

	// Channel 99: simple position-based prune (no BlockID available)
	if m.config.Channel == 99 {
		keepStart := m.currentSongIndex - 3
		if keepStart <= 0 {
			return
		}
		pruneEnd := keepStart
		m.songs = m.songs[pruneEnd:]
		m.currentSongIndex -= pruneEnd
		m.playlistStartIdx -= pruneEnd
		logger.Printf("Channel 99: pruned %d old songs, currentIdx=%d", pruneEnd, m.currentSongIndex)
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
		return hasInfo && hasArt
	}
	return false
}

// applyPendingUpdate copies pending data into the displayed fields and
// refreshes the current view. Called when the user presses 'u'.
// Returns a command to redraw images if needed.
func (m *Model) applyPendingUpdate() tea.Cmd {
	if m.currentSong == nil || m.pendingEventID != m.currentSong.EventID {
		return nil
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

	// Trigger artist thumbnail redraw if in artist view
	if m.bottomViewMode == ViewArtist && m.artistArtLoaded && m.artistArtStr != "" {
		return renderArtistArtAfterDelay()
	}
	return nil
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
		if m.offlineMode {
			content = "  Lyrics not available in offline mode"
		} else if m.lyrics == "" {
			content = "  Loading lyrics..."
		} else {
			content = indentLines(m.lyrics, "  ") + strings.Repeat("\n", 10)
			if m.hasPendingUpdate() {
				content += "\n  \x1b[3m(press 'u' to update)\x1b[0m"
			}
		}

	case ViewSyncedLyrics:
		if m.offlineMode {
			content = "  Synced lyrics not available in offline mode"
		} else if len(m.syncedLyrics) == 0 {
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
		if m.offlineMode {
			content = "  Artist info not available in offline mode"
		} else {
			// Narrow viewport width when artist image is displayed beside it
			if m.artistArtLoaded && m.artistArtStr != "" {
				imgGap := m.artistArtWidth + 5
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
				// Album description (from TADB album search)
				if m.artistInfo.AlbumDescription != "" {
					lines = append(lines, "")
					lines = append(lines, indent+m.artistInfo.AlbumDescription)
					if m.artistInfo.AlbumSource != "" {
						lines = append(lines, indent+"Source: "+m.artistInfo.AlbumSource)
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
		}

	case ViewComments:
		if !m.rpAPI.IsAuthenticated() {
			content = "  Comments require RP authentication"
		} else if m.commentsStatus != "" {
			content = "  " + m.commentsStatus
		} else if len(m.comments) == 0 {
			content = "  Loading comments..."
		} else {
			var lines []string
			// Calculate visible range for current page
			start := m.commentsPage * m.commentsPerPage
			if start >= len(m.comments) {
				// If we're loading more comments, show a loading indicator instead of resetting
				if m.commentsStatus != "" {
					content = "  " + m.commentsStatus
					m.viewport.SetContent(content)
					return
				}
				start = 0
				m.commentsPage = 0
			}
			end := start + m.commentsPerPage
			if end > len(m.comments) {
				end = len(m.comments)
			}
			visibleComments := m.comments[start:end]

			for _, c := range visibleComments {
				// Format: <time> <username>
				// <quoted text if any>
				// <message>
				timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Accent))
				userStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Cursor))
				header := timeStyle.Render(c.PostedTime) + " " + userStyle.Render(c.Username)
				lines = append(lines, header)
				if c.QuotedText != "" {
					for _, qLine := range strings.Split(c.QuotedText, "\n") {
						lines = append(lines, "  "+m.styles.MutedStyle.Render("> "+qLine))
					}
				}
				for _, mLine := range strings.Split(c.Message, "\n") {
					lines = append(lines, "  "+mLine)
				}
				lines = append(lines, "")
			}
			// Show pagination info
			if m.commentsTotal > 0 {
				totalPages := (m.commentsTotal - 1) / m.commentsPerPage
				currentPage := m.commentsPage + 1
				pageStart := start + 1
				pageEnd := end
				lines = append(lines, "")
				if totalPages > 0 {
					nav := fmt.Sprintf("Page %d/%d (%d-%d/%d comments)", currentPage, totalPages+1, pageStart, pageEnd, m.commentsTotal)
					if !m.commentsLoaded {
						nav += " — press 'l' to load more"
					}
					if m.commentsPage > 0 {
						nav += ", 'P' for previous"
					}
					lines = append(lines, m.styles.MutedStyle.Render(nav))
				} else {
					lines = append(lines, m.styles.MutedStyle.Render(fmt.Sprintf("%d comments", len(m.comments))))
				}
			}
			// Show pending update indicator
			if m.pendingComments != nil {
				lines = append(lines, "")
				lines = append(lines, m.styles.MutedStyle.Render("(press 'u' to update to current song)"))
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

	// Clear comments for the new song, unless currently viewing them
	if m.bottomViewMode != ViewComments {
		m.comments = nil
		m.commentsStatus = ""
		m.commentsPage = 0
		m.commentsSongID = 0
	}

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

	// Auto-download RP favorites if enabled
	if !m.offlineMode && !m.jukeboxMode && m.rpAPI.IsAuthenticated() && m.config.AutoDownloadRPFavorites && m.currentSong != nil && m.currentSong.SongID != 0 {
		if !m.cacheManager.IsFavorite(m.currentSong) && m.downloadingFav != m.currentSong.EventID {
			cutoff := m.authClient.Chan99Cutoff()
			userRating := 0
			if m.currentSong.UserRating != "" && m.currentSong.UserRating != "0" {
				fmt.Sscanf(m.currentSong.UserRating, "%d", &userRating)
			}
			if userRating >= cutoff {
				m.downloadingFav = m.currentSong.EventID
				m.updatePlaylist()
				logger.Printf("Auto-downloading RP favorite: %s - %s (rating %d >= cutoff %d)", m.currentSong.Artist, m.currentSong.Title, userRating, cutoff)
				cmds = append(cmds, favoriteDownloadCmd(m.cacheManager, m.currentSong, m.rpAPI.GetFileExtension(), m.downloadResults))
			}
		}
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

	// Fetch lyrics and artist info (skip in offline mode - no API lookups)
	if !m.offlineMode {
		cmds = append(cmds, m.fetchLyricsCmd())
		cmds = append(cmds, m.fetchArtistCmd())
		// Always fetch comments for new song
		// When in comments view, they go to pending so user can press 'u' to update
		cmds = append(cmds, m.fetchCommentsCmd())
	}

	// Channel 99: refill when the last song in the playlist starts playing
	if m.config.Channel == 99 && m.currentSongIndex == len(m.songs)-1 {
		logger.Printf("Channel 99: last song started, refilling 2 more songs")
		cmds = append(cmds, m.fetchChan99Cmd(2))
	}

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

// fetchChan99Cmd returns a command that fetches n songs from channel 99
func (m Model) fetchChan99Cmd(n int) tea.Cmd {
	return fetchChan99Cmd(m.rpAPI, n)
}

// fetchCommentsCmd fetches comments for the current song from RP
func (m Model) fetchCommentsCmd() tea.Cmd {
	if m.currentSong == nil || m.currentSong.SongID == 0 {
		return nil
	}
	if !m.rpAPI.IsAuthenticated() {
		return nil
	}

	songID := m.currentSong.SongID
	perPage := m.commentsPerPage
	if perPage == 0 {
		perPage = 20
	}
	return func() tea.Msg {
		resp, err := m.commentsClient.GetComments(songID, perPage, "oldest")
		if err != nil {
			return commentsFetchedMsg{songID: songID, err: err}
		}
		comments := make([]*api.Comment, len(resp.Comments))
		for i := range resp.Comments {
			comments[i] = &resp.Comments[i]
		}
		return commentsFetchedMsg{
			songID:   songID,
			comments: comments,
			total:    resp.TotalComments,
			more:     resp.MoreComments,
			offset:   resp.MoreOffset,
			loadMore: false,
		}
	}
}

// fetchCommentsPageCmd fetches a specific page of comments
func (m Model) fetchCommentsPageCmd(page int) tea.Cmd {
	if m.currentSong == nil || m.currentSong.SongID == 0 {
		return nil
	}
	if !m.rpAPI.IsAuthenticated() {
		return nil
	}

	songID := m.currentSong.SongID
	perPage := m.commentsPerPage
	if perPage == 0 {
		perPage = 20
	}
	offset := page * perPage
	existing := m.comments
	existingTotal := m.commentsTotal

	return func() tea.Msg {
		resp, err := m.commentsClient.GetCommentsWithOffset(songID, perPage, "oldest", offset)
		if err != nil {
			return commentsFetchedMsg{songID: songID, err: err}
		}
		comments := make([]*api.Comment, len(resp.Comments))
		for i := range resp.Comments {
			comments[i] = &resp.Comments[i]
		}
		// Append to existing comments
		allComments := append(existing, comments...)
		return commentsFetchedMsg{
			songID:   songID,
			comments: allComments,
			total:    existingTotal,
			more:     resp.MoreComments,
			offset:   resp.MoreOffset,
			loadMore: true,
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
			a, e := tadbClient.SearchArtist(ctx, song.Artist, song.Album)
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
			// Album info (description + sales) from album search
			if tadb.artist.AlbumInfo != nil {
				if tadb.artist.AlbumInfo.Description != "" {
					info.AlbumDescription = tadb.artist.AlbumInfo.Description
					info.AlbumSource = "theaudiodb"
				}
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
			discogsArtist, err := discogsClient.SearchArtist(ctx, song.Artist, song.Album)
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

// loadImageCmd loads an image from URL or local file path
func (m Model) loadImageCmd(path string) tea.Cmd {
	if m.currentSong == nil {
		return nil
	}

	eventID := m.currentSong.EventID
	songTitle := m.currentSong.Title
	return func() tea.Msg {
		var data []byte
		var err error

		// Check if it's a local file path
		if _, fileErr := os.Stat(path); fileErr == nil {
			logger.Printf("Loading album art from file: %s for %s", path, songTitle)
			data, err = os.ReadFile(path)
		} else {
			logger.Printf("Fetching album art: %s for %s", path, songTitle)
			resp, httpErr := http.Get(path)
			if httpErr != nil {
				logger.Printf("Album art fetch error: %v", httpErr)
				return imageLoadedMsg{eventID: eventID, err: httpErr}
			}
			defer resp.Body.Close()
			data, err = io.ReadAll(resp.Body)
		}

		if err != nil {
			logger.Printf("Album art read error: %v", err)
			return imageLoadedMsg{eventID: eventID, err: err}
		}

		logger.Printf("Album art loaded: %s, %d bytes", songTitle, len(data))
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
	// For halfblocks: mosaic uses pixels (2 per cell), so double dimensions.
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

	// Calculate pixel dimensions for the renderer
	var renderWidth, renderHeight int
	if m.imageProtocol == termimg.Halfblocks {
		// Mosaic uses pixels (2 per cell dimension), so double
		renderWidth = displayWidth * 2
		renderHeight = displayHeight * 2
	} else {
		renderWidth = displayWidth
		renderHeight = displayHeight
	}

	logger.Printf("Artist thumbnail sizing: src=%dx%d, display=%dx%d cells, render=%dx%d, cellRatio=%.2f",
		imgBounds.Dx(), imgBounds.Dy(), displayWidth, displayHeight, renderWidth, renderHeight, m.cellRatio)

	tiImg := termimg.New(img).
		Size(renderWidth, renderHeight).
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
			width:    displayWidth,  // store in cells
			height:   displayHeight, // store in cells
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
		// Check terminal size against initial layout (only once)
		// Use package-level variables to persist across View() calls
		if !layoutCheckDone && m.width > 0 && m.height > 0 {
			fits, suboptimal, warning, _ := checkTerminalSize(m.width, m.height, m.initialLayout)
			if !fits || suboptimal {
				// Store state in package variables for persistence across renders
				layoutCheckDone = true
				layoutPromptActive = true
				layoutPromptWidth = m.width
				layoutPromptHeight = m.height
				layoutPromptLayout = m.initialLayout
				fittingLayouts := getFittingLayouts(m.width, m.height)

				// Build prompt with initial choice + warning + fitting options + quit
				var prompt string
				if warning == "terminal too narrow or too short" {
					prompt = fmt.Sprintf("Terminal size: %dx%d (requires %dx%d for %s)\n\n",
						m.height, m.width,
						layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].minCols,
						m.initialLayout)
				} else if warning != "" {
					prompt = fmt.Sprintf("Terminal size: %dx%d (%s recommended %dx%d)\n\n",
						m.height, m.width, m.initialLayout,
						layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].recCols)
					prompt += warning + "\n\n"
				} else {
					prompt = fmt.Sprintf("Terminal size: %dx%d (%s recommended %dx%d)\n\n",
						m.height, m.width, m.initialLayout,
						layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].recCols)
				}

				// Build option string: initial choice (with warning) + fitting layouts + quit
				opts := make([]string, 0, len(fittingLayouts)+2)

				// Add initial choice with warning prefix
				initialWarn := ""
				if warning == "terminal too narrow or too short" {
					initialWarn = " (display problems expected)"
				} else if warning != "" {
					initialWarn = " (may have display issues)"
				}
				initialOpt := m.styles.AccentStyle.Render(strings.ToLower(m.initialLayout[:1])) + " " +
					m.styles.MutedStyle.Render(m.initialLayout) + initialWarn
				opts = append(opts, initialOpt)

				// Add other fitting layouts
				for _, l := range fittingLayouts {
					if l != m.initialLayout {
						opt := m.styles.AccentStyle.Render(strings.ToLower(l[:1])) + " " +
							m.styles.MutedStyle.Render(l)
						opts = append(opts, opt)
					}
				}
				opts = append(opts, m.styles.AccentStyle.Render("q")+" "+m.styles.MutedStyle.Render("quit"))

				prompt += m.styles.MutedStyle.Render("Press a key to select") + "\n\n"
				prompt += strings.Join(opts, ", ")
				return m.altView(prompt)
			}
			// Fits well, mark as done so we don't check again
			layoutCheckDone = true
		}

		// If we're waiting for user to choose layout, show prompt
		if layoutPromptActive && m.width > 0 && m.height > 0 {
			fittingLayouts := getFittingLayouts(m.width, m.height)
			_, _, warning, _ := checkTerminalSize(m.width, m.height, m.initialLayout)

			// Build prompt with initial choice + warning + fitting options + quit
			var prompt string
			if warning == "terminal too narrow or too short" {
				prompt = fmt.Sprintf("Terminal size: %dx%d (requires %dx%d for %s)\n\n",
					m.height, m.width,
					layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].minCols,
					m.initialLayout)
			} else if warning != "" {
				prompt = fmt.Sprintf("Terminal size: %dx%d (%s recommended %dx%d)\n\n",
					m.height, m.width, m.initialLayout,
					layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].recCols)
				prompt += warning + "\n\n"
			} else {
				prompt = fmt.Sprintf("Terminal size: %dx%d (%s recommended %dx%d)\n\n",
					m.height, m.width, m.initialLayout,
					layoutReqs[m.initialLayout].minRows, layoutReqs[m.initialLayout].recCols)
			}

			// Build option string: initial choice (with warning) + fitting layouts + quit
			opts := make([]string, 0, len(fittingLayouts)+2)

			// Add initial choice with warning prefix
			initialWarn := ""
			if warning == "terminal too narrow or too short" {
				initialWarn = " (display problems expected)"
			} else if warning != "" {
				initialWarn = " (may have display issues)"
			}
			initialOpt := m.styles.AccentStyle.Render(strings.ToLower(m.initialLayout[:1])) + " " +
				m.styles.MutedStyle.Render(m.initialLayout) + initialWarn
			opts = append(opts, initialOpt)

			// Add other fitting layouts
			for _, l := range fittingLayouts {
				if l != m.initialLayout {
					opt := m.styles.AccentStyle.Render(strings.ToLower(l[:1])) + " " +
						m.styles.MutedStyle.Render(l)
					opts = append(opts, opt)
				}
			}
			opts = append(opts, m.styles.AccentStyle.Render("q")+" "+m.styles.MutedStyle.Render("quit"))

			prompt += m.styles.MutedStyle.Render("Press a key to select") + "\n\n"
			prompt += strings.Join(opts, ", ")
			return m.altView(prompt)
		}

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
		case ModalStationWarning:
			if m.stationWarningModal != nil {
				modalView = m.stationWarningModal.View()
			}
		case ModalRating:
			if m.ratingModal != nil {
				modalView = m.ratingModal.View()
			}
		case ModalNetworkTransition:
			if m.networkTransitionModal != nil {
				modalView = m.networkTransitionModal.View()
			}
		case ModalSleepTimer:
			if m.sleepTimerModal != nil {
				modalView = m.sleepTimerModal.View()
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

	// 1. Header - render after NowPlaying width is configured for proper centering in compact mode
	// (moved from above to after width configuration below)

	// Determine display info based on mode
	offlineCacheInfo := ""
	if m.offlineMode {
		stationName := config.StationNames[m.offlineStation]
		if stationName == "" {
			stationName = fmt.Sprintf("Station %d", m.offlineStation)
		}
		bitrateName := config.BitrateNames[m.offlineBitrate]
		if bitrateName == "" {
			bitrateName = fmt.Sprintf("Bitrate %d", m.offlineBitrate)
		}
		offlineCacheInfo = fmt.Sprintf("%s • %s", stationName, bitrateName)
	}

	// Configure NowPlaying width and truncation based on layout
	showBottomSection := m.layoutMode == LayoutLarge
	isNarrow := m.layoutMode == LayoutNarrow
	isCompact := m.layoutMode == LayoutCompact

	if isNarrow {
		// Narrow: limit to album art width so text doesn't wrap
		artHeight := 16
		artWidth := int(float64(artHeight) * m.cellRatio)
		if artWidth < 10 {
			artWidth = 10
		}
		m.nowPlayingWidget.SetWidth(min(m.width-4, artWidth))
		m.nowPlayingWidget.SetMaxWidth(artWidth)
		m.nowPlayingWidget.SetContentWidth(0) // No limit - album art is above content
	} else if isCompact {
		// Compact: no album art, full width but truncate long text
		m.nowPlayingWidget.SetWidth(m.width - 4)
		m.nowPlayingWidget.SetMaxWidth(m.width - 6)
		m.nowPlayingWidget.SetContentWidth(0) // No album art, no limit needed
	} else {
		// Large and medium: full width, no truncation
		m.nowPlayingWidget.SetWidth(m.width - 4)
		m.nowPlayingWidget.SetMaxWidth(0)
		// Set content width to prevent "clear to end of line" from slicing album art
		if m.config.ShowAlbumArt {
			artHeight := 16
			artWidth := int(float64(artHeight) * m.cellRatio)
			if artWidth < 10 {
				artWidth = 10
			}
			artCol := m.width - artWidth - 2
			if artCol > 10 {
				// Leave 2 char margin before album art
				contentWidth := artCol - 2
				m.nowPlayingWidget.SetContentWidth(contentWidth)
			} else {
				m.nowPlayingWidget.SetContentWidth(0)
			}
		} else {
			m.nowPlayingWidget.SetContentWidth(0)
		}
	}

	// Determine RP favorites indicator (needed for NowPlaying view)
	rpFavIndicator := m.getRPFavoriteIndicator()

	// Update sleep timer display on widget
	if m.sleepTimerActive {
		remaining := m.sleepTimerExpiresAt.Sub(time.Now())
		mins := int(remaining.Minutes()) + 1
		if mins < 0 {
			mins = 0
		}
		m.nowPlayingWidget.SetSleepTimer(true, mins)
	} else {
		m.nowPlayingWidget.SetSleepTimer(false, 0)
	}

	// First render NowPlaying to get actual content width
	nowPlayingView := m.nowPlayingWidget.View(
		m.currentSong,
		m.isPaused,
		m.playbackPos.TimePos,
		m.connectedAt,
		m.config.GetDisplayInfo(),
		m.getFavoriteCount(),
		m.currentSong != nil && m.cacheManager.IsFavorite(m.currentSong),
		m.getSkipsAvailable(),
		m.getPrevAvailable(),
		m.statusMsg,
		m.statusIsError,
		m.connErrorMsg,
		m.config.MinFavorites,
		len(m.favoritesQueue),
		m.jukeboxMode,
		m.jukeboxPlayed,
		m.jukeboxTotal,
		m.offlineMode,
		offlineCacheInfo,
		m.getUserRating(),
		rpFavIndicator,
		m.theme.Cursor,
	)

	// For compact/narrow layouts, center header over NowPlaying widget's actual width
	if isCompact || isNarrow {
		nowPlayingLines := strings.Split(nowPlayingView, "\n")
		actualWidth := 0
		for _, line := range nowPlayingLines {
			w := lipgloss.Width(line)
			if w > actualWidth {
				actualWidth = w
			}
		}
		if actualWidth > 0 {
			m.headerWidget.SetWidth(actualWidth)
		}
	}

	// 1. Header (rendered after width configuration for correct centering in compact mode)
	header := m.headerWidget.View()

	// Write header to buffer
	var b strings.Builder
	b.WriteString(header + "\n\n")

	// For narrow layout: reserve space for album art at top-left
	// by prepending blank lines before NowPlaying
	// All protocols use tea.Raw for image rendering, so we just reserve space here
	if isNarrow {
		artHeight := 16
		for i := 0; i < artHeight+1; i++ {
			b.WriteString("\n")
		}
	}

	// Now-playing info - compact/narrow use less vertical spacing
	if isCompact || isNarrow {
		// Remove one trailing blank line for compact/narrow layouts
		b.WriteString(strings.TrimSuffix(nowPlayingView, "\n\n") + "\n")
	} else {
		b.WriteString(nowPlayingView + "\n\n")
	}

	// Show spinner animation when waiting for new songs AND MPV has stopped
	// Only show when truly at end of content, not while still playing and polling for next block
	if m.pollingNextBlock && !m.mpvBackend.IsRunning() && !m.jukeboxMode && !m.offlineMode {
		b.WriteString(m.styles.MutedStyle.Render("Ahead of livestream. Awaiting new songs"+m.spinner.View()+" ") + "\n")
	}

	// Footer rendering
	m.footerWidget.SetFlashStateByService(m.scrobbleStates)
	m.footerWidget.SetJukeboxMode(m.jukeboxMode)
	m.footerWidget.SetOfflineMode(m.offlineMode, m.offlineCache)
	m.footerWidget.SetMiniMode(isCompact || isNarrow)
	m.footerWidget.SetConnectionState(m.connState)
	// For compact/narrow layouts, center footer over NowPlaying widget's actual width
	// For large/medium, use full width
	if isCompact || isNarrow {
		// Calculate actual width from rendered NowPlaying content
		nowPlayingLines := strings.Split(nowPlayingView, "\n")
		actualWidth := 0
		for _, line := range nowPlayingLines {
			w := lipgloss.Width(line)
			if w > actualWidth {
				actualWidth = w
			}
		}
		m.footerWidget.SetWidth(actualWidth)
	} else {
		m.footerWidget.SetWidth(m.width)
	}
	footer := m.footerWidget.View()
	footerHeight := lipgloss.Height(footer)

	// The bottom section should fill the remaining space except for the footer
	currentHeight := lipgloss.Height(b.String())

	remainingHeight := m.height - currentHeight - footerHeight

	// Sync viewport height to actual available space so scroll math is correct
	if showBottomSection && m.bottomViewMode != ViewPlaylist && m.bottomViewMode != ViewOff && remainingHeight > 0 {
		m.viewport.SetHeight(remainingHeight)
	}

	// 3. Bottom Section (Playlist, Visualizer, or other) — only in large layout
	if showBottomSection {
		var bottomSection string
		if m.bottomViewMode == ViewPlaylist {
			if remainingHeight > 0 {
				m.playlistWidget.SetSize(m.width, remainingHeight)
			}
			bottomSection = m.playlistWidget.View()
		} else if m.bottomViewMode == ViewVisualizer && m.vis != nil {
			m.vis.SetRows(max(3, remainingHeight))
			if m.vis.AudioReady() {
				bottomSection = m.vis.Render(m.width)
			} else {
				// Show loading message while audio tap connects
				modeName := m.vis.ModeName()
				source := m.vis.AudioSource()
				available := m.vis.AvailableSamples()
				lines := []string{
					"",
					fmt.Sprintf("Loading %s visualization...", modeName),
					fmt.Sprintf("Connecting to %s audio...", source),
					fmt.Sprintf("Samples available: %d (need %d)", available, 2048),
					"",
				}
				bottomSection = strings.Join(lines, "\n")
			}
		} else if m.bottomViewMode != ViewOff {
			viewportContent := m.viewport.View()
			// Offset viewport to the right when artist image is beside it
			if m.bottomViewMode == ViewArtist && m.artistArtLoaded && m.artistArtStr != "" {
				// Check if space was sufficient for thumbnail rendering
				availableSpace := m.height - 20 - 3
				if availableSpace >= m.artistArtHeight {
					// Artist thumbnail is rendered via tea.Raw() in renderImagesCmd()
					// Just pad viewport content to the right of the image
					leftPad := strings.Repeat(" ", m.artistArtWidth+5)
					vpLines := strings.Split(viewportContent, "\n")
					for i, line := range vpLines {
						vpLines[i] = leftPad + line
					}
					viewportContent = strings.Join(vpLines, "\n")
				} else {
					// Not enough space - show fallback message instead
					viewportContent = "Increase terminal height to view artist info and image."
				}
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
	}

	if showBottomSection {
		b.WriteString("\n")
	}
	b.WriteString(footer)

	return m.altView(b.String())
}

// positionMultiLineImage positions a multi-line image string at the specified row and column.
// This is needed for protocols like halfblocks that output multiple lines with \n separators.
// Each \n in the output resets the cursor to column 1, so we must position each line individually.
func positionMultiLineImage(imgStr string, startRow, startCol int) string {
	lines := strings.Split(imgStr, "\n")
	var b strings.Builder
	for i, line := range lines {
		if line != "" {
			b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s", startRow+i, startCol, line))
		}
	}
	return b.String()
}

// renderImagesCmd returns a tea.Cmd that sends all terminal images (album art
// and artist thumbnail) via tea.Raw. All protocols use tea.Raw to bypass
// bubbletea's StyledString which doesn't support cursor positioning escapes.
func (m Model) renderImagesCmd() tea.Cmd {
	// Suppress all images when layout prompt is active
	if layoutPromptActive {
		return nil
	}

	if m.activeModal != ModalNone {
		return nil
	}

	// Suppress all images when in fullscreen visualizer
	if m.visFullscreen && m.bottomViewMode == ViewVisualizer {
		return nil
	}

	hasAlbumArt := m.config.ShowAlbumArt && m.albumArtLoaded && m.albumArtStr != "" && m.layoutMode != LayoutCompact
	hasArtistArt := m.artistArtLoaded && m.artistArtStr != "" && m.bottomViewMode == ViewArtist

	if !hasAlbumArt && !hasArtistArt {
		return nil
	}

	var raw string

	// Kitty needs ClearAll to remove previous placements
	if m.imageProtocol == termimg.Kitty {
		raw = termimg.ClearAllString()
	} else {
		raw = ""
	}

	if hasAlbumArt {
		artHeight := 16
		artWidth := int(float64(artHeight) * m.cellRatio)
		if artWidth < 10 {
			artWidth = 10
		}
		var artCol int
		if m.layoutMode == LayoutNarrow {
			artCol = 2 // left-aligned for narrow layout
		} else {
			artCol = m.width - artWidth - 2
			if artCol < 1 {
				artCol = 1
			}
		}
		// For Kitty: single escape sequence, position directly
		// For other protocols (halfblocks/sixel/iterm2): may contain newlines, position each line
		if m.imageProtocol == termimg.Kitty {
			raw += fmt.Sprintf("\x1b[s\x1b[3;%dH%s\x1b[u", artCol, m.albumArtStr)
		} else {
			raw += "\x1b[s" + positionMultiLineImage(m.albumArtStr, 3, artCol) + "\x1b[u"
		}
	}

	if hasArtistArt {
		// Check if there's enough space: available = height - start_row(20) - footer_estimate(3)
		availableSpace := m.height - 20 - 3
		if availableSpace >= m.artistArtHeight {
			// Bottom section starts after: header(1) + gap(2) + nowPlaying(15 lines) + gap(2) = row 20
			if m.imageProtocol == termimg.Kitty {
				raw += fmt.Sprintf("\x1b[s\x1b[%d;%dH%s\x1b[u", 20, 2, m.artistArtStr)
			} else {
				raw += "\x1b[s" + positionMultiLineImage(m.artistArtStr, 20, 2) + "\x1b[u"
			}
		}
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

// getUserRating returns the current song's user rating if authenticated, empty string otherwise
func (m Model) getUserRating() string {
	if m.currentSong == nil || !m.rpAPI.IsAuthenticated() {
		return ""
	}
	return m.currentSong.UserRating
}

// getRPFavoriteIndicator returns an indicator emoji when a song is an RP favorite
// but not yet downloaded locally, and auto-download is disabled
func (m Model) getRPFavoriteIndicator() string {
	if m.currentSong == nil || !m.rpAPI.IsAuthenticated() {
		return ""
	}
	if m.config.AutoDownloadRPFavorites {
		return ""
	}
	if m.cacheManager.IsFavorite(m.currentSong) {
		return ""
	}
	cutoff := m.authClient.Chan99Cutoff()
	userRating := 0
	if m.currentSong.UserRating != "" && m.currentSong.UserRating != "0" {
		fmt.Sscanf(m.currentSong.UserRating, "%d", &userRating)
	}
	if userRating >= cutoff {
		return "⬇️❔"
	}
	return ""
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
