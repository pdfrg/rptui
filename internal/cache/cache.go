// Package cache manages favorites, blocklist, and offline cache storage
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
	EventID         int64  `json:"event_id"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	Year            string `json:"year"`
	Duration        int64  `json:"duration"`
	GaplessURL      string `json:"gapless_url"`
	CoverLarge      string `json:"cover_large"`
	Rating          string `json:"rating"`
	ListenerRating  string `json:"listener_rating"`
	AudioPath       string `json:"audio_path,omitempty"`
	SchedTimeMillis int64  `json:"sched_time_millis,omitempty"`
	AddedAt         int64  `json:"added_at"`
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

// CacheManager manages favorites, blocklist, and offline caches
type CacheManager struct {
	mu           sync.RWMutex
	favoritesDir string
	blocklistDir string
	offlineDir   string
	maxFavorites int
	favorites    []CachedSong
	blocklist    []CachedSong
	downloadWG   sync.WaitGroup
	downloading  sync.Map
}

// NewCacheManager creates a new cache manager
func NewCacheManager(favoritesDir, blocklistDir string, maxFavorites int) *CacheManager {
	return &CacheManager{
		favoritesDir: favoritesDir,
		blocklistDir: blocklistDir,
		maxFavorites: maxFavorites,
	}
}

// SetOfflineDir sets the offline cache directory
func (c *CacheManager) SetOfflineDir(dir string) {
	c.offlineDir = dir
}

// GetOfflineDir returns the offline cache directory
func (c *CacheManager) GetOfflineDir() string {
	return c.offlineDir
}

// EnsureDirectories creates the cache directories and loads metadata
func (c *CacheManager) EnsureDirectories() error {
	if err := os.MkdirAll(c.favoritesDir, 0755); err != nil {
		return fmt.Errorf("failed to create favorites directory: %w", err)
	}
	if err := os.MkdirAll(c.blocklistDir, 0755); err != nil {
		return fmt.Errorf("failed to create blocklist directory: %w", err)
	}
	if c.offlineDir != "" {
		if err := os.MkdirAll(c.offlineDir, 0755); err != nil {
			return fmt.Errorf("failed to create offline directory: %w", err)
		}
	}

	// Clean up orphaned .tmp files from crashed/interrupted downloads
	if tmpFiles, err := filepath.Glob(filepath.Join(c.favoritesDir, "*.tmp")); err == nil {
		for _, f := range tmpFiles {
			os.Remove(f)
		}
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

// ToggleFavorite checks if a song is already a favorite.
func (c *CacheManager) ToggleFavorite(song *models.Song, fileExt string) (bool, string, error) {
	if c.IsFavorite(song) {
		if err := c.RemoveFavorite(song.EventID); err != nil {
			return false, "", err
		}
		return false, "", nil
	}
	return true, "", nil
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

// GetFavoritesDiskSpace calculates total disk space used by favorites
func (c *CacheManager) GetFavoritesDiskSpace() string {
	var total int64
	entries, err := os.ReadDir(c.favoritesDir)
	if err != nil {
		return "?"
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			total += info.Size()
		}
	}
	return formatBytes(total)
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

// StartFavoriteDownload downloads the audio for a favorite and adds it to metadata.
func (c *CacheManager) StartFavoriteDownload(song *models.Song, fileExt string, onDone func(success bool)) {
	if song == nil {
		return
	}
	if _, loaded := c.downloading.LoadOrStore(song.EventID, true); loaded {
		return
	}
	c.downloadWG.Add(1)
	go func() {
		defer c.downloadWG.Done()
		defer c.downloading.Delete(song.EventID)
		success := c.downloadAndAdd(song, fileExt)
		onDone(success)
	}()
}

// downloadAndAdd downloads audio to a .tmp file, renames on success, then adds to favorites.
func (c *CacheManager) downloadAndAdd(song *models.Song, fileExt string) bool {
	audioPath := c.buildAudioPath(song, fileExt)
	tmpPath := audioPath + ".tmp"

	if _, err := os.Stat(audioPath); err == nil {
		return false
	}

	logger.Printf("Downloading favorite audio: %s", filepath.Base(audioPath))

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(song.GaplessURL)
	if err != nil {
		logger.Printf("Failed to download audio for event %d: %v", song.EventID, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Printf("Audio download returned status %d for event %d", resp.StatusCode, song.EventID)
		return false
	}

	if err := os.MkdirAll(c.favoritesDir, 0755); err != nil {
		logger.Printf("Failed to create favorites directory: %v", err)
		return false
	}

	f, err := os.Create(tmpPath)
	if err != nil {
		logger.Printf("Failed to create temp audio file: %v", err)
		return false
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		logger.Printf("Failed to write audio file: %v", err)
		return false
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		logger.Printf("Failed to close audio file: %v", err)
		return false
	}

	if err := os.Rename(tmpPath, audioPath); err != nil {
		os.Remove(tmpPath)
		logger.Printf("Failed to rename temp audio file: %v", err)
		return false
	}

	_, err = c.AddFavorite(song, fileExt)
	if err != nil {
		logger.Printf("Failed to add favorite after download: %v", err)
		return false
	}

	logger.Printf("Downloaded and added favorite: %s", filepath.Base(audioPath))
	return true
}

// WaitForDownloads waits for all in-progress downloads to complete, up to a timeout.
func (c *CacheManager) WaitForDownloads(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		c.downloadWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		logger.Printf("All downloads completed")
	case <-time.After(timeout):
		logger.Printf("Download wait timed out after %v", timeout)
	}
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

// formatBytes formats bytes into human-readable string
func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	} else if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
}
