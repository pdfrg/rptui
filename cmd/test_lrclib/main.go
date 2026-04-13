package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pdfrg/rptui/internal/api"
)

const userAgent = "rptui-test/1.0 (https://github.com/user/rptui; test@example.com)"

// LRCLib API rate limiting
var (
	lrcHTTP     = &http.Client{Timeout: 15 * time.Second}
	lastLRCCall time.Time
	lrcMu       sync.Mutex
)

const minLRCDelay = 250 * time.Millisecond // LRCLib asks for 2 req/s but we're conservative
const burstLimit = 10                      // Max requests before backoff
var requestCount = 0
var requestMu sync.Mutex

// lrcGet makes a rate-limited GET request to LRCLib
func lrcGet(ctx context.Context, urlStr string) (*http.Response, error) {
	lrcMu.Lock()
	defer lrcMu.Unlock()

	requestMu.Lock()
	requestCount++
	count := requestCount
	requestMu.Unlock()

	// Rate limit delay
	elapsed := time.Since(lastLRCCall)
	if elapsed < minLRCDelay {
		time.Sleep(minLRCDelay - elapsed)
	}
	lastLRCCall = time.Now()

	// Occasional extra delay to avoid bulk limits
	if count%burstLimit == 0 {
		time.Sleep(500 * time.Millisecond)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := lrcHTTP.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle rate limiting - LRCLib returns 429
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		fmt.Printf("  [Rate limited, waiting %v]\n", retryAfter)
		resp.Body.Close()
		time.Sleep(retryAfter)

		req2, _ := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		req2.Header.Set("User-Agent", userAgent)
		return lrcHTTP.Do(req2)
	}

	return resp, nil
}

func main() {
	var (
		rpChannels   string
		testProblems bool
		verbose      bool
		artist       string
		track        string
		album        string
		duration     float64 // in seconds
	)
	flag.StringVar(&rpChannels, "channels", "", "RP channels to pull from (comma-separated, e.g. '0,1,3')")
	flag.BoolVar(&testProblems, "problems", false, "Test problem tracks from fixes.txt")
	flag.BoolVar(&verbose, "v", false, "Verbose output with detailed diagnostics")
	flag.StringVar(&artist, "artist", "", "Test a single artist name")
	flag.StringVar(&track, "track", "", "Test a single track name")
	flag.StringVar(&album, "album", "", "Album name for single test")
	flag.Float64Var(&duration, "duration", 0, "Duration in seconds for single test")
	flag.Parse()

	ctx := context.Background()

	// Single artist/track test mode
	if artist != "" && track != "" {
		if album == "" {
			album = "—"
		}
		if duration == 0 {
			duration = 240
		}
		testSingleTrack(ctx, artist, track, album, duration, verbose)
		return
	}

	// Playlist test mode
	var tests []TestTrack

	switch {
	case testProblems:
		tests = problemTracks
	case rpChannels != "":
		tests = fetchRPTracks(ctx, rpChannels)
	default:
		fmt.Println("No mode specified. Use -h for options.")
		fmt.Println("Testing problem tracks + sampling from RP Main Mix.")
		tests = problemTracks
		rpTracks := fetchRPTracks(ctx, "0")
		for _, t := range rpTracks {
			if !hasTrack(tests, t) {
				tests = append(tests, t)
			}
		}
	}

	fmt.Printf("Testing %d tracks\n\n", len(tests))

	results := make(map[string]*TestResult)
	for i, t := range tests {
		fmt.Printf("[%d/%d] %s - %s\n", i+1, len(tests), t.Artist, t.Track)
		r := testTrack(ctx, t, verbose)
		results[fmt.Sprintf("%s - %s", t.Artist, t.Track)] = r
		fmt.Println()
	}

	printSummary(results)
}

type TestTrack struct {
	Artist   string
	Track    string
	Album    string
	Duration float64 // seconds
}

type TestResult struct {
	Track       string
	Artist      string
	Album       string
	Duration    float64
	HasPlain    bool
	HasSynced   bool
	PlainLen    int
	SyncedLen   int
	DurationAPI float64 // from LRCLib
	Lang        string
	Matched     string // How we matched
	TopResults  []ResultItem
	Error       string
	Steps       []string
}

type ResultItem struct {
	Rank         int
	ArtistName   string
	TrackName    string
	AlbumName    string
	Duration     float64
	DurationDiff int
	HasPlain     bool
	HasSynced    bool
	PlainLen     int
	SyncedLen    int
	Lang         string
	Instrumental bool
	Score        float64 // duration match score
}

// Problem tracks from testing
var problemTracks = []TestTrack{
	{"Radiohead", "Paranoid Android", "OK Computer", 363},
	{"Pink Floyd", "Comfortably Numb", "The Wall", 382},
	{"Led Zeppelin", "Stairway to Heaven", "Led Zeppelin IV", 482},
	{"The Beatles", "Come Together", "Abbey Road", 259},
	{"David Bowie", "Life on Mars?", "Hunky Dory", 334},
	{"Queen", "Bohemian Rhapsody", "A Night at the Opera", 354},
	{"Michael Jackson", "Billie Jean", "Thriller", 294},
	{"Nirvana", "Smells Like Teen Spirit", "Nevermind", 301},
	{"Stevie Wonder", "Superstition", "Talking Book", 244},
	{"Fleetwood Mac", "Gold Dust Woman", "Rumours", 309},
	{"The Doobie Brothers", "What a Fool Believes", "Minute by Minute", 231},
	{"Tom Petty", "Free Fallin'", "Full Moon Fever", 265},
	{"Bob Marley & The Wailers", "No Woman, No Cry", "Natty Dread", 257},
	{"Joni Mitchell", "Both Sides Now", "Clouds", 271},
	{"Eric Clapton", "Layla", "Layla and Other Assorted Jiams", 442},
	{"Aerosmith", "Dream On", "Aerosmith", 267},
	{"Bob Dylan", "Desolation Row", "Highway 61 Revisited", 437},
	{"The Rolling Stones", "Gimme Shelter", "Sticky Fingers", 272},
	{"The Who", "Baba O'Riley", "Who's Next", 310},
	{"Lynyrd Skynyrd", "Free Bird", "(Pronounced 'Lĕh-'nérd 'Skin-'nérd)", 548},
	// Add more...
}

func testSingleTrack(ctx context.Context, artist, track, album string, duration float64, verbose bool) {
	t := TestTrack{Artist: artist, Track: track, Album: album, Duration: duration}
	r := testTrack(ctx, t, verbose)
	printTrackResult(r, verbose)
}

func testTrack(ctx context.Context, t TestTrack, verbose bool) *TestResult {
	r := &TestResult{
		Track:    t.Track,
		Artist:   t.Artist,
		Album:    t.Album,
		Duration: t.Duration,
	}

	targetSec := int(t.Duration)

	// Step 1: Try original query with album
	r.Steps = append(r.Steps, fmt.Sprintf("Step 1: Try %q / %q with album", t.Artist, t.Track))
	results, err := searchLRCLib(ctx, t.Artist, t.Track, t.Album)
	if err != nil {
		r.Steps = append(r.Steps, fmt.Sprintf("  Error: %v", err))
		r.Error = err.Error()
	} else {
		r.Steps = append(r.Steps, fmt.Sprintf("  Got %d results", len(results)))
	}
	items := buildResultItems(results, targetSec)
	r.TopResults = items

	if verbose {
		printResults := items
		if len(printResults) > 5 {
			printResults = items[:5]
		}
		for _, item := range printResults {
			plainText := "plain"
			if !item.HasPlain {
				plainText = "NO plain"
			}
			syncedText := "synced"
			if !item.HasSynced {
				syncedText = "NO synced"
			}
			fmt.Printf("    [%d] %s - %s | dur=%d (+/-%d) | %s | %s | %s\n",
				item.Rank, item.ArtistName, item.TrackName,
				int(item.Duration), item.DurationDiff,
				plainText, syncedText, item.AlbumName)
		}
	}

	// Best match logic
	best := selectBest(results, targetSec, t.Album, verbose, r)

	if best == nil {
		// Step 2: Try cleaned without album
		r.Steps = append(r.Steps, "Step 2: Try cleaned query without album")
		cleanArtist := cleanArtistName(t.Artist)
		cleanTrack := cleanTrackName(t.Track)
		results2, err := searchLRCLibNoAlbum(ctx, cleanArtist, cleanTrack)
		if err == nil && len(results2) > 0 {
			items2 := buildResultItems(results2, targetSec)
			best = selectBestCleaned(results2, targetSec, t.Album, verbose, r, items2)
		}

		// Step 2b: If step 2 got results but none matched, try track-only search
		if best == nil && results2 != nil && len(results2) > 0 {
			r.Steps = append(r.Steps, "Step 2b: No good match from cleaned search, trying track-only")
		}
		if best == nil {
			// Step 3: Try track-only search (important for obscure albums)
			r.Steps = append(r.Steps, "Step 3: Try track-only search")
			results3, err := searchLRCLibTrackOnly(ctx, cleanTrack)
			if err == nil && len(results3) > 0 {
				items3 := buildResultItems(results3, targetSec)
				best = selectBestCleaned(results3, targetSec, t.Album, verbose, r, items3)
			}
		}

		if best == nil {
			// Step 4: Try looser matching on ALL results gathered
			r.Steps = append(r.Steps, "Step 4: Try looser duration matching")
			// Combine all results we've gathered
			allResults := append(append(results, results2...), results...)
			best = selectBestLooser(allResults, targetSec, verbose, r)
		}
	}

	if best != nil {
		r.HasPlain = best.PlainLyrics != ""
		r.HasSynced = best.SyncedLyrics != ""
		r.PlainLen = len(best.PlainLyrics)
		r.SyncedLen = len(best.SyncedLyrics)
		r.DurationAPI = best.Duration
		r.Lang = best.Lang
		r.Matched = fmt.Sprintf("matched with +%ds duration", abs(int(best.Duration)-targetSec))
	} else {
		r.Matched = "no match found"
	}

	return r
}

// selectBest implements 3-tier selection:
// Tier 1: Synced lyrics + duration ±2s (best for sync)
// Tier 2: Plain lyrics + matching album (any duration)
// Tier 3: Any plain lyrics (fallback)
func selectBest(results []LyricsResult, targetSec int, album string, verbose bool, r *TestResult) *LyricsResult {
	if len(results) == 0 {
		return nil
	}

	// === Tier 1: Synced + duration ±2s ===
	for _, res := range results {
		if res.SyncedLyrics == "" {
			continue
		}
		diff := abs(int(res.Duration) - targetSec)
		if diff <= 2 {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> TIER 1: Synced + exact duration match: %s (%ds, diff=%d)", res.TrackName, int(res.Duration), diff))
			return &res
		}
	}

	// === Tier 2: Plain lyrics + matching album (any duration) ===
	for _, res := range results {
		if res.PlainLyrics == "" {
			continue
		}
		if albumNamesMatch(res.AlbumName, album) {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> TIER 2: Plain + album match: %s album=%q", res.TrackName, res.AlbumName))
			return &res
		}
	}

	// === Tier 3: Any plain lyrics (fallback) ===
	for _, res := range results {
		if res.PlainLyrics != "" {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> TIER 3: Plain fallback: %s album=%q", res.TrackName, res.AlbumName))
			return &res
		}
	}

	return nil
}

// normalizeAlbumName removes brackets and normalizes for comparison
func normalizeAlbumName(album string) string {
	album = strings.ToLower(album)
	album = strings.ReplaceAll(album, "[", "")
	album = strings.ReplaceAll(album, "]", "")
	album = strings.ReplaceAll(album, "(", "")
	album = strings.ReplaceAll(album, ")", "")
	album = strings.ReplaceAll(album, "-", " ")
	album = strings.Join(strings.Fields(album), " ")
	return strings.TrimSpace(album)
}

// albumNamesMatch checks if two album names refer to the same album
func albumNamesMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	// Direct match
	if strings.EqualFold(a, b) {
		return true
	}
	// Normalized comparison
	na := normalizeAlbumName(a)
	nb := normalizeAlbumName(b)
	if na == nb {
		return true
	}
	// One contains the other
	if len(na) > 3 && len(nb) > 3 {
		if strings.Contains(na, nb) || strings.Contains(nb, na) {
			return true
		}
	}
	return false
}

func selectBestCleaned(results []LyricsResult, targetSec int, album string, verbose bool, r *TestResult, items []ResultItem) *LyricsResult {
	if len(results) == 0 {
		return nil
	}

	// Try first result if good enough
	best := selectBest(results, targetSec, album, verbose, r)
	if best != nil {
		return best
	}

	// Fallback to first with plain lyrics
	for _, res := range results {
		if res.PlainLyrics != "" {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> Fallback to first with lyrics: %s", res.TrackName))
			return &res
		}
	}

	return nil
}

func selectBestLooser(results []LyricsResult, targetSec int, verbose bool, r *TestResult) *LyricsResult {
	if len(results) == 0 {
		return nil
	}

	// Any plain lyrics
	for _, res := range results {
		if res.PlainLyrics != "" {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> Any with lyrics: %s (%ds)", res.TrackName, int(res.Duration)))
			return &res
		}
	}

	// Any synced lyrics
	for _, res := range results {
		if res.SyncedLyrics != "" {
			r.Steps = append(r.Steps, fmt.Sprintf("  -> Any with synced: %s", res.TrackName))
			return &res
		}
	}

	return nil
}

func searchLRCLib(ctx context.Context, artist, track, album string) ([]LyricsResult, error) {
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", track)
	if album != "" && album != "—" {
		params.Set("album_name", album)
	}

	u := fmt.Sprintf("https://lrclib.net/api/search?%s", params.Encode())

	resp, err := lrcGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []LyricsResult{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var results []LyricsResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

func searchLRCLibNoAlbum(ctx context.Context, artist, track string) ([]LyricsResult, error) {
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", track)

	u := fmt.Sprintf("https://lrclib.net/api/search?%s", params.Encode())

	resp, err := lrcGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []LyricsResult{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var results []LyricsResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

func searchLRCLibTrackOnly(ctx context.Context, track string) ([]LyricsResult, error) {
	params := url.Values{}
	params.Set("track_name", track)

	u := fmt.Sprintf("https://lrclib.net/api/search?%s", params.Encode())

	resp, err := lrcGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []LyricsResult{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var results []LyricsResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

type LyricsResult struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
	Lang         string  `json:"lang"`
	IsRC         bool    `json:"isRc"`
	SpotifyID    string  `json:"spotifyId"`
}

func buildResultItems(results []LyricsResult, targetSec int) []ResultItem {
	var items []ResultItem
	for i, r := range results {
		diff := abs(int(r.Duration) - targetSec)

		// Duration score (1.0 = exact, decreases with distance)
		score := 1.0
		if diff > 0 {
			score = 1.0 / (1.0 + float64(diff)/10.0)
		}
		// Bonus for plain lyrics
		if r.PlainLyrics != "" {
			score += 0.2
		}
		// Bonus for instrumental = bad
		if r.Instrumental {
			score -= 0.9
		}

		items = append(items, ResultItem{
			Rank:         i + 1,
			ArtistName:   r.ArtistName,
			TrackName:    r.TrackName,
			AlbumName:    r.AlbumName,
			Duration:     r.Duration,
			DurationDiff: diff,
			HasPlain:     r.PlainLyrics != "",
			HasSynced:    r.SyncedLyrics != "",
			PlainLen:     len(r.PlainLyrics),
			SyncedLen:    len(r.SyncedLyrics),
			Lang:         r.Lang,
			Instrumental: r.Instrumental,
			Score:        score,
		})
	}

	// Sort by score
	sort.Slice(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})

	return items
}

func cleanArtistName(artist string) string {
	cleaned := strings.ToLower(artist)
	// Remove "the " prefix
	cleaned = regexp.MustCompile(`(?i)^the\s+`).ReplaceAllString(cleaned, "")
	// Take first artist if multiple
	if idx := strings.Index(cleaned, ","); idx > 0 {
		cleaned = strings.TrimSpace(cleaned[:idx])
	}
	// Remove "featuring", "feat", "ft."
	cleaned = regexp.MustCompile(`(?i)\s*(?:feat\.?|featuring|ft\.?)\s*.*`).ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func cleanTrackName(track string) string {
	// Remove (feat. X), (w/ X), (with X), (X Version), (X Remix), etc.
	re := regexp.MustCompile(`\s*\([^)]*(?:feat\.|w\/|with|version|remix|remaster|edit)[^)]*\)`)
	cleaned := re.ReplaceAllString(track, "")
	return strings.TrimSpace(cleaned)
}

func fetchRPTracks(ctx context.Context, channelsStr string) []TestTrack {
	channels := strings.Split(channelsStr, ",")
	tracks := make(map[string]TestTrack)

	for _, chStr := range channels {
		chStr = strings.TrimSpace(chStr)
		var ch int
		fmt.Sscanf(chStr, "%d", &ch)

		rpAPI := api.NewRadioParadiseAPI(ch, 3)
		playlist, err := rpAPI.GetPlaylist(ctx)
		if err != nil {
			fmt.Printf("Failed to fetch playlist for channel %d: %v\n", ch, err)
			continue
		}

		fmt.Printf("Channel %d: %d songs\n", ch, len(playlist.Songs))
		for _, song := range playlist.Songs {
			artist, _ := song["artist"].(string)
			track, _ := song["title"].(string)
			album, _ := song["album"].(string)
			durMs, _ := song["duration"].(float64)
			durSec := durMs / 1000.0 // RP returns milliseconds

			key := artist + " - " + track
			if artist != "" && track != "" && artist != "Unknown Artist" {
				tracks[key] = TestTrack{
					Artist:   artist,
					Track:    track,
					Album:    album,
					Duration: durSec,
				}
			}
		}
	}

	var list []TestTrack
	for _, t := range tracks {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Artist < list[j].Artist
	})
	return list
}

func hasTrack(list []TestTrack, t TestTrack) bool {
	for _, existing := range list {
		if existing.Artist == t.Artist && existing.Track == t.Track {
			return true
		}
	}
	return false
}

func printTrackResult(r *TestResult, verbose bool) {
	fmt.Printf("\n=== %s - %s ===\n", r.Artist, r.Track)
	fmt.Printf("Album: %s\n", r.Album)
	fmt.Printf("Duration: %d seconds\n", int(r.Duration))
	fmt.Printf("\nResult:\n")
	fmt.Printf("  Has plain lyrics:  %s\n", boolToYN(r.HasPlain))
	fmt.Printf("  Has synced lyrics: %s\n", boolToYN(r.HasSynced))
	if r.HasPlain {
		fmt.Printf("  Plain length: %d chars\n", r.PlainLen)
	}
	if r.HasSynced {
		fmt.Printf("  Synced length: %d chars\n", r.SyncedLen)
	}
	fmt.Printf("  API Duration: %.0f seconds\n", r.DurationAPI)
	fmt.Printf("  Language: %s\n", r.Lang)
	fmt.Printf("  Match: %s\n", r.Matched)

	if verbose && len(r.TopResults) > 0 {
		fmt.Printf("\nTop %d results:\n", min(10, len(r.TopResults)))
		for i, item := range r.TopResults[:min(10, len(r.TopResults))] {
			plainStatus := "has lyrics"
			if !item.HasPlain {
				plainStatus = "NO lyrics"
			}
			fmt.Printf("  [%d] %s - %s\n", i+1, item.ArtistName, item.TrackName)
			fmt.Printf("      Album: %s | Duration: %.0fs (+/-%d) | %s | Lang: %s | Score: %.2f\n",
				item.AlbumName, item.Duration, item.DurationDiff, plainStatus, item.Lang, item.Score)
		}
	}

	if verbose && len(r.Steps) > 0 {
		fmt.Printf("\nDiagnostic steps:\n")
		for _, s := range r.Steps {
			fmt.Printf("  %s\n", s)
		}
	}
}

func printSummary(results map[string]*TestResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	total := len(results)
	var foundPlain, foundSynced, noLyrics int

	for _, r := range results {
		if r.HasPlain {
			foundPlain++
			if r.HasSynced {
				foundSynced++
			}
		} else {
			noLyrics++
		}
	}

	pct := func(n int) float64 { return float64(n) / float64(total) * 100 }

	fmt.Printf("\nTotal tracks: %d\n", total)
	fmt.Printf("Has plain lyrics:  %d (%.0f%%)\n", foundPlain, pct(foundPlain))
	fmt.Printf("Has synced lyrics: %d (%.0f%%)\n", foundSynced, pct(foundSynced))
	fmt.Printf("No lyrics found:   %d (%.0f%%)\n", noLyrics, pct(noLyrics))

	// Tracks with no synced (has plain but no synced)
	var noSynced []string
	for _, r := range results {
		if r.HasPlain && !r.HasSynced {
			lrcDur := r.DurationAPI
			if lrcDur == 0 {
				lrcDur = r.Duration
			}
			noSynced = append(noSynced, fmt.Sprintf("%s - %s - %s (%.0fs)", r.Artist, r.Track, r.Album, lrcDur))
		}
	}

	// Tracks with no lyrics
	var noLyricsList []string
	for _, r := range results {
		if !r.HasPlain {
			noLyricsList = append(noLyricsList, fmt.Sprintf("%s - %s", r.Artist, r.Track))
		}
	}

	if len(noSynced) > 0 {
		fmt.Printf("\nTracks with plain but NO synced (%d):\n", len(noSynced))
		for _, t := range noSynced {
			fmt.Printf("  - %s\n", t)
		}
	}

	if len(noLyricsList) > 0 {
		fmt.Printf("\nTracks with NO lyrics (%d):\n", len(noLyricsList))
		for _, t := range noLyricsList {
			fmt.Printf("  - %s\n", t)
		}
	}
}

func boolToYN(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
