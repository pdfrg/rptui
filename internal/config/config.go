// Package config handles user configuration persistence
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

// Station names mapping
var StationNames = map[int]string{
	0:   "The Main Mix",
	1:   "Mellow Mix",
	2:   "RockIt!",
	3:   "The Globe",
	5:   "Beyond...",
	42:  "Serenity",
	99:  "My Paradise",
	945: "KFAT",
}

// Bitrate names mapping
var BitrateNames = map[int]string{
	1: "64k AAC",
	2: "128k AAC",
	3: "320k AAC",
	4: "FLAC",
}

// Config manages user configuration persistence
type Config struct {
	path                  string
	Channel               int    `toml:"channel" comment:"default station for new sessions\nchanges made in app are saved to file and retained on restart\n0=Main, 1=Mellow, 2=Rock, 3=Globe, 42=Serenity, 5=Beyond, 945=KFAT (default: 0)"`
	Bitrate               int    `toml:"bitrate" comment:"1=64k AAC, 2=128k AAC, 3=320k AAC, 4=FLAC (default: 3)"`
	ShowAlbumArt          bool   `toml:"show_album_art" comment:"display album art for each song\nuses the best supported image protocol with auto fallback\nkitty > iterm2 > sixel > unicode (default: true)"`
	CopyAlbumArt          bool   `toml:"copy_album_art" comment:"save album art to file, useful for desktop/statusbar widgets (default: false)"`
	AlbumArtPath          string `toml:"album_art_path" comment:"file path for album art copy, needed if copy_album_art is true (default: /tmp/cover.jpg)"`
	FavoritesDir          string `toml:"favorites_dir" comment:"directory for favorites metadata and audio files (default: XDG_CACHE_HOME/rptui/favorites)"`
	MaxFavorites          int    `toml:"max_favorites" comment:"maximum favorites to save, to limit disk use\nset to 999999 for effectively unlimited (default: 100)"`
	MinFavorites          int    `toml:"min_favorites" comment:"minimum favorites to enable favorites mode\nautoplay favorites while awaiting RP API response for uninterrupted playback\nmust be <= max_favorites (default: 10)"`
	ShowSkipWarning       bool   `toml:"show_skip_warning" comment:"warn when skipping ahead of livestream without enough favorites\nset to false to disable (default: true)"`
	ColorsFile            string `toml:"colors_file" comment:"custom colors.toml file path, takes priority over theme setting\nfallback order: colors_file > theme > omarchy current theme > Catppuccin Mocha (default: '')"`
	Theme                 string `toml:"theme" comment:"built-in theme name\ncatppuccin-mocha, gruvbox-dark, dark-red, osaka-jade, synth, basic (default: '')"`
	TransparentBackground bool   `toml:"transparent_background" comment:"use terminal's default background color (default: false)"`
	DisableTheme          bool   `toml:"disable_theme" comment:"disable all theming, use terminal's default colors (default: false)"`

	// Terminal palette indices (used when disable_theme = true)
	TerminalPalette TerminalPaletteConfig `toml:"terminal_palette" comment:"palette indices for cursor/accent/muted when disable_theme is true"`

	// Discogs API authentication (optional, enables images + higher rate limits)
	// Auth priority: discogs_token (personal access) > discogs_key+discogs_secret (developer app) > env vars > unauthenticated
	DiscogsToken  string `toml:"discogs_token" comment:"Discogs personal access token\nenables artist images + higher API rate limits\nget one at: https://www.discogs.com/settings/developers\nalternative: set discogs_key + discogs_secret, or env vars DISCOGS_TOKEN / DISCOGS_KEY + DISCOGS_SECRET (default: '')"`
	DiscogsKey    string `toml:"discogs_key" comment:"Discogs consumer key (developer app auth)\nalternative to discogs_token, requires both key and secret (default: '')"`
	DiscogsSecret string `toml:"discogs_secret" comment:"Discogs consumer secret (developer app auth)\nalternative to discogs_token, requires both key and secret (default: '')"`

	// Scrobble services
	LastFM       LastFMConfig       `toml:"lastfm" comment:"Last.fm scrobbling\nrun 'rptui --lastfm-auth' once to obtain a session key"`
	ListenBrainz ListenBrainzConfig `toml:"listenbrainz" comment:"ListenBrainz scrobbling\ntoken found at: https://listenbrainz.org/profile/"`

	// Lidarr integration
	Lidarr LidarrConfig `toml:"lidarr" comment:"Lidarr music collection manager\nshows artist/album monitoring status, opens Lidarr web UI\napi_key from: Lidarr Settings > General"`

	// Visualizer settings
	Visualizer VisualizerConfig `toml:"visualizer" comment:"audio visualizer settings"`

	// Desktop notifications
	NotificationsEnabled bool `toml:"notifications_enabled" comment:"show desktop notifications on song changes (default: false)"`
	NotificationsShowArt bool `toml:"notifications_show_art" comment:"include album art thumbnail in notifications (default: true)"`

	// RP Auth (optional, enables ratings, comments, favorites sync, channel 99)
	RPAuth RPAuthConfig `toml:"rp_auth" comment:"Radio Paradise account (optional)\nenables user ratings, comments, favorites sync, and My Paradise channel\nusername: your RP account username\npassword: your RP account password (used to obtain session token)"`

	// RP Favorites auto-download
	AutoDownloadRPFavorites bool `toml:"auto_download_rp_favorites" comment:"when authenticated, automatically download songs to local favorites\nif your RP rating >= your My Paradise cutoff (chan_99_cutoff)\nuseful for keeping local favorites in sync with RP favorites (default: false)"`

	// RP auto-blocklist (blocks songs with low user ratings)
	AutoBlocklistRPEnabled   bool `toml:"auto_blocklist_rp_enabled" comment:"when authenticated, automatically blocklist songs rated at or below the threshold\nthreshold: rating value 1-4, songs rated <= threshold are blocked (default: false)"`
	AutoBlocklistRPThreshold int  `toml:"auto_blocklist_rp_threshold" comment:"rating threshold for auto-blocklist (1-4, default: 3)\nsongs with your RP rating <= this value are automatically blocked"`

	// DJ segment skipping (SMAD detection)
	SkipDJSegments      bool    `toml:"skip_dj_segments" comment:"enable automatic skipping of DJ speech at end of songs\nif enabled without enough favorites (min_favorites), may cause brief gaps in playback at block boundaries"`
	DJCheckSeconds      int     `toml:"dj_check_seconds" comment:"seconds from end of song to check for DJ speech (default: 80)"`
	DJConfidence        float64 `toml:"dj_confidence" comment:"minimum confidence for speech detection (0.0-1.0, default: 0.88)"`
	DJSafetyBuffer      float64 `toml:"dj_safety_buffer" comment:"extra seconds to add after detected speech for safe skipping (default: 0.5)"`
	DJMinSpeechDuration float64 `toml:"dj_min_speech_duration" comment:"minimum speech segment duration in seconds to count as DJ talk (default: 15.0)"`

	// Layout mode
	Layout string `toml:"layout" comment:"UI layout mode\nlarge: full layout with all elements (default)\nmedium: no bottom view (no playlist/lyrics/visualizer)\ncompact: no album art, no bottom view, mini footer\nnarrow: album art top-left, now playing below, mini footer (default: large)"`

	// Image protocol override (optional)
	ForceProtocol string `toml:"force_protocol" comment:"force a specific image protocol instead of auto-detecting\nuseful for testing or for terminals that don't support kitty\noptions: kitty, sixel, halfblocks, iterm2, or empty for auto-detect (default: '')"`

	// Jukebox mode
	Jukebox JukeboxConfig `toml:"jukebox" comment:"favorites jukebox mode\nplay your favorites in random order\nmin_faves: minimum favorites required (default: 20)\nrepeat: reshuffle and repeat after playing all (default: false)\ncrossfade_duration: seconds for volume crossfade between songs, 0=disabled (default: 3.0)"`
}

// JukeboxConfig holds jukebox mode settings
type JukeboxConfig struct {
	MinFaves          int     `toml:"min_faves" comment:"minimum favorites required to enable jukebox mode (default: 20)"`
	Repeat            bool    `toml:"repeat" comment:"reshuffle and repeat after playing all favorites (default: false)"`
	CrossfadeDuration float64 `toml:"crossfade_duration" comment:"seconds for pseudo-crossfade volume ramp between songs, 0=disabled (default: 3.0)"`
}

// LastFMConfig holds Last.fm scrobble settings
type LastFMConfig struct {
	Enabled    bool   `toml:"enabled" comment:"enable Last.fm scrobbling (default: false)"`
	SessionKey string `toml:"session_key" comment:"obtained via 'rptui --lastfm-auth' (default: '')"`
}

// ListenBrainzConfig holds ListenBrainz scrobble settings
type ListenBrainzConfig struct {
	Enabled bool   `toml:"enabled" comment:"enable ListenBrainz scrobbling (default: false)"`
	Token   string `toml:"token" comment:"user token from https://listenbrainz.org/profile/ (default: '')"`
}

// RPAuthConfig holds Radio Paradise account credentials (optional)
type RPAuthConfig struct {
	Username string `toml:"username" comment:"RP account username"`
	Password string `toml:"password" comment:"RP account password (used to obtain session token)"`
}

// LidarrConfig holds Lidarr integration settings
type LidarrConfig struct {
	Enabled bool   `toml:"enabled" comment:"enable Lidarr integration (default: false)"`
	URL     string `toml:"url" comment:"Lidarr base URL (e.g., http://localhost:8686)"`
	APIKey  string `toml:"api_key" comment:"Lidarr API key from Settings > General"`
}

// TerminalPaletteConfig holds palette indices for disable_theme mode
type TerminalPaletteConfig struct {
	Cursor int `toml:"cursor" comment:"palette index for cursor color (0-15, default: 2 = green)"`
	Accent int `toml:"accent" comment:"palette index for accent color (0-15, default: 4 = blue)"`
	Muted  int `toml:"muted" comment:"palette index for muted color (0-15, default: 8 = gray)"`
}

// VisualizerConfig holds visualizer settings
type VisualizerConfig struct {
	Mode         string `toml:"mode" comment:"default visualizer mode\nBars, Braille, ClassicPeak, Wave, Stars, BrailleBars, Rain, Segmented, Binary (default: Bars)"`
	ShowInfo     string `toml:"show_info" comment:"song info overlay in fullscreen visualizer\nfade, on, off (default: fade)"`
	InfoDuration int    `toml:"info_duration" comment:"seconds to show song info overlay (default: 5)"`
	RealAudio    bool   `toml:"real_audio" comment:"use real audio capture\nLinux: PipeWire (pw-record) or PulseAudio (parecord)\nWindows: WASAPI loopback\nmacOS: not supported (default: true)"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	favoritesDir := filepath.Join(xdg.CacheHome, "rptui", "favorites")
	return &Config{
		Channel:         0,
		Bitrate:         3,
		ShowAlbumArt:    true,
		AlbumArtPath:    filepath.Join(os.TempDir(), "cover.jpg"),
		CopyAlbumArt:    false,
		FavoritesDir:    favoritesDir,
		MaxFavorites:    100,
		MinFavorites:    10,
		ShowSkipWarning: true,
		ColorsFile:      "",
		Visualizer: VisualizerConfig{
			Mode:         "Segmented",
			ShowInfo:     "fade",
			InfoDuration: 5,
			RealAudio:    true,
		},
		NotificationsEnabled:     false,
		NotificationsShowArt:     true,
		AutoBlocklistRPEnabled:   false,
		AutoBlocklistRPThreshold: 3,
		SkipDJSegments:           false,
		DJCheckSeconds:           80,
		DJConfidence:             0.88,
		DJSafetyBuffer:           0.5,
		DJMinSpeechDuration:      15.0,
		Jukebox: JukeboxConfig{
			MinFaves:          20,
			Repeat:            false,
			CrossfadeDuration: 3.0,
		},
		Layout:                "large",
		ForceProtocol:         "",
		TransparentBackground: false,
		DisableTheme:          false,
		TerminalPalette: TerminalPaletteConfig{
			Cursor: 2, // green
			Accent: 4, // blue
			Muted:  8, // gray
		},
	}
}

// NewConfig creates a new Config and loads existing values
func NewConfig() (*Config, error) {
	cfg := DefaultConfig()

	// Use XDG config directory
	configDir := filepath.Join(xdg.ConfigHome, "rptui")
	cfg.path = filepath.Join(configDir, "config.toml")

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Load existing config if present
	if err := cfg.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// Config file doesn't exist yet, create it with defaults
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("failed to save default config: %w", err)
		}
	}

	return cfg, nil
}

// Load loads config from file
func (c *Config) Load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	// Decode into a temporary struct to validate
	var temp map[string]any
	if err := toml.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("failed to parse config TOML: %w", err)
	}

	// Unmarshal into config
	if err := toml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	// Validate and apply defaults for any missing fields
	c.applyDefaults()

	return nil
}

// Save saves config to file
func (c *Config) Save() error {
	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Ensure favorites directory parent exists
	favoritesDir := filepath.Dir(c.FavoritesDir)
	if err := os.MkdirAll(favoritesDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	header := []byte("# rptui configuration file\n\n")
	output := append(header, data...)

	if err := os.WriteFile(c.path, output, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// applyDefaults ensures all fields have valid values
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()

	// Validate channel (must be a known station)
	if _, ok := StationNames[c.Channel]; !ok {
		c.Channel = defaults.Channel
	}

	// Validate bitrate (1-4 are valid bitrates; MP3 removed)
	if c.Bitrate < 1 || c.Bitrate > 4 {
		c.Bitrate = defaults.Bitrate
	}

	// Validate favorites settings
	if c.MaxFavorites < 1 {
		c.MaxFavorites = defaults.MaxFavorites
	}
	if c.MinFavorites < 0 {
		c.MinFavorites = defaults.MinFavorites
	}
	if c.MinFavorites > c.MaxFavorites {
		c.MinFavorites = c.MaxFavorites / 2
	}

	// Ensure favorites dir is set
	if c.FavoritesDir == "" {
		c.FavoritesDir = defaults.FavoritesDir
	}

	// Ensure album art path is set
	if c.AlbumArtPath == "" {
		c.AlbumArtPath = defaults.AlbumArtPath
	}

	// Ensure visualizer settings are valid
	if c.Visualizer.Mode == "" {
		c.Visualizer.Mode = defaults.Visualizer.Mode
	}
	if c.Visualizer.ShowInfo == "" {
		c.Visualizer.ShowInfo = defaults.Visualizer.ShowInfo
	} else if c.Visualizer.ShowInfo != "fade" && c.Visualizer.ShowInfo != "on" && c.Visualizer.ShowInfo != "off" {
		c.Visualizer.ShowInfo = defaults.Visualizer.ShowInfo
	}
	if c.Visualizer.InfoDuration <= 0 {
		c.Visualizer.InfoDuration = defaults.Visualizer.InfoDuration
	}

	// Validate jukebox settings
	if c.Jukebox.MinFaves < 1 {
		c.Jukebox.MinFaves = defaults.Jukebox.MinFaves
	}
	if c.Jukebox.CrossfadeDuration < 0 {
		c.Jukebox.CrossfadeDuration = defaults.Jukebox.CrossfadeDuration
	}

	// Validate auto-blocklist threshold (must be 1-4)
	if c.AutoBlocklistRPThreshold < 1 || c.AutoBlocklistRPThreshold > 4 {
		c.AutoBlocklistRPThreshold = defaults.AutoBlocklistRPThreshold
	}

	// Validate layout
	validLayouts := map[string]bool{"large": true, "medium": true, "compact": true, "narrow": true}
	if c.Layout == "" || !validLayouts[c.Layout] {
		c.Layout = defaults.Layout
	}

	// Validate force_protocol
	if c.ForceProtocol != "" {
		validProtocols := map[string]bool{"kitty": true, "sixel": true, "halfblocks": true, "iterm2": true}
		if !validProtocols[c.ForceProtocol] {
			c.ForceProtocol = ""
		}
	}

	// TransparentBackground and DisableTheme don't need validation (bool is always valid)

	// Validate DJ segment skipping settings
	if c.DJCheckSeconds < 5 || c.DJCheckSeconds > 120 {
		c.DJCheckSeconds = defaults.DJCheckSeconds
	}
	if c.DJConfidence < 0.1 || c.DJConfidence > 0.99 {
		c.DJConfidence = defaults.DJConfidence
	}
	if c.DJSafetyBuffer < 0 || c.DJSafetyBuffer > 5 {
		c.DJSafetyBuffer = defaults.DJSafetyBuffer
	}
	if c.DJMinSpeechDuration < 5 || c.DJMinSpeechDuration > 60 {
		c.DJMinSpeechDuration = defaults.DJMinSpeechDuration
	}
}

// GetDisplayInfo returns display string for current station/bitrate
func (c *Config) GetDisplayInfo() string {
	stationName := StationNames[c.Channel]
	if stationName == "" {
		stationName = fmt.Sprintf("Station %d", c.Channel)
	}

	bitrateName := BitrateNames[c.Bitrate]
	if bitrateName == "" {
		bitrateName = fmt.Sprintf("Bitrate %d", c.Bitrate)
	}

	return fmt.Sprintf("%s • %s", stationName, bitrateName)
}

// GetFavoritesDir returns the favorites directory path
func (c *Config) GetFavoritesDir() string {
	return c.FavoritesDir
}

// GetBlocklistDir returns the blocklist directory path
func (c *Config) GetBlocklistDir() string {
	return filepath.Join(filepath.Dir(c.FavoritesDir), "blocklist")
}

// GetScrobbleCacheDir returns the scrobble cache directory path.
func GetScrobbleCacheDir() string {
	return filepath.Join(xdg.CacheHome, "rptui", "scrobbles")
}

// StationIssue represents a discrepancy between local config and RP API
type StationIssue struct {
	Kind    string // "new", "missing", "renamed"
	Message string
}

// CheckStationIssues compares local StationNames against RP channel list.
// It returns issues for new, missing, or renamed stations.
func CheckStationIssues(rpChannels map[int]string) []StationIssue {
	var issues []StationIssue

	// Check for missing or renamed stations
	for localID, localName := range StationNames {
		// Skip channel 99 (My Paradise) - it's authenticated-only, not in downloadable channels list
		if localID == 99 {
			continue
		}
		if rpName, ok := rpChannels[localID]; !ok {
			// Station ID no longer exists on RP — could be missing or renamed
			issues = append(issues, StationIssue{
				Kind:    "missing",
				Message: fmt.Sprintf("Station %d (%s) is no longer available on RP", localID, localName),
			})
		} else if rpName != localName {
			// Same ID but different name
			issues = append(issues, StationIssue{
				Kind:    "renamed",
				Message: fmt.Sprintf("Station %d renamed from \"%s\" to \"%s\"", localID, localName, rpName),
			})
		}
	}

	// Check for new stations not in our list
	for rpID, rpName := range rpChannels {
		if _, ok := StationNames[rpID]; !ok {
			issues = append(issues, StationIssue{
				Kind:    "new",
				Message: fmt.Sprintf("New station available: %s (%d)", rpName, rpID),
			})
		}
	}

	return issues
}
