package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/cache"
	"rptui-bubbletea/internal/config"
	_ "rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/tui"
)

// CacheRequest holds parameters for a cache recording request
type CacheRequest struct {
	Duration string
	Station  string
	Bitrate  string
}

func main() {
	jukeboxMode := false
	var cacheRequests []CacheRequest
	offlineMode := false
	offlineCacheName := ""
	listCaches := false
	deleteCacheName := ""

	args := os.Args[1:]

	// Parse arguments
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printHelp()
			return
		case "--version", "-v":
			printVersion()
			return
		case "--lastfm-auth":
			handleLastFMAuth()
			return
		case "--jukebox", "-j":
			jukeboxMode = true
		case "--cache":
			req, skip := parseCacheRequest(args[i+1:])
			if req.Duration == "" {
				fmt.Fprintf(os.Stderr, "Error: --cache requires a duration argument (e.g., 2h, 3.5h)\n")
				os.Exit(1)
			}
			cacheRequests = append(cacheRequests, req)
			i += skip
		case "--offline":
			offlineMode = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				offlineCacheName = args[i+1]
				i++
			}
		case "--list-caches":
			listCaches = true
		case "--delete-cache":
			if i+1 < len(args) {
				deleteCacheName = args[i+1]
				i++
			} else {
				fmt.Fprintf(os.Stderr, "Error: --delete-cache requires a cache name\n")
				os.Exit(1)
			}
		}
	}

	// Handle management commands first
	if listCaches {
		handleListCaches()
		return
	}

	if deleteCacheName != "" {
		handleDeleteCache(deleteCacheName)
		return
	}

	// Handle cache recording mode
	if len(cacheRequests) > 0 {
		handleCacheRecording(cacheRequests)
		return
	}

	// Handle offline playback mode
	if offlineMode {
		handleOfflineMode(offlineCacheName)
		return
	}

	// Normal mode or jukebox mode
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	theme, err := config.LoadTheme(cfg.ColorsFile, cfg.Theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load theme: %v\n", err)
		theme = config.DefaultTheme()
	}

	m := tui.NewModel(cfg, theme, jukeboxMode)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Radio Paradise programming is human-curated and commercial-free.")
	fmt.Fprintln(os.Stderr, "Please consider supporting RP by visiting their website:")
	fmt.Fprintln(os.Stderr, "https://radioparadise.com/donate")
}

// handleLastFMAuth handles the --lastfm-auth flag
func handleLastFMAuth() {
	sessionKey, err := api.LastFMDoAuth()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	cfg.LastFM.SessionKey = sessionKey
	cfg.LastFM.Enabled = true
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Session key saved to config. Last.fm scrobbling is now enabled.")
}

// parseCacheRequest parses a --cache argument and its optional station/bitrate
// Returns the request and number of additional args consumed
func parseCacheRequest(args []string) (CacheRequest, int) {
	req := CacheRequest{}
	skip := 0

	if len(args) == 0 {
		return req, skip
	}

	req.Duration = args[0]
	skip++

	if len(args) > skip && !strings.HasPrefix(args[skip], "--") {
		req.Station = args[skip]
		skip++
	}

	if len(args) > skip && !strings.HasPrefix(args[skip], "--") {
		req.Bitrate = args[skip]
		skip++
	}

	return req, skip
}

// handleListCaches lists all available offline caches
func handleListCaches() {
	offlineDir := filepath.Join(xdg.CacheHome, "rptui", "offline")

	caches, err := cache.ListCaches(offlineDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing caches: %v\n", err)
		os.Exit(1)
	}

	if len(caches) == 0 {
		fmt.Println("No offline caches found.")
		return
	}

	fmt.Printf("Available offline caches (%d total):\n\n", len(caches))
	for i, c := range caches {
		stationName := config.StationNames[c.Station]
		if stationName == "" {
			stationName = fmt.Sprintf("Station %d", c.Station)
		}
		bitrateName := config.BitrateNames[c.Bitrate]
		if bitrateName == "" {
			bitrateName = fmt.Sprintf("%d", c.Bitrate)
		}

		fmt.Printf("%d. %s - %s (%s) - %s - %d songs - %s\n",
			i+1,
			c.CreatedAt.Format("2006-01-02"),
			stationName,
			bitrateName,
			cache.FormatDuration(int64(c.ActualSeconds)),
			c.SongCount,
			formatBytes(c.SizeBytes),
		)
	}
}

// handleDeleteCache deletes a specific cache
func handleDeleteCache(name string) {
	offlineDir := filepath.Join(xdg.CacheHome, "rptui", "offline")

	// Verify cache exists
	caches, err := cache.ListCaches(offlineDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing caches: %v\n", err)
		os.Exit(1)
	}

	found := false
	for _, c := range caches {
		if c.Name == name {
			found = true
			break
		}
	}

	if !found {
		fmt.Fprintf(os.Stderr, "Cache '%s' not found.\n", name)
		os.Exit(1)
	}

	// Confirm deletion
	fmt.Printf("Are you sure you want to delete cache '%s'? (y/n): ", name)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(line)) != "y" {
		fmt.Println("Deletion cancelled.")
		return
	}

	if err := cache.DeleteCache(offlineDir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting cache: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Cache '%s' deleted.\n", name)
}

// handleCacheRecording handles the --cache recording mode
func handleCacheRecording(requests []CacheRequest) {
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	offlineDir := filepath.Join(xdg.CacheHome, "rptui", "offline")
	if err := os.MkdirAll(offlineDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating offline directory: %v\n", err)
		os.Exit(1)
	}

	// Calculate FLAC estimate from existing favorites
	flacBytesPerSec := cache.CalculateFLACBytesPerSecond(cfg.FavoritesDir)

	// Process each cache request sequentially
	for i, req := range requests {
		if len(requests) > 1 {
			fmt.Printf("\n=== Cache request %d of %d ===\n", i+1, len(requests))
		}

		// Parse duration
		targetSeconds, err := cache.ParseDuration(req.Duration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Determine station
		station := cfg.Channel
		if req.Station != "" {
			station, err = parseStation(req.Station)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Determine bitrate
		bitrate := cfg.Bitrate
		if req.Bitrate != "" {
			bitrate, err = cache.ParseBitrate(req.Bitrate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Generate cache name
		cacheName := cache.GenerateCacheName(station, bitrate)

		// Show disk space estimate
		estimates := cache.EstimateDiskUsage(bitrate, targetSeconds, flacBytesPerSec)
		currentBitrateName := config.BitrateNames[bitrate]
		estimatedSize := estimates[currentBitrateName]

		freeSpace, err := cache.GetFreeDiskSpace(offlineDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not determine free disk space: %v\n", err)
			freeSpace = 0
		}

		fmt.Printf("\nCache recording configuration:\n")
		fmt.Printf("  Station: %s\n", config.StationNames[station])
		fmt.Printf("  Bitrate: %s\n", currentBitrateName)
		fmt.Printf("  Target duration: %s\n", cache.FormatDuration(int64(targetSeconds)))
		fmt.Printf("  Estimated size: %s\n", formatBytes(estimatedSize))

		if freeSpace > 0 {
			fmt.Printf("  Free disk space: %s\n", formatBytes(freeSpace))

			remainingAfter := freeSpace - estimatedSize
			if remainingAfter < 0 {
				fmt.Fprintf(os.Stderr, "\nError: Not enough disk space. Need %s, have %s.\n",
					formatBytes(estimatedSize), formatBytes(freeSpace))
				os.Exit(1)
			}

			if remainingAfter < 1024*1024*1024 { // 1 GB
				fmt.Printf("\n⚠ Warning: Less than 1 GB will remain after recording (%s remaining).\n",
					formatBytes(remainingAfter))
			}
		}

		// Confirm
		fmt.Print("\nContinue? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			fmt.Println("Recording cancelled.")
			if len(requests) > 1 && i < len(requests)-1 {
				fmt.Println("Skipping remaining requests.")
			}
			return
		}

		// Create API client
		rpAPI := api.NewRadioParadiseAPI(station, bitrate)

		// Create cache recorder
		recorder := cache.NewCacheRecorder(offlineDir, cacheName, station, bitrate, targetSeconds)
		if err := recorder.Setup(); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up cache: %v\n", err)
			os.Exit(1)
		}

		// Create block fetcher function
		blockFetcher := func(st, br int) (cache.BlockInfo, error) {
			block, err := rpAPI.GetBlock(nil)
			if err != nil {
				return cache.BlockInfo{}, err
			}

			songs, _ := rpAPI.ParseBlockSongs(block)
			var cached []cache.CachedSong
			for _, s := range songs {
				cached = append(cached, cache.CachedSong{
					EventID:         s.EventID,
					Title:           s.Title,
					Artist:          s.Artist,
					Album:           s.Album,
					Year:            s.Year,
					Duration:        s.Duration,
					GaplessURL:      s.GaplessURL,
					CoverLarge:      s.CoverLarge,
					Rating:          s.Rating,
					ListenerRating:  s.ListenerRating,
					SchedTimeMillis: s.PlayTime,
				})
			}
			return cache.BlockInfo{
				BlockID: block.BlockID,
				Songs:   cached,
			}, nil
		}

		// Record with progress output
		fmt.Printf("\nStarting cache recording: %s\n", cacheName)
		fmt.Println("Press Ctrl+C to cancel (will prompt for confirmation).")
		fmt.Println()

		err = recorder.Record(blockFetcher, func(progress cache.ProgressInfo) {
			// Text-only progress
			fmt.Printf("Downloading: %s (%s)... done (%s)\n",
				progress.CurrentSong,
				cache.FormatDuration(progress.Duration),
				formatBytes(progress.Size),
			)
			fmt.Printf("Progress: %s / %s (%.1f%%) - %d songs downloaded\n",
				cache.FormatDuration(progress.TotalSeconds),
				cache.FormatDuration(int64(progress.TargetSeconds)),
				float64(progress.TotalSeconds)/float64(progress.TargetSeconds)*100,
				progress.SongIndex,
			)
			fmt.Println()
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "\nRecording failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// handleOfflineMode handles the --offline playback mode
func handleOfflineMode(cacheName string) {
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	offlineDir := filepath.Join(xdg.CacheHome, "rptui", "offline")

	// If no cache name specified, prompt for selection
	if cacheName == "" {
		caches, err := cache.ListCaches(offlineDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing caches: %v\n", err)
			os.Exit(1)
		}

		if len(caches) == 0 {
			fmt.Fprintf(os.Stderr, "No offline caches found. Use --cache to record first.\n")
			os.Exit(1)
		}

		if len(caches) == 1 {
			cacheName = caches[0].Name
		} else {
			// Show selection prompt
			fmt.Println("Multiple caches available:")
			fmt.Println()
			for i, c := range caches {
				stationName := config.StationNames[c.Station]
				if stationName == "" {
					stationName = fmt.Sprintf("Station %d", c.Station)
				}
				bitrateName := config.BitrateNames[c.Bitrate]
				if bitrateName == "" {
					bitrateName = fmt.Sprintf("%d", c.Bitrate)
				}

				fmt.Printf("%d. %s %s %s %s\n",
					i+1,
					c.CreatedAt.Format("2006-01-02"),
					stationName,
					bitrateName,
					cache.FormatDuration(int64(c.ActualSeconds)),
				)
			}

			fmt.Printf("\nPlease enter 1-%d to select: ", len(caches))
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			var choice int
			fmt.Sscanf(strings.TrimSpace(line), "%d", &choice)

			if choice < 1 || choice > len(caches) {
				fmt.Fprintf(os.Stderr, "Invalid selection.\n")
				os.Exit(1)
			}

			cacheName = caches[choice-1].Name
		}
	}

	// Load cache
	songs, err := cache.LoadCache(offlineDir, cacheName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading cache '%s': %v\n", cacheName, err)
		os.Exit(1)
	}

	if len(songs) == 0 {
		fmt.Fprintf(os.Stderr, "Cache '%s' is empty.\n", cacheName)
		os.Exit(1)
	}

	theme, err := config.LoadTheme(cfg.ColorsFile, cfg.Theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load theme: %v\n", err)
		theme = config.DefaultTheme()
	}

	// Launch TUI in offline mode
	m := tui.NewOfflineModel(cfg, theme, songs, cacheName)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Radio Paradise programming is human-curated and commercial-free.")
	fmt.Fprintln(os.Stderr, "Please consider supporting RP by visiting their website:")
	fmt.Fprintln(os.Stderr, "https://radioparadise.com/donate")
}

// parseStation parses a station name or number
func parseStation(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Try exact match first
	for ch, name := range config.StationNames {
		if strings.ToLower(name) == s {
			return ch, nil
		}
	}

	// Try partial match (e.g., "main" matches "Main Mix")
	for ch, name := range config.StationNames {
		if strings.Contains(strings.ToLower(name), s) {
			return ch, nil
		}
	}

	// Try parsing as number
	var station int
	_, err := fmt.Sscanf(s, "%d", &station)
	if err == nil {
		if _, ok := config.StationNames[station]; ok {
			return station, nil
		}
	}

	// Build helpful error message
	var options []string
	for ch, name := range config.StationNames {
		options = append(options, fmt.Sprintf("  %d - %s", ch, name))
	}
	return 0, fmt.Errorf("invalid station: %q\n\nValid stations:\n%s", s, strings.Join(options, "\n"))
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

// printHelp displays usage information and exits
func printHelp() {
	help := `Radio Paradise TUI - A terminal UI for Radio Paradise

USAGE:
    rptui [FLAGS]

FLAGS:
    -h, --help              Show this help message and exit
    -v, --version           Show version information and exit
    -j, --jukebox           Launch in jukebox mode (random favorites playback)

OFFLINE CACHE:
    --cache <DURATION> [STATION] [BITRATE]
                            Record audio cache for offline playback
                            DURATION: recording length (e.g., 2h, 3.5h)
                            STATION: station name or number (default: from config)
                            BITRATE: bitrate name or number (default: from config)
                            Example: rptui --cache 2h "Rock Mix" FLAC

    --offline [CACHE_NAME]  Launch TUI in offline playback mode
                            If CACHE_NAME omitted, prompts for selection
                            Example: rptui --offline 2024-01-15_main_mix_320k

    --list-caches           List all available offline caches and exit

    --delete-cache <NAME>   Delete a named offline cache (prompts for confirmation)

ACTIONS:
    --lastfm-auth           Run Last.fm OAuth authentication flow and save session key

EXAMPLES:
    rptui                   Launch with default settings
    rptui -j                Launch in jukebox mode
    rptui --cache 4h        Record 4 hours of current station/bitrate
    rptui --offline         Play back a previously recorded cache
    rptui --list-caches     See what caches are available

STATIONS:
    0 - Main Mix     1 - Mellow Mix    2 - Rock Mix
    3 - Global Mix   5 - Beyond...

BITRATES:
    1 - 64k AAC   2 - 128k AAC   3 - 320k AAC   4 - FLAC

CONFIGURATION:
    Config file: ~/.config/rptui/config.toml
    Cache dir:   $XDG_CACHE_HOME/rptui/
    Log file:    rptui.log (in project directory)
`
	fmt.Print(help)
}

// printVersion displays version information and exits
func printVersion() {
	version := "dev"
	goVersion := runtime.Version()
	osArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}

	fmt.Printf("rptui %s (%s, %s)\n", version, goVersion, osArch)
}
