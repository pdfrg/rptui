package cache

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pdfrg/rptui/internal/config"
)

// CacheEntry represents a completed offline cache
type CacheEntry struct {
	Name          string    `json:"name"`
	Station       int       `json:"station"`
	Bitrate       int       `json:"bitrate"`
	TargetSeconds int       `json:"target_seconds"`
	ActualSeconds int       `json:"actual_seconds"`
	SongCount     int       `json:"song_count"`
	SizeBytes     int64     `json:"size_bytes"`
	CreatedAt     time.Time `json:"created_at"`
}

// CacheIndex holds all cache entries
type CacheIndex struct {
	Caches []CacheEntry `json:"caches"`
}

// CacheRecorder handles recording a cache session
type CacheRecorder struct {
	cacheDir      string
	name          string
	station       int
	bitrate       int
	targetSeconds int
	songs         []CachedSong
	totalSeconds  int64
	totalBytes    int64
	songIndex     int
	client        *http.Client
	cancelCh      chan struct{}
}

// NewCacheRecorder creates a new cache recorder
func NewCacheRecorder(offlineDir, name string, station, bitrate, targetSeconds int) *CacheRecorder {
	return &CacheRecorder{
		cacheDir:      filepath.Join(offlineDir, name),
		name:          name,
		station:       station,
		bitrate:       bitrate,
		targetSeconds: targetSeconds,
		client:        &http.Client{Timeout: 5 * time.Minute},
		cancelCh:      make(chan struct{}),
	}
}

// Setup creates the cache directory structure and config
func (r *CacheRecorder) Setup() error {
	if err := os.MkdirAll(filepath.Join(r.cacheDir, "songs"), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	cfg := CacheConfig{
		Station:       r.station,
		Bitrate:       r.bitrate,
		TargetSeconds: r.targetSeconds,
		CreatedAt:     time.Now(),
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(r.cacheDir, "config.json"), data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// CacheConfig stores cache recording configuration
type CacheConfig struct {
	Station       int       `json:"station"`
	Bitrate       int       `json:"bitrate"`
	TargetSeconds int       `json:"target_seconds"`
	CreatedAt     time.Time `json:"created_at"`
}

// Cleanup removes the entire cache directory (used on interrupt)
func (r *CacheRecorder) Cleanup() error {
	return os.RemoveAll(r.cacheDir)
}

// ProgressInfo holds recording progress information
type ProgressInfo struct {
	SongIndex     int
	TotalSeconds  int64
	TargetSeconds int
	TotalBytes    int64
	CurrentSong   string
	Duration      int64
	Size          int64
}

// BlockInfo holds metadata about a fetched block
type BlockInfo struct {
	BlockID string
	Songs   []CachedSong
}

// BlockFetcher is a function that fetches a block of songs from RP API
type BlockFetcher func(station, bitrate int) (BlockInfo, error)

// Record downloads songs until target duration is reached.
// It fetches a block, downloads all songs, then sleeps until ~120s before
// the last song's scheduled end time. Then it polls every 5s for a new block.
// Returns error or nil on success.
func (r *CacheRecorder) Record(blockFetcher BlockFetcher, progressFn func(ProgressInfo)) error {
	// Register interrupt handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		<-sigCh
		fmt.Println("\n⚠ Interrupting will result in loss of all cache data!")
		fmt.Print("Continue interrupt? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) == "y" {
			r.cancelCh <- struct{}{}
		} else {
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		}
	}()

	lastBlockID := ""
	isFirstBlock := true

	for {
		// Check for cancellation
		select {
		case <-r.cancelCh:
			return r.Cleanup()
		default:
		}

		// Fetch block from RP API
		if isFirstBlock {
			fmt.Println("Querying RP API...")
		}
		blockInfo, err := blockFetcher(r.station, r.bitrate)
		if err != nil {
			return fmt.Errorf("failed to fetch block: %w", err)
		}
		isFirstBlock = false

		// Skip if same block (already downloaded) or promo block (block_id = 0)
		if blockInfo.BlockID == lastBlockID || blockInfo.BlockID == "0" {
			// Sleep 5s then retry (polling mode)
			select {
			case <-r.cancelCh:
				return r.Cleanup()
			case <-time.After(5 * time.Second):
			}
			continue
		}

		// New block detected
		if lastBlockID != "" {
			fmt.Printf("\nNew block detected: %s\n", blockInfo.BlockID)
		}

		lastBlockID = blockInfo.BlockID
		songs := blockInfo.Songs

		if len(songs) == 0 {
			select {
			case <-r.cancelCh:
				return r.Cleanup()
			case <-time.After(5 * time.Second):
			}
			continue
		}

		// Calculate when to start polling for next block
		// Use last song's sched_time_millis + duration, minus 120s
		lastSong := songs[len(songs)-1]
		blockEndMs := lastSong.SchedTimeMillis + lastSong.Duration
		pollStartUnix := blockEndMs/1000 - 120
		pollStartTime := time.Unix(pollStartUnix, 0)
		waitDuration := time.Until(pollStartTime)

		fmt.Printf("\nDownloading %d songs from block %s...\n", len(songs), lastBlockID)

		// Download all songs in this block
		for _, song := range songs {
			select {
			case <-r.cancelCh:
				return r.Cleanup()
			default:
			}

			// Download audio
			fileExt := getFileExtension(r.bitrate)
			filename := fmt.Sprintf("%03d.%s", r.songIndex+1, fileExt)
			audioPath := filepath.Join(r.cacheDir, "songs", filename)

			fmt.Printf("  Downloading: %s - %s (%s)...\n",
				song.Artist, song.Title, FormatDuration(song.Duration/1000))

			size, err := r.downloadSong(song.GaplessURL, audioPath)
			if err != nil {
				logger.Printf("Failed to download song %d: %v", r.songIndex+1, err)
				fmt.Printf("  ✗ Failed: %v\n", err)
				continue
			}

			song.AudioPath = audioPath

			// Download album art if available
			if song.CoverLarge != "" {
				artFilename := fmt.Sprintf("%03d.jpg", r.songIndex+1)
				artPath := filepath.Join(r.cacheDir, "covers", artFilename)
				if err := os.MkdirAll(filepath.Join(r.cacheDir, "covers"), 0755); err == nil {
					if _, err := r.downloadSong(song.CoverLarge, artPath); err == nil {
						song.CoverLarge = artPath
					} else {
						logger.Printf("Failed to download cover for song %d: %v", r.songIndex+1, err)
					}
				}
			}

			r.songs = append(r.songs, song)
			r.totalSeconds += song.Duration / 1000
			r.totalBytes += size
			r.songIndex++

			// Save metadata after each song (resilience)
			if err := r.saveMetadata(); err != nil {
				logger.Printf("Failed to save metadata: %v", err)
			}

			// Report progress
			if progressFn != nil {
				progressFn(ProgressInfo{
					SongIndex:     r.songIndex,
					TotalSeconds:  r.totalSeconds,
					TargetSeconds: r.targetSeconds,
					TotalBytes:    r.totalBytes,
					CurrentSong:   fmt.Sprintf("%s - %s", song.Artist, song.Title),
					Duration:      song.Duration / 1000,
					Size:          size,
				})
			}

			fmt.Printf("  ✓ Downloaded: %s (%s)\n", FormatBytes(size), FormatDuration(song.Duration/1000))

			// Check if we've exceeded target (but finish current song)
			if r.totalSeconds >= int64(r.targetSeconds) {
				return r.complete()
			}
		}

		// Check target after block download
		if r.totalSeconds >= int64(r.targetSeconds) {
			return r.complete()
		}

		// Sleep until polling time (or 5s if we're already past it)
		if waitDuration > 0 {
			expectedTime := pollStartTime.Format("15:04:05")
			fmt.Printf("\nAll songs from block %s downloaded. Sleeping until %s for next block...\n",
				lastBlockID, expectedTime)
			select {
			case <-r.cancelCh:
				return r.Cleanup()
			case <-time.After(waitDuration):
			}
		}

		// Now poll every 5s for new block
		logger.Printf("Starting polling for next block (last block: %s)", lastBlockID)
		for {
			select {
			case <-r.cancelCh:
				return r.Cleanup()
			default:
			}

			blockInfo, err := blockFetcher(r.station, r.bitrate)
			if err != nil {
				logger.Printf("Poll error: %v, retrying in 5s", err)
				select {
				case <-r.cancelCh:
					return r.Cleanup()
				case <-time.After(5 * time.Second):
				}
				continue
			}

			if blockInfo.BlockID != lastBlockID && blockInfo.BlockID != "0" {
				logger.Printf("New block detected: %s (was %s)", blockInfo.BlockID, lastBlockID)
				break // Go back to outer loop to download new block
			}

			select {
			case <-r.cancelCh:
				return r.Cleanup()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// downloadSong downloads audio from URL to file, returns file size
func (r *CacheRecorder) downloadSong(url, path string) (int64, error) {
	resp, err := r.client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return 0, err
	}

	size, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return 0, err
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return 0, err
	}

	return size, nil
}

// saveMetadata writes metadata.json
func (r *CacheRecorder) saveMetadata() error {
	path := filepath.Join(r.cacheDir, "metadata.json")
	data, err := json.MarshalIndent(r.songs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// complete finalizes the cache and updates the index
func (r *CacheRecorder) complete() error {
	// Calculate total size from actual files
	var totalSize int64
	entries, err := os.ReadDir(filepath.Join(r.cacheDir, "songs"))
	if err == nil {
		for _, e := range entries {
			if e.Type().IsRegular() {
				info, err := e.Info()
				if err == nil {
					totalSize += info.Size()
				}
			}
		}
	} else {
		totalSize = r.totalBytes
	}

	// Update cache index
	index, err := LoadCacheIndex(filepath.Dir(r.cacheDir))
	if err != nil {
		index = &CacheIndex{}
	}

	entry := CacheEntry{
		Name:          r.name,
		Station:       r.station,
		Bitrate:       r.bitrate,
		TargetSeconds: r.targetSeconds,
		ActualSeconds: int(r.totalSeconds),
		SongCount:     len(r.songs),
		SizeBytes:     totalSize,
		CreatedAt:     time.Now(),
	}
	index.Caches = append(index.Caches, entry)

	if err := index.Save(filepath.Dir(r.cacheDir)); err != nil {
		return fmt.Errorf("failed to save cache index: %w", err)
	}

	fmt.Printf("\nCache complete: %s\n", r.name)
	fmt.Printf("  Duration: %s\n", FormatDuration(r.totalSeconds))
	fmt.Printf("  Songs: %d\n", len(r.songs))
	fmt.Printf("  Size: %s\n", formatBytes(totalSize))

	return nil
}

// LoadCacheIndex loads the cache index from file
func LoadCacheIndex(offlineDir string) (*CacheIndex, error) {
	path := filepath.Join(offlineDir, "cache_index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CacheIndex{}, nil
		}
		return nil, err
	}

	var index CacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// Save writes the cache index to file
func (ci *CacheIndex) Save(offlineDir string) error {
	data, err := json.MarshalIndent(ci, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(offlineDir, "cache_index.json"), data, 0644)
}

// ListCaches returns all available caches
func ListCaches(offlineDir string) ([]CacheEntry, error) {
	index, err := LoadCacheIndex(offlineDir)
	if err != nil {
		return nil, err
	}

	// Validate each cache exists
	var valid []CacheEntry
	for _, entry := range index.Caches {
		cachePath := filepath.Join(offlineDir, entry.Name)
		if _, err := os.Stat(cachePath); err == nil {
			valid = append(valid, entry)
		}
	}

	return valid, nil
}

// DeleteCache removes a cache by name
func DeleteCache(offlineDir, name string) error {
	cachePath := filepath.Join(offlineDir, name)
	if err := os.RemoveAll(cachePath); err != nil {
		return err
	}

	// Update index
	index, err := LoadCacheIndex(offlineDir)
	if err != nil {
		return nil
	}

	var updated []CacheEntry
	for _, entry := range index.Caches {
		if entry.Name != name {
			updated = append(updated, entry)
		}
	}
	index.Caches = updated
	return index.Save(offlineDir)
}

// LoadCache loads a cache's songs for offline playback
func LoadCache(offlineDir, name string) ([]CachedSong, error) {
	cachePath := filepath.Join(offlineDir, name)
	metadataPath := filepath.Join(cachePath, "metadata.json")

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	var songs []CachedSong
	if err := json.Unmarshal(data, &songs); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Verify audio files exist, fix paths if needed
	for i, song := range songs {
		if _, err := os.Stat(song.AudioPath); err != nil {
			// Try to find file in songs directory
			files, _ := filepath.Glob(filepath.Join(cachePath, "songs", "*"))
			if i < len(files) {
				songs[i].AudioPath = files[i]
			}
		}
	}

	return songs, nil
}

// EstimateDiskUsage estimates disk usage for cache recording
func EstimateDiskUsage(bitrate, targetSeconds int, flacBytesPerSec int64) map[string]int64 {
	seconds := int64(targetSeconds)
	estimates := make(map[string]int64)

	// AAC bitrates in bytes per second
	aacRates := map[int]int64{
		1: 64 * 1000 / 8,
		2: 128 * 1000 / 8,
		3: 320 * 1000 / 8,
	}

	for br, rate := range aacRates {
		estimates[config.BitrateNames[br]] = rate * seconds
	}

	if flacBytesPerSec > 0 {
		estimates[config.BitrateNames[4]] = flacBytesPerSec * seconds
	}

	return estimates
}

// CalculateFLACBytesPerSecond calculates average FLAC bytes/second from favorites
func CalculateFLACBytesPerSecond(favoritesDir string) int64 {
	entries, err := os.ReadDir(favoritesDir)
	if err != nil {
		return 0
	}

	var totalSize int64
	var count int64

	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(strings.ToLower(entry.Name()), ".flac") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
			count++
		}
	}

	if count > 0 {
		// Average FLAC file size / 4 minutes (240 seconds) as estimate
		avgSize := totalSize / count
		return avgSize / 240
	}

	// Default FLAC estimate: ~300 KB/s
	return 300 * 1024
}

// GenerateCacheName creates an auto-generated cache name
func GenerateCacheName(station, bitrate int) string {
	stationName := strings.ToLower(strings.ReplaceAll(config.StationNames[station], " ", "_"))
	if stationName == "" {
		stationName = fmt.Sprintf("station%d", station)
	}

	bitrateName := config.BitrateNames[bitrate]
	if bitrateName == "" {
		bitrateName = fmt.Sprintf("%d", bitrate)
	}
	// Extract just the number part (e.g., "320k AAC" -> "320k")
	parts := strings.Split(bitrateName, " ")
	if len(parts) > 0 {
		bitrateName = parts[0]
	}

	now := time.Now()
	return fmt.Sprintf("%s_%s_%s", stationName, bitrateName, now.Format("20060102_1504"))
}

// ParseDuration parses duration string like "2h", "3.5h" to seconds
func ParseDuration(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	var hours float64
	_, err := fmt.Sscanf(s, "%f", &hours)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s", s)
	}

	return int(hours * 3600), nil
}

// ParseBitrate parses bitrate string like "320", "flac" to int
func ParseBitrate(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Check for named bitrates
	for br, name := range config.BitrateNames {
		lowerName := strings.ToLower(name)
		if lowerName == s || strings.Split(lowerName, " ")[0] == s {
			return br, nil
		}
	}

	// Try parsing as number
	br, err := strconv.Atoi(s)
	if err == nil {
		switch br {
		case 64:
			return 1, nil
		case 128:
			return 2, nil
		case 320:
			return 3, nil
		case 4:
			return 4, nil // FLAC index
		}
	}

	// Build helpful error message
	var options []string
	for _, name := range config.BitrateNames {
		options = append(options, fmt.Sprintf("  %s", name))
	}
	return 0, fmt.Errorf("invalid bitrate: %q\n\nValid bitrates:\n%s", s, strings.Join(options, "\n"))
}

// GetFreeDiskSpace returns free disk space in bytes for the given path.
// Returns 0 if the platform doesn't support querying disk space.
func GetFreeDiskSpace(path string) (int64, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	// Try to get free space via syscall on Unix-like systems
	// Windows would need golang.org/x/sys/windows
	sys := stat.Sys()
	if s, ok := sys.(interface {
		Bavail() int64
		Bsize() int64
	}); ok {
		return s.Bavail() * s.Bsize(), nil
	}

	// Return 0 if not available (caller should handle this gracefully)
	return 0, nil
}

// getFileExtension returns the file extension for a bitrate
func getFileExtension(bitrate int) string {
	switch bitrate {
	case 1, 2, 3:
		return "aac"
	case 4:
		return "flac"
	default:
		return "aac"
	}
}

// FormatDuration formats seconds to human-readable string
func FormatDuration(seconds int64) string {
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60

	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

// FormatBytes formats bytes into human-readable string
func FormatBytes(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
	}
}
