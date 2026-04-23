package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/adrg/xdg"
	"github.com/pdfrg/rptui/internal/api"
	"github.com/pdfrg/rptui/internal/cache"
	"github.com/pdfrg/rptui/internal/config"
	_ "github.com/pdfrg/rptui/internal/loginit"
	"github.com/pdfrg/rptui/internal/smad"
	"github.com/pdfrg/rptui/internal/tui"
)

var Version = "dev"

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
	layoutOverride := ""
	sleepTimerDuration := time.Duration(0)
	alarmTime := time.Time{}
	setupDJSkip := false

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
		case "--rp-auth":
			handleRPAuth()
			return
		case "--jukebox", "-j":
			jukeboxMode = true
		case "--layout":
			if i+1 < len(args) {
				layoutOverride = args[i+1]
				i++
			} else {
				fmt.Fprintf(os.Stderr, "Error: --layout requires an argument (large, medium, compact, narrow)\n")
				os.Exit(1)
			}
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
		case "--create-colors-file":
			handleCreateColorsFile()
			return
		case "--test-terminal-colors":
			handleTestTerminalColors()
			return
		case "--setup-dj-skip":
			setupDJSkip = true
			continue
		case "--sleep":
			if i+1 < len(args) {
				d, err := time.ParseDuration(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: --sleep requires a duration (e.g., 20m, 1.5h)\n")
					os.Exit(1)
				}
				sleepTimerDuration = d
				i++
			} else {
				fmt.Fprintf(os.Stderr, "Error: --sleep requires a duration argument (e.g., 20m, 1.5h)\n")
				os.Exit(1)
			}
		case "--alarm":
			if i+1 < len(args) {
				alarm, err := parseAlarmTime(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					fmt.Fprintf(os.Stderr, "Valid formats: 7:20am, 7:20a.m., 19:20, 7:20 a\n")
					os.Exit(1)
				}
				alarmTime = alarm
				i++
			} else {
				fmt.Fprintf(os.Stderr, "Error: --alarm requires a time argument (e.g., 7:20am, 19:20)\n")
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
		handleOfflineMode(offlineCacheName, layoutOverride)
		return
	}

	// Normal mode or jukebox mode
	if setupDJSkip {
		cacheDir := filepath.Join(xdg.CacheHome, "rptui")
		if smad.IsSetupComplete(cacheDir) {
			fmt.Println("DJ skip setup is already complete. Nothing to do.")
			return
		}

		fmt.Println("This will set up DJ speech detection by:")
		fmt.Println("  1. Creating an isolated Python virtual environment")
		fmt.Println("  2. Installing PyTorch + audio libraries (~2.5GB download, 10-20 min)")
		fmt.Println("  3. Downloading the TVSM speech detection model (~11MB)")
		fmt.Println("  4. Converting the model to runtime format")
		fmt.Println()
		fmt.Println("Disk space required: ~2.5GB")
		fmt.Println()
		fmt.Print("Continue? [y/N] ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("Aborted.")
			return
		}

		err := smad.Setup("", cacheDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error setting up DJ skip: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("DJ skip setup complete.")
		return
	}

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

	// Handle alarm mode: block until alarm time, then start app
	if !alarmTime.IsZero() {
		handleAlarmMode(alarmTime)
		// After alarm fires, proceed to normal TUI
	}

	m := tui.NewModel(cfg, theme, jukeboxMode, layoutOverride, sleepTimerDuration)

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

// handleRPAuth handles the --rp-auth flag for interactive RP login
func handleRPAuth() {
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	authClient := api.NewRPAuthClient()

	// Check if already authenticated
	authDir := filepath.Join(xdg.ConfigHome, "rptui")
	authPath := filepath.Join(authDir, "auth.toml")
	if err := authClient.LoadState(authPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load auth state: %v\n", err)
	}

	if authClient.HasAuth() {
		fmt.Printf("Currently authenticated as: %s (user ID: %s)\n", authClient.Username(), authClient.UserID())
		fmt.Print("Re-authenticate? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(line)) != "y" {
			fmt.Println("Authentication unchanged.")
			return
		}
	}

	// Prompt for credentials
	fmt.Print("RP username: ")
	reader := bufio.NewReader(os.Stdin)
	username, _ := reader.ReadString('\n')
	username = strings.TrimSpace(username)

	fmt.Print("RP password: ")
	password, _ := reader.ReadString('\n')
	password = strings.TrimSpace(password)

	if username == "" || password == "" {
		fmt.Println("Username and password are required.")
		os.Exit(1)
	}

	// Attempt authentication
	fmt.Println("\nAuthenticating with Radio Paradise...")
	if err := authClient.Login(username, password); err != nil {
		fmt.Fprintf(os.Stderr, "Authentication failed: %v\n", err)
		os.Exit(1)
	}

	// Save credentials to config
	cfg.RPAuth.Username = username
	cfg.RPAuth.Password = password
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save config: %v\n", err)
	}

	// Save session tokens
	if err := authClient.SaveState(authPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save auth state: %v\n", err)
	}

	fmt.Printf("Successfully authenticated as %s (user ID: %s)\n", username, authClient.UserID())
	fmt.Println("Session tokens saved. Authenticated features are now available.")
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
func handleOfflineMode(cacheName string, layoutOverride string) {
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
	m := tui.NewOfflineModel(cfg, theme, songs, cacheName, layoutOverride)

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
        --layout LAYOUT     Set UI layout: large, medium, compact, narrow
                             large: full layout with all elements (default)
                             medium: no bottom view (no playlist/lyrics/visualizer)
                             compact: no album art, no bottom view, mini footer
                             narrow: album art top-left, now playing below, mini footer
 
 OFFLINE CACHE:
    --cache <DURATION> [STATION] [BITRATE]
                            Record audio cache for offline playback
                            DURATION: recording length (e.g., 2h, 3.5h)
                            STATION: station name or number (default: from config)
                            BITRATE: bitrate name or number (default: from config)
                            Example: rptui --cache 2h Rock FLAC (station substrings accepted)

    --offline [CACHE_NAME]  Launch TUI in offline playback mode
                            If CACHE_NAME omitted, prompts for selection
                            Example: rptui --offline 2024-01-15_main_mix_320k

    --list-caches           List all available offline caches and exit

    --delete-cache <NAME>   Delete a named offline cache (prompts for confirmation)

SLEEP TIMER / ALARM:
    --sleep <DURATION>       Start sleep timer (e.g., 20m, 1.5h)
                            App pauses and quits after timer expires
    --alarm <TIME>           Schedule alarm (e.g., 7:20am, 7:20 a.m., 19:20)
                            App starts at specified time

ACTIONS:
	--setup-dj-skip     Download TVSM model for DJ speech skipping
	                    (~2.5GB Python dependencies, 10-20 min install time)
    --lastfm-auth           Run Last.fm OAuth authentication flow and save session key
    --rp-auth               Authenticate with Radio Paradise account
                             Enables user ratings, comments, favorites sync, and My Paradise channel
                             (optional — all features work without an RP account)
    --create-colors-file    Print color theme template to stdout
    --test-terminal-colors  Query and display terminal color information

EXAMPLES:
    rptui                   Launch with default settings
    rptui -j                Launch in jukebox mode
    rptui --cache 4h        Record 4 hours of current station/bitrate
    rptui --offline         Play back a previously recorded cache
    rptui --list-caches     See what caches are available
    rptui --sleep 30m       Auto-quit after 30 minutes
    rptui --alarm 7:20am    Start app at 7:20am tomorrow

STATIONS:
    0 - The Main Mix  1 - Mellow Mix    2 - RockIt!
    3 - The Globe     42 - Serenity     5 - Beyond...
    945 - KFAT

BITRATES:
    1 - 64k AAC   2 - 128k AAC   3 - 320k AAC   4 - FLAC

CONFIGURATION:
    Config file: ~/.config/rptui/config.toml
    Config dir: $XDG_CONFIG_HOME/rptui/ (default: ~/.config/rptui/)
    Cache dir:   $XDG_CACHE_HOME/rptui/ (default: ~/.cache/rptui/)
    Log file:    $XDG_STATE_HOME/rptui/rptui.log (default: ~/.local/state/rptui/)
`
	fmt.Print(help)
}

// printVersion displays version information and exits
func printVersion() {
	goVersion := runtime.Version()
	osArch := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	version := Version
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				version = info.Main.Version
			}
		}
	}

	fmt.Printf("rptui %s (%s, %s)\n", version, goVersion, osArch)
}

// parseAlarmTime parses an alarm time string and returns the target time.
// Supported formats: 7:20am, 7:20a.m., 7:20 a, 7:20, 19:20, 7:20AM, etc.
func parseAlarmTime(s string) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	// Regex patterns for various formats
	patterns := []struct {
		regex    *regexp.Regexp
		is24Hour bool
	}{
		// 12-hour: 7:20am, 7:20 a, 7:20am, 7:20 a.m.
		{regexp.MustCompile(`^(\d{1,2}):(\d{2})\s*(a\.?m\.?|p\.?m\.?)$`), false},
		// 24-hour: 19:20, 07:20
		{regexp.MustCompile(`^(\d{1,2}):(\d{2})$`), true},
	}

	now := time.Now()
	var hour, minute int
	var isPM bool

	for _, p := range patterns {
		match := p.regex.FindStringSubmatch(s)
		if len(match) >= 3 {
			fmt.Sscanf(match[1], "%d", &hour)
			fmt.Sscanf(match[2], "%d", &minute)

			// Check for AM/PM
			if len(match) >= 4 && match[3] != "" {
				ampm := match[3]
				// Remove dots and spaces
				ampm = strings.ReplaceAll(ampm, ".", "")
				ampm = strings.ReplaceAll(ampm, " ", "")
				isPM = strings.HasPrefix(ampm, "p")
			}

			// Validate
			if hour > 23 || hour < 0 || minute > 59 || minute < 0 {
				continue
			}

			// Convert 12-hour to 24-hour
			if !p.is24Hour {
				if isPM && hour != 12 {
					hour += 12
				} else if !isPM && hour == 12 {
					hour = 0
				}
			}

			// Build target time (today or tomorrow)
			target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

			// If time has passed today, set to tomorrow
			if !target.After(now) {
				target = target.Add(24 * time.Hour)
			}

			return target, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid alarm time: %q (valid formats: 7:20am, 7:20 a.m., 19:20)", s)
}

// handleAlarmMode blocks until the alarm time, then returns
func handleAlarmMode(alarmTime time.Time) {
	now := time.Now()
	duration := alarmTime.Sub(now)

	if duration <= 0 {
		return
	}

	// Print info message
	fmt.Fprintf(os.Stderr, "Alarm scheduled for %s. Sleeping until then...\n", alarmTime.Format("Mon 3:04 PM"))

	// Spinner characters for animation
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	spinnerIdx := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	lastPrint := time.Now().Add(-1 * time.Second)

	for {
		select {
		case <-ticker.C:
			now := time.Now()

			// Check if alarm time reached
			if now.After(alarmTime) || now.Equal(alarmTime) {
				fmt.Fprintln(os.Stderr, "\rAlarm time reached! Starting Radio Paradise...")
				return
			}

			// Print spinner every second
			if now.Sub(lastPrint) >= time.Second {
				remaining := alarmTime.Sub(now)
				hrs := int(remaining.Hours())
				mins := int(remaining.Minutes()) % 60
				secs := int(remaining.Seconds()) % 60

				var timeStr string
				if hrs > 0 {
					timeStr = fmt.Sprintf("%d:%02d:%02d", hrs, mins, secs)
				} else {
					timeStr = fmt.Sprintf("%d:%02d", mins, secs)
				}

				fmt.Fprintf(os.Stderr, "\r%c %s remaining  ", spinners[spinnerIdx%len(spinners)], timeStr)

				spinnerIdx++
				lastPrint = now
			}
		}
	}
}

// handleCreateColorsFile outputs a color template file to stdout
func handleCreateColorsFile() {
	template := `# RadioParadise TUI Color Theme
# Place in ~/.config/rptui/colors.toml and reference in config.toml:
#   colors_file = "/home/username/.config/rptui/colors.toml"
# Priority: colors_file > theme > Omarchy > Catppuccin Mocha fallback

# [base] - Core UI colors (required)
# [colors] - ANSI 256-color palette (optional, used for fallbacks)

[base]
# Main UI colors
background = "#1e1e2e"  # Window background, panels
foreground = "#cdd6f4"  # Primary text, song info
accent = "#89b4fa"     # Song titles, hotkeys, progress bar gradient, current selection
muted = "#6c7086"      # Secondary text, borders, inactive elements
cursor = "#f5c2e7"     # Playback position indicator, current playlist item

[colors]
# ANSI 256-color palette (colors 0-7 standard, 8-15 bright)
# Used as fallbacks when accent/cursor need to differ from foreground
color0  = "#45475a"    # black
color1  = "#f38ba8"    # red
color2  = "#a6e3a1"    # green
color3  = "#f9e2af"    # yellow
color4  = "#89b4fa"    # blue
color5  = "#f5c2e7"    # magenta
color6  = "#94e2d5"    # cyan
color7  = "#bac2de"    # white
color8  = "#585b70"    # bright black (gray)
color9  = "#f38ba8"    # bright red
color10 = "#a6e3a1"    # bright green
color11 = "#f9e2af"    # bright yellow
color12 = "#89b4fa"    # bright blue
color13 = "#f5c2e7"    # bright magenta
color14 = "#94e2d5"    # bright cyan
color15 = "#a6adc8"    # bright white
 `
	fmt.Print(template)
}

// handleTestTerminalColors queries and displays terminal color information
func handleTestTerminalColors() {
	_ = config.IsTerminalColorAvailable() // Ensure query runs
	fg, bg, _, success, _ := config.TestTerminalColors()
	_ = success

	fmt.Println("=== Terminal Color Detection ===")
	fmt.Println()

	ok := fg != "" && bg != ""
	if ok {
		fmt.Println("Status: DETECTED")
	} else {
		fmt.Println("Status: NOT DETECTED")
		fmt.Println()
		fmt.Println("Terminal color detection works best in modern terminals like:")
		fmt.Println("  - Kitty, Ghostty, iTerm2, Windows Terminal, Rio")
		fmt.Println("  - Does not work inside screen/tmux")
		fmt.Println("  - Set COLORFGBG environment variable as fallback:")
		fmt.Println("    export COLORFGBG=7;0   # white foreground, black background")
	}
	fmt.Println()

	if fg != "" {
		fmt.Printf("Default Foreground: %s\n", fg)
	} else {
		fmt.Println("Default Foreground: (not detected)")
	}
	if bg != "" {
		fmt.Printf("Default Background: %s\n", bg)
	} else {
		fmt.Println("Default Background: (not detected)")
	}
	fmt.Println()

	// Get palette indices from config (default values)
	cfg := config.DefaultConfig()
	cursorIdx := cfg.TerminalPalette.Cursor
	accentIdx := cfg.TerminalPalette.Accent
	mutedIdx := cfg.TerminalPalette.Muted

	// Try to get palette colors from cache
	_, _, cachedPalette, ok := config.GetCachedTerminalColors()

	fmt.Println("Palette (index: color):")
	for i := 0; i < 16; i++ {
		color := ""
		if ok && cachedPalette != nil {
			color = cachedPalette[i]
		}
		if color == "" {
			color = "(not detected)"
		}
		idxStr := fmt.Sprintf("%d", i)
		if len(idxStr) == 1 {
			idxStr = " " + idxStr
		}
		highlight := ""
		if i == cursorIdx {
			highlight = " <- cursor"
		}
		if i == accentIdx {
			highlight += " <- accent"
		}
		if i == mutedIdx {
			highlight += " <- muted"
		}
		fmt.Printf("  %s: %s%s\n", idxStr, color, highlight)
	}
	fmt.Println()

	fmt.Println("Indices used for theme (when disable_theme=true):")
	fmt.Printf("  Cursor: %d\n", cursorIdx)
	fmt.Printf("  Accent: %d\n", accentIdx)
	fmt.Printf("  Muted: %d\n", mutedIdx)
}
