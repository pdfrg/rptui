// Package cache manages favorites and blocklist storage
package cache

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rptui-bubbletea/internal/models"
)

// Logger for cache package
var logger *log.Logger

func init() {
	// Setup logging to file
	f, err := os.OpenFile("rptui-go.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logger = log.New(f, "[CACHE] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "[CACHE] ", log.LstdFlags|log.Lshortfile)
	}
}

// CachedSong represents a song stored in cache
type CachedSong struct {
	EventID    int64  `json:"event_id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	Year       string `json:"year"`
	Duration   int64  `json:"duration"`
	GaplessURL string `json:"gapless_url"`
	CoverLarge     string `json:"cover_large"`
	Rating         string `json:"rating"`
	ListenerRating string `json:"listener_rating"`
	AddedAt        int64  `json:"added_at"` // Unix timestamp
}

// UnmarshalJSON implements custom JSON unmarshaling to handle string event_id
func (cs *CachedSong) UnmarshalJSON(data []byte) error {
	// Create alias to avoid infinite recursion
	type Alias CachedSong
	aux := &struct {
		EventID any `json:"event_id"`
		*Alias
	}{
		Alias: (*Alias)(cs),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle event_id as string or number
	switch v := aux.EventID.(type) {
	case string:
		fmt.Sscanf(v, "%d", &cs.EventID)
	case float64:
		cs.EventID = int64(v)
	case int64:
		cs.EventID = v
	}

	return nil
}

// CacheManager manages both favorites and blocklist
type CacheManager struct {
	favoritesDir string
	blocklistDir string
	maxFavorites int
}

// NewCacheManager creates a new cache manager
func NewCacheManager(favoritesDir, blocklistDir string, maxFavorites int) *CacheManager {
	return &CacheManager{
		favoritesDir: favoritesDir,
		blocklistDir: blocklistDir,
		maxFavorites: maxFavorites,
	}
}

// EnsureDirectories creates the cache directories if they don't exist
func (c *CacheManager) EnsureDirectories() error {
	if err := os.MkdirAll(c.favoritesDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
	}
	if err := os.MkdirAll(c.blocklistDir, 0755); err != nil {
		return fmt.Errorf("failed to create blocklist directory: %w", err)
	}
	return nil
}

// IsFavorite checks if a song is in favorites
func (c *CacheManager) IsFavorite(song *models.Song) bool {
	// Check individual file first (Go format)
	path := c.getFavoritePath(song.EventID)
	if _, err := os.Stat(path); err == nil {
		return true
	}

	// Check metadata.json (Python format)
	metadataPath := filepath.Join(c.favoritesDir, "metadata.json")

	if _, err := os.Stat(metadataPath); err == nil {
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			return false
		}
		var favorites []CachedSong
		if err := json.Unmarshal(data, &favorites); err != nil {
			return false
		}
		for _, fav := range favorites {
			if fav.EventID == song.EventID {
				return true
			}
		}
	}

	return false
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsBlocked checks if a song is in blocklist
func (c *CacheManager) IsBlocked(song *models.Song) bool {
	path := c.getBlocklistPath(song.EventID)
	_, err := os.Stat(path)
	return err == nil
}

// AddFavorite adds a song to favorites
func (c *CacheManager) AddFavorite(song *models.Song) error {
	// Check if already exists
	if c.IsFavorite(song) {
		return nil // Already exists
	}

	// Check max limit
	favorites, err := c.GetFavorites()
	if err != nil {
		return err
	}

	if len(favorites) >= c.maxFavorites {
		// Remove oldest favorite
		if len(favorites) > 0 {
			oldest := favorites[len(favorites)-1]
			if err := c.RemoveFavoriteByID(oldest.EventID); err != nil {
				return err
			}
		}
	}

	// Save favorite
	cached := CachedSong{
		EventID:        song.EventID,
		Title:          song.Title,
		Artist:         song.Artist,
		Album:          song.Album,
		Year:           song.Year,
		Duration:       song.Duration,
		GaplessURL:     song.GaplessURL,
		CoverLarge:     song.CoverLarge,
		Rating:         song.Rating,
		ListenerRating: song.ListenerRating,
		AddedAt:        time.Now().Unix(),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal favorite: %w", err)
	}

	path := c.getFavoritePath(song.EventID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write favorite: %w", err)
	}

	return nil
}

// RemoveFavorite removes a song from favorites
func (c *CacheManager) RemoveFavorite(song *models.Song) error {
	return c.RemoveFavoriteByID(song.EventID)
}

// RemoveFavoriteByID removes a song from favorites by event ID
func (c *CacheManager) RemoveFavoriteByID(eventID int64) error {
	path := c.getFavoritePath(eventID)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("failed to remove favorite: %w", err)
	}
	return nil
}

// ToggleFavorite adds or removes a song from favorites
func (c *CacheManager) ToggleFavorite(song *models.Song) (bool, error) {
	if c.IsFavorite(song) {
		if err := c.RemoveFavorite(song); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := c.AddFavorite(song); err != nil {
		return false, err
	}
	return true, nil
}

// AddBlocklist adds a song to blocklist
func (c *CacheManager) AddBlocklist(song *models.Song) error {
	if c.IsBlocked(song) {
		return nil // Already exists
	}

	cached := CachedSong{
		EventID:        song.EventID,
		Title:          song.Title,
		Artist:         song.Artist,
		Album:          song.Album,
		Year:           song.Year,
		Duration:       song.Duration,
		GaplessURL:     song.GaplessURL,
		CoverLarge:     song.CoverLarge,
		Rating:         song.Rating,
		ListenerRating: song.ListenerRating,
		AddedAt:        time.Now().Unix(),
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal blocklist entry: %w", err)
	}

	path := c.getBlocklistPath(song.EventID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write blocklist entry: %w", err)
	}

	return nil
}

// RemoveBlocklist removes a song from blocklist
func (c *CacheManager) RemoveBlocklist(song *models.Song) error {
	return c.RemoveBlocklistByID(song.EventID)
}

// RemoveBlocklistByID removes a song from blocklist by event ID
func (c *CacheManager) RemoveBlocklistByID(eventID int64) error {
	path := c.getBlocklistPath(eventID)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already removed
		}
		return fmt.Errorf("failed to remove blocklist entry: %w", err)
	}
	return nil
}

// ToggleBlocklist adds or removes a song from blocklist
func (c *CacheManager) ToggleBlocklist(song *models.Song) (bool, error) {
	if c.IsBlocked(song) {
		if err := c.RemoveBlocklist(song); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := c.AddBlocklist(song); err != nil {
		return false, err
	}
	return true, nil
}

// GetFavorites returns all favorite songs sorted by added date (newest first)
func (c *CacheManager) GetFavorites() ([]CachedSong, error) {
	return c.loadCachedSongs(c.favoritesDir)
}

// GetBlocklist returns all blocklisted songs sorted by added date (newest first)
func (c *CacheManager) GetBlocklist() ([]CachedSong, error) {
	return c.loadCachedSongs(c.blocklistDir)
}

// loadCachedSongs loads all cached songs from a directory
func (c *CacheManager) loadCachedSongs(dir string) ([]CachedSong, error) {
	var songs []CachedSong

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return songs, nil
		}
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // Skip unreadable files
		}

		var song CachedSong
		if err := json.Unmarshal(data, &song); err != nil {
			continue // Skip invalid files
		}

		songs = append(songs, song)
	}

	// Sort by added date (newest first)
	sort.Slice(songs, func(i, j int) bool {
		return songs[i].AddedAt > songs[j].AddedAt
	})

	return songs, nil
}

// GetFavoriteCount returns the number of favorites
func (c *CacheManager) GetFavoriteCount() (int, error) {
	// Check for metadata.json first (Python format)
	metadataPath := filepath.Join(c.favoritesDir, "metadata.json")

	if _, err := os.Stat(metadataPath); err == nil {
		data, err := os.ReadFile(metadataPath)
		if err != nil {
			return 0, err
		}
		var favorites []CachedSong
		if err := json.Unmarshal(data, &favorites); err != nil {
			return 0, err
		}
		return len(favorites), nil
	}

	// Count individual JSON files (Go format)
	favorites, err := c.GetFavorites()
	if err != nil {
		return 0, err
	}
	return len(favorites), nil
}

// GetBlocklistCount returns the number of blocklisted songs
func (c *CacheManager) GetBlocklistCount() (int, error) {
	blocklist, err := c.GetBlocklist()
	if err != nil {
		return 0, err
	}
	return len(blocklist), nil
}

// GetFavoriteByEventID returns a favorite song by event ID
func (c *CacheManager) GetFavoriteByEventID(eventID int64) (*CachedSong, error) {
	path := c.getFavoritePath(eventID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var song CachedSong
	if err := json.Unmarshal(data, &song); err != nil {
		return nil, err
	}

	return &song, nil
}

// getFavoritePath returns the path for a favorite file
func (c *CacheManager) getFavoritePath(eventID int64) string {
	return filepath.Join(c.favoritesDir, fmt.Sprintf("%d.json", eventID))
}

// getBlocklistPath returns the path for a blocklist file
func (c *CacheManager) getBlocklistPath(eventID int64) string {
	return filepath.Join(c.blocklistDir, fmt.Sprintf("%d.json", eventID))
}

// CleanupOldFavorites removes oldest favorites if count exceeds max
func (c *CacheManager) CleanupOldFavorites() error {
	favorites, err := c.GetFavorites()
	if err != nil {
		return err
	}

	for len(favorites) > c.maxFavorites {
		oldest := favorites[len(favorites)-1]
		if err := c.RemoveFavoriteByID(oldest.EventID); err != nil {
			return err
		}
		favorites = favorites[:len(favorites)-1]
	}

	return nil
}

// ToSong converts a CachedSong to a models.Song
func (cs *CachedSong) ToSong() *models.Song {
	return &models.Song{
		Title:          cs.Title,
		Artist:         cs.Artist,
		Album:          cs.Album,
		Year:           cs.Year,
		Duration:       cs.Duration,
		EventID:        cs.EventID,
		GaplessURL:     cs.GaplessURL,
		CoverLarge:     cs.CoverLarge,
		Rating:         cs.Rating,
		ListenerRating: cs.ListenerRating,
	}
}
