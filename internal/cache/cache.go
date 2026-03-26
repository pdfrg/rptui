// Package cache manages favorites and blocklist storage
package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/models"
)

// Logger for cache package
var logger *log.Logger

func init() {
	logger = loginit.InitLogger("[CACHE] ")
}

// CachedSong represents a song stored in cache
type CachedSong struct {
	EventID        int64  `json:"event_id"`
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	Album          string `json:"album"`
	Year           string `json:"year"`
	Duration       int64  `json:"duration"`
	GaplessURL     string `json:"gapless_url"`
	CoverLarge     string `json:"cover_large"`
	Rating         string `json:"rating"`
	ListenerRating string `json:"listener_rating"`
	AudioPath      string `json:"audio_path,omitempty"` // Local file path for downloaded audio
	AddedAt        int64  `json:"added_at"`             // Unix timestamp
}

// UnmarshalJSON implements custom JSON unmarshaling to handle string event_id
func (cs *CachedSong) UnmarshalJSON(data []byte) error {
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
	mu           sync.RWMutex
	favoritesDir string
	blocklistDir string
	maxFavorites int
	favorites    []CachedSong
	blocklist    []CachedSong
}

// NewCacheManager creates a new cache manager
func NewCacheManager(favoritesDir, blocklistDir string, maxFavorites int) *CacheManager {
	return &CacheManager{
		favoritesDir: favoritesDir,
		blocklistDir: blocklistDir,
		maxFavorites: maxFavorites,
	}
}

// EnsureDirectories creates the cache directories and loads metadata
func (c *CacheManager) EnsureDirectories() error {
	if err := os.MkdirAll(c.favoritesDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
	}
	if err := os.MkdirAll(c.blocklistDir, 0755); err != nil {
		return fmt.Errorf("failed to create blocklist directory: %w", err)
	}
	c.favorites = c.loadMetadata(c.favoritesDir)
	c.blocklist = c.loadMetadata(c.blocklistDir)
	return nil
}

// loadMetadata loads songs from metadata.json in the given directory
func (c *CacheManager) loadMetadata(dir string) []CachedSong {
	path := filepath.Join(dir, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Printf("Failed to read %s: %v", path, err)
		}
		return []CachedSong{}
	}
	var songs []CachedSong
	if err := json.Unmarshal(data, &songs); err != nil {
		logger.Printf("Failed to parse %s: %v", path, err)
		return []CachedSong{}
	}
	logger.Printf("Loaded %d entries from %s", len(songs), path)
	return songs
}

// saveMetadata writes songs to metadata.json in the given directory
func (c *CacheManager) saveMetadata(dir string, songs []CachedSong) error {
	path := filepath.Join(dir, "metadata.json")
	data, err := json.MarshalIndent(songs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// IsFavorite checks if a song is in favorites
func (c *CacheManager) IsFavorite(song *models.Song) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, fav := range c.favorites {
		if fav.EventID == song.EventID {
			return true
		}
	}
	return false
}

// IsBlocked checks if a song is in blocklist
func (c *CacheManager) IsBlocked(song *models.Song) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, b := range c.blocklist {
		if b.EventID == song.EventID {
			return true
		}
	}
	return false
}

// AddFavorite adds a song to favorites and triggers audio download
func (c *CacheManager) AddFavorite(song *models.Song, fileExt string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	for _, fav := range c.favorites {
		if fav.EventID == song.EventID {
			return "", nil
		}
	}

	// Enforce max limit
	if len(c.favorites) >= c.maxFavorites {
		// Remove oldest (last = oldest, since newest is prepended)
		oldest := c.favorites[len(c.favorites)-1]
		c.removeAudioFile(oldest.AudioPath)
		c.favorites = c.favorites[:len(c.favorites)-1]
	}

	audioPath := c.buildAudioPath(song, fileExt)

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
		AudioPath:      audioPath,
		AddedAt:        time.Now().Unix(),
	}

	// Prepend (newest first)
	c.favorites = append([]CachedSong{cached}, c.favorites...)

	if err := c.saveMetadata(c.favoritesDir, c.favorites); err != nil {
		// Rollback
		c.favorites = c.favorites[1:]
		return "", err
	}

	return audioPath, nil
}

// RemoveFavorite removes a song from favorites by event ID
func (c *CacheManager) RemoveFavorite(eventID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, fav := range c.favorites {
		if fav.EventID == eventID {
			c.removeAudioFile(fav.AudioPath)
			c.favorites = append(c.favorites[:i], c.favorites[i+1:]...)
			return c.saveMetadata(c.favoritesDir, c.favorites)
		}
	}
	return nil
}

// ToggleFavorite adds or removes a song from favorites
// Returns (added bool, audioPath, error)
func (c *CacheManager) ToggleFavorite(song *models.Song, fileExt string) (bool, string, error) {
	if c.IsFavorite(song) {
		if err := c.RemoveFavorite(song.EventID); err != nil {
			return false, "", err
		}
		return false, "", nil
	}

	audioPath, err := c.AddFavorite(song, fileExt)
	if err != nil {
		return false, "", err
	}
	return true, audioPath, nil
}

// AddBlocklist adds a song to blocklist
func (c *CacheManager) AddBlocklist(song *models.Song) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, b := range c.blocklist {
		if b.EventID == song.EventID {
			return nil
		}
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

	c.blocklist = append([]CachedSong{cached}, c.blocklist...)
	return c.saveMetadata(c.blocklistDir, c.blocklist)
}

// RemoveBlocklist removes a song from blocklist by event ID
func (c *CacheManager) RemoveBlocklist(eventID int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, b := range c.blocklist {
		if b.EventID == eventID {
			c.blocklist = append(c.blocklist[:i], c.blocklist[i+1:]...)
			return c.saveMetadata(c.blocklistDir, c.blocklist)
		}
	}
	return nil
}

// ToggleBlocklist adds or removes a song from blocklist
func (c *CacheManager) ToggleBlocklist(song *models.Song) (bool, error) {
	if c.IsBlocked(song) {
		if err := c.RemoveBlocklist(song.EventID); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := c.AddBlocklist(song); err != nil {
		return false, err
	}
	return true, nil
}

// GetFavorites returns all favorite songs (newest first)
func (c *CacheManager) GetFavorites() ([]CachedSong, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]CachedSong, len(c.favorites))
	copy(result, c.favorites)
	return result, nil
}

// GetBlocklist returns all blocklisted songs (newest first)
func (c *CacheManager) GetBlocklist() ([]CachedSong, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]CachedSong, len(c.blocklist))
	copy(result, c.blocklist)
	return result, nil
}

// GetFavoriteCount returns the number of favorites
func (c *CacheManager) GetFavoriteCount() (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.favorites), nil
}

// GetBlocklistCount returns the number of blocklisted songs
func (c *CacheManager) GetBlocklistCount() (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.blocklist), nil
}

// GetFavoriteByEventID returns a favorite song by event ID
func (c *CacheManager) GetFavoriteByEventID(eventID int64) (*CachedSong, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.favorites {
		if c.favorites[i].EventID == eventID {
			return &c.favorites[i], nil
		}
	}
	return nil, nil
}
func (c *CacheManager) UpdateFavoriteAudioPath(eventID int64, audioPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, fav := range c.favorites {
		if fav.EventID == eventID {
			c.favorites[i].AudioPath = audioPath
			if err := c.saveMetadata(c.favoritesDir, c.favorites); err != nil {
				logger.Printf("Failed to update metadata after download: %v", err)
			}
			return
		}
	}
}

// ToSong converts a CachedSong to a models.Song
func (cs *CachedSong) ToSong() *models.Song {
	url := cs.GaplessURL
	if cs.AudioPath != "" {
		url = cs.AudioPath
	}
	return &models.Song{
		Title:          cs.Title,
		Artist:         cs.Artist,
		Album:          cs.Album,
		Year:           cs.Year,
		Duration:       cs.Duration,
		EventID:        cs.EventID,
		GaplessURL:     url,
		CoverLarge:     cs.CoverLarge,
		Rating:         cs.Rating,
		ListenerRating: cs.ListenerRating,
	}
}

// DownloadFavorite downloads the audio file for a favorite in the background
func (c *CacheManager) DownloadFavorite(audioPath, url string, eventID int64) {
	if audioPath == "" || url == "" {
		return
	}
	// Skip if already downloaded
	if _, err := os.Stat(audioPath); err == nil {
		return
	}
	go c.downloadAudio(audioPath, url, eventID)
}

func (c *CacheManager) downloadAudio(path, url string, eventID int64) {
	logger.Printf("Downloading favorite audio: %s", filepath.Base(path))

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		logger.Printf("Failed to download audio for event %d: %v", eventID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Printf("Audio download returned status %d for event %d", resp.StatusCode, eventID)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logger.Printf("Failed to create directory for audio: %v", err)
		return
	}

	f, err := os.Create(path)
	if err != nil {
		logger.Printf("Failed to create audio file %s: %v", path, err)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		logger.Printf("Failed to write audio file %s: %v", path, err)
		os.Remove(path)
		return
	}

	logger.Printf("Downloaded audio: %s", filepath.Base(path))
}

// buildAudioPath generates a human-readable file path for a favorite's audio
func (c *CacheManager) buildAudioPath(song *models.Song, fileExt string) string {
	safeArtist := sanitizeFilename(song.Artist, 50)
	safeAlbum := sanitizeFilename(song.Album, 50)
	safeTitle := sanitizeFilename(song.Title, 50)
	filename := fmt.Sprintf("%s-%s-%s.%s", safeArtist, safeAlbum, safeTitle, fileExt)
	return filepath.Join(c.favoritesDir, filename)
}

// removeAudioFile deletes an audio file if the path is set
func (c *CacheManager) removeAudioFile(path string) {
	if path == "" {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		logger.Printf("Failed to remove audio file %s: %v", path, err)
	}
}

// sanitizeFilename removes unsafe characters and truncates to maxLen
func sanitizeFilename(s string, maxLen int) string {
	unsafe := `/\:*?"<>|`
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(unsafe, r) {
			continue
		}
		b.WriteRune(r)
	}
	result := strings.TrimSpace(b.String())
	if len(result) > maxLen {
		result = result[:maxLen]
	}
	return result
}
