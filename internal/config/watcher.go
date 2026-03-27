package config

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adrg/xdg"
)

type ThemeWatcher struct {
	mu          sync.Mutex
	events      chan string
	closed      bool
	colorsFile  string
	omarchyPath string
	lastMtime   map[string]time.Time
	stopChan    chan struct{}
}

func NewThemeWatcher(colorsFile string) *ThemeWatcher {
	return &ThemeWatcher{
		events:     make(chan string, 1),
		colorsFile: colorsFile,
		lastMtime:  make(map[string]time.Time),
		stopChan:   make(chan struct{}),
	}
}

func (tw *ThemeWatcher) Events() <-chan string {
	return tw.events
}

func (tw *ThemeWatcher) Start() error {
	tw.omarchyPath = filepath.Join(xdg.ConfigHome, "omarchy", "current", "theme", "colors.toml")

	// Initialize mtimes to avoid spurious reload on first poll
	if tw.colorsFile != "" {
		if info, err := os.Stat(tw.colorsFile); err == nil {
			tw.lastMtime[tw.colorsFile] = info.ModTime()
		}
	}
	if info, err := os.Stat(tw.omarchyPath); err == nil {
		tw.lastMtime[tw.omarchyPath] = info.ModTime()
	}

	go tw.run()

	return nil
}

func (tw *ThemeWatcher) run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tw.checkForChanges()
		case <-tw.stopChan:
			return
		}
	}
}

func (tw *ThemeWatcher) checkForChanges() {
	paths := []string{}
	if tw.colorsFile != "" {
		paths = append(paths, tw.colorsFile)
	}
	paths = append(paths, tw.omarchyPath)

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		newMtime := info.ModTime()
		lastMtime, exists := tw.lastMtime[path]

		if !exists || !newMtime.Equal(lastMtime) {
			tw.lastMtime[path] = newMtime

			select {
			case tw.events <- path:
			default:
			}
		}
	}
}

func (tw *ThemeWatcher) Close() {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.closed {
		return
	}
	tw.closed = true
	close(tw.stopChan)
	close(tw.events)
}
