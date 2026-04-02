package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ScrobbleEntry represents a failed scrobble to be retried later.
type ScrobbleEntry struct {
	Artist       string `json:"artist"`
	Track        string `json:"track"`
	Album        string `json:"album,omitempty"`
	DurationSecs int    `json:"duration_secs"`
	Timestamp    int64  `json:"timestamp"`
	Service      string `json:"service"` // "fm" or "lb"
}

// ScrobbleCache manages a disk-backed queue of failed scrobbles.
type ScrobbleCache struct {
	dir string
	mu  sync.Mutex
}

// NewScrobbleCache creates a cache backed by the given directory.
func NewScrobbleCache(dir string) *ScrobbleCache {
	return &ScrobbleCache{dir: dir}
}

func (c *ScrobbleCache) cachePath() string {
	return filepath.Join(c.dir, "pending.json")
}

// Load reads all cached scrobbles from disk. Returns empty slice if none.
func (c *ScrobbleCache) Load() []ScrobbleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.cachePath())
	if err != nil {
		return nil
	}

	var entries []ScrobbleEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		scrobbleLogger.Printf("Corrupt scrobble cache, discarding: %v", err)
		return nil
	}
	return entries
}

// Add appends a scrobble entry to the disk cache.
func (c *ScrobbleCache) Add(entry ScrobbleEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return fmt.Errorf("failed to create scrobble cache dir: %w", err)
	}

	var entries []ScrobbleEntry
	data, err := os.ReadFile(c.cachePath())
	if err == nil {
		json.Unmarshal(data, &entries) // ignore parse errors, start fresh
	}

	entries = append(entries, entry)

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scrobble cache: %w", err)
	}
	return os.WriteFile(c.cachePath(), out, 0644)
}

// Replace overwrites the cache with the given entries.
func (c *ScrobbleCache) Replace(entries []ScrobbleEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(entries) == 0 {
		return os.Remove(c.cachePath())
	}

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scrobble cache: %w", err)
	}
	return os.WriteFile(c.cachePath(), out, 0644)
}
