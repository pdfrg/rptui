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
	0: "Main Mix",
	1: "Mellow Mix",
	2: "Rock Mix",
	3: "Global Mix",
	5: "Beyond...",
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
	path            string
	Channel         int    `toml:"channel"`
	Bitrate         int    `toml:"bitrate"`
	ShowAlbumArt    bool   `toml:"show_album_art"`
	AlbumArtPath    string `toml:"album_art_path"`
	CopyAlbumArt    bool   `toml:"copy_album_art"`
	FavoritesDir    string `toml:"favorites_dir"`
	MaxFavorites    int    `toml:"max_favorites"`
	MinFavorites    int    `toml:"min_favorites"`
	ShowSkipWarning bool   `toml:"show_skip_warning"`
	ColorsFile      string `toml:"colors_file"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	favoritesDir := filepath.Join(xdg.CacheHome, "rptui", "favorites")
	return &Config{
		Channel:         0,
		Bitrate:         3,
		ShowAlbumArt:    true,
		AlbumArtPath:    "/tmp/cover.jpg",
		CopyAlbumArt:    false,
		FavoritesDir:    favoritesDir,
		MaxFavorites:    100,
		MinFavorites:    10,
		ShowSkipWarning: true,
		ColorsFile:      "",
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
		// Config file doesn't exist yet, use defaults
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
	// Ensure favorites directory parent exists
	favoritesDir := filepath.Dir(c.FavoritesDir)
	if err := os.MkdirAll(favoritesDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(c.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// applyDefaults ensures all fields have valid values
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()

	// Validate channel (0-5 are valid stations)
	if c.Channel < 0 || c.Channel > 5 {
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
