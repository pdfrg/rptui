package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LRCLibClient provides access to the LRCLib lyrics API
type LRCLibClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewLRCLibClient creates a new LRCLib API client
func NewLRCLibClient() *LRCLibClient {
	return &LRCLibClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://lrclib.net/api",
		userAgent: "rptui-go/1.0",
	}
}

// LyricsResponse represents the LRCLib API response
type LyricsResponse struct {
	ID               int64   `json:"id"`
	Name             string  `json:"name"`
	TrackName        string  `json:"trackName"`
	ArtistName       string  `json:"artistName"`
	AlbumName        string  `json:"albumName"`
	Duration         float64 `json:"duration"`
	Instrumental     bool    `json:"instrumental"`
	PlainLyrics      string  `json:"plainLyrics"`
	SyncedLyrics     string  `json:"syncedLyrics"`
	Lang             string  `json:"lang"`
	IsRC             bool    `json:"isRc"`
	SpotifyID        string  `json:"spotifyId"`
}

// GetLyrics fetches lyrics by artist and track name
func (l *LRCLibClient) GetLyrics(ctx context.Context, artist, track string) (*LyricsResponse, error) {
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", track)

	u := fmt.Sprintf("%s/get?%s", l.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", l.userAgent)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch lyrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Lyrics not found
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result LyricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetLyricsByDuration fetches lyrics with duration filter for better matching
// Uses progressive fallback: original → cleaned → no album → looser matching
func (l *LRCLibClient) GetLyricsByDuration(ctx context.Context, artist, track, album string, duration float64) (*LyricsResponse, error) {
	// LRCLib duration is in SECONDS, not milliseconds!
	durationSec := int(duration / 1000)
	if durationSec <= 0 {
		durationSec = 180 // Default 3 minutes if invalid
	}

	logger.Printf("LRCLib: Looking for '%s' by %s from %s", track, artist, album)

	// === PHASE 1: Original inputs (63% work without cleaning) ===

	// Step 1: Original + Original + Original (with album)
	logger.Printf("LRCLib Step 1: Original query with album")
	if result, err := l.searchWithAlbum(ctx, artist, track, album); err == nil && result != nil {
		logger.Printf("✓ LRCLib matched: original + album")
		return l.selectBestByDuration(result, durationSec), nil
	}

	// Step 2: Original + Original (NO album) - filter by album score
	logger.Printf("LRCLib Step 2: Original query without album")
	if results, err := l.searchNoAlbum(ctx, artist, track); err == nil && len(results) > 0 {
		if best := l.filterByAlbum(results, album, 0.80); best != nil {
			logger.Printf("✓ LRCLib matched: original, no album (strict)")
			return best, nil
		}
	}

	// === PHASE 2: Cleaned inputs (for the 37% that failed) ===

	// Clean inputs
	cleanArtist := l.cleanArtist(artist)
	cleanTrack := l.cleanTitle(track)
	cleanAlbum := l.cleanAlbum(album)

	// Step 3: Cleaned + Cleaned + Cleaned (with album)
	logger.Printf("LRCLib Step 3: Cleaned query with album")
	if result, err := l.searchWithAlbum(ctx, cleanArtist, cleanTrack, cleanAlbum); err == nil && result != nil {
		logger.Printf("✓ LRCLib matched: cleaned + album")
		return l.selectBestByDuration(result, durationSec), nil
	}

	// Step 4: Cleaned + Cleaned (NO album) - filter by album score ≥0.80
	logger.Printf("LRCLib Step 4: Cleaned query without album (strict)")
	if results, err := l.searchNoAlbum(ctx, cleanArtist, cleanTrack); err == nil && len(results) > 0 {
		if best := l.filterByAlbum(results, album, 0.80); best != nil {
			logger.Printf("✓ LRCLib matched: cleaned, no album (strict)")
			return best, nil
		}
	}

	// Step 5: Cleaned + Cleaned (NO album) - filter by album score ≥0.50 (loose)
	logger.Printf("LRCLib Step 5: Cleaned query without album (loose)")
	if results, err := l.searchNoAlbum(ctx, cleanArtist, cleanTrack); err == nil && len(results) > 0 {
		if best := l.filterByAlbum(results, album, 0.50); best != nil {
			logger.Printf("✓ LRCLib matched: cleaned, no album (loose)")
			return best, nil
		}
	}

	logger.Printf("✗ LRCLib: No match found after all attempts")
	return nil, nil
}

// searchWithAlbum searches LRCLib with artist, track, and album
func (l *LRCLibClient) searchWithAlbum(ctx context.Context, artist, track, album string) ([]LyricsResponse, error) {
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", track)
	if album != "" && album != "—" {
		params.Set("album_name", album)
	}

	u := fmt.Sprintf("%s/search?%s", l.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", l.userAgent)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var results []LyricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

// searchNoAlbum searches LRCLib with only artist and track (no album)
func (l *LRCLibClient) searchNoAlbum(ctx context.Context, artist, track string) ([]LyricsResponse, error) {
	params := url.Values{}
	params.Set("artist_name", artist)
	params.Set("track_name", track)

	u := fmt.Sprintf("%s/search?%s", l.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", l.userAgent)

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var results []LyricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

// selectBestByDuration selects the best match from results based on duration
// For synced lyrics: ±2 seconds; for plain lyrics: ±30 seconds
func (l *LRCLibClient) selectBestByDuration(results []LyricsResponse, targetDuration int) *LyricsResponse {
	if len(results) == 0 {
		return nil
	}

	// If only one result, return it
	if len(results) == 1 {
		return &results[0]
	}

	// Find best match by duration
	var bestSynced *LyricsResponse
	var bestPlain *LyricsResponse
	bestSyncedDiff := 2  // ±2 seconds for synced
	bestPlainDiff := 30  // ±30 seconds for plain

	for _, r := range results {
		rDuration := int(r.Duration)
		diff := abs(rDuration - targetDuration)

		// For SYNCED lyrics: need close duration match (±2 seconds)
		if diff <= bestSyncedDiff {
			bestSyncedDiff = diff
			bestSynced = &r
		}

		// For PLAIN lyrics: looser duration match (±30 seconds)
		if diff <= bestPlainDiff {
			bestPlainDiff = diff
			bestPlain = &r
		}
	}

	// Prefer synced if available
	if bestSynced != nil {
		return bestSynced
	}
	return bestPlain
}

// filterByAlbum filters results by album name match score
// Returns the best match above minScore, or nil if none found
func (l *LRCLibClient) filterByAlbum(results []LyricsResponse, targetAlbum string, minScore float64) *LyricsResponse {
	var bestMatch *LyricsResponse
	bestScore := minScore

	for _, r := range results {
		score := l.albumMatchScore(r.AlbumName, targetAlbum)
		logger.Printf("  LRCLib album check: '%s' vs '%s' = %.2f", r.AlbumName, targetAlbum, score)
		if score >= bestScore {
			bestScore = score
			bestMatch = &r
		}
	}

	return bestMatch
}

// albumMatchScore returns 0.0-1.0 similarity between album names
func (l *LRCLibClient) albumMatchScore(lrclibAlbum, ourAlbum string) float64 {
	if lrclibAlbum == "" || ourAlbum == "" {
		return 0.0
	}

	// Exact match (case-insensitive)
	if strings.EqualFold(lrclibAlbum, ourAlbum) {
		return 1.0
	}

	// Normalize both
	lrclibNorm := l.normalizeAlbum(lrclibAlbum)
	ourNorm := l.normalizeAlbum(ourAlbum)

	if lrclibNorm == ourNorm {
		return 0.95
	}

	// Check if one contains the other
	if strings.Contains(lrclibNorm, ourNorm) || strings.Contains(ourNorm, lrclibNorm) {
		return 0.85
	}

	// Check word overlap
	lrclibWords := strings.Fields(lrclibNorm)
	ourWords := strings.Fields(ourNorm)

	if len(lrclibWords) == 0 || len(ourWords) == 0 {
		return 0.0
	}

	matched := 0
	for _, lw := range lrclibWords {
		for _, ow := range ourWords {
			if strings.EqualFold(lw, ow) {
				matched++
				break
			}
		}
	}

	// Return ratio of matched words
	score := float64(matched) / float64(max(len(lrclibWords), len(ourWords)))
	return score
}

// normalizeAlbum removes common album suffixes/variants for comparison
func (l *LRCLibClient) normalizeAlbum(album string) string {
	// Lowercase
	album = strings.ToLower(album)

	// Remove common suffixes
	suffixes := []string{
		" - single", " - ep", " - deluxe", " - remastered",
		" (single)", " (ep)", " (deluxe)", " (remastered)",
		" (explicit)", " (radio edit)", " (album version)",
	}
	for _, suffix := range suffixes {
		album = strings.TrimSuffix(album, suffix)
	}

	// Remove brackets and content
	replacements := []struct{ old, new string }{
		{"[", ""}, {"]", ""},
		{"(", ""}, {")", ""},
		{" - ", " "},
		{":", ""},
	}
	for _, r := range replacements {
		album = strings.ReplaceAll(album, r.old, r.new)
	}

	// Remove extra spaces
	album = strings.Join(strings.Fields(album), " ")

	return strings.TrimSpace(album)
}

// cleanTitle removes parenthetical content that confuses LRCLib matching
func (l *LRCLibClient) cleanTitle(title string) string {
	// Remove (feat. X), (w/ X), (with X), (X Version), (X Remix), etc.
	re := regexp.MustCompile(`\s*\([^)]*(?:feat\.|w\/|with|version|remix|remaster)[^)]*\)`)
	cleaned := re.ReplaceAllString(title, "")
	return strings.TrimSpace(cleaned)
}

// cleanAlbum removes version info that confuses matching
func (l *LRCLibClient) cleanAlbum(album string) string {
	// Remove [X Version], (X Edition), etc.
	re := regexp.MustCompile(`\s*\[[^\]]*(?:version|edition)[^\]]*\]`)
	cleaned := re.ReplaceAllString(album, "")
	// Also remove " - Single", " - EP"
	re = regexp.MustCompile(`\s*-\s*(?:Single|EP)\s*$`)
	cleaned = re.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

// cleanArtist handles multiple artist names
func (l *LRCLibClient) cleanArtist(artist string) string {
	// "Artist A, Artist B, C & D" → try "Artist A"
	if idx := strings.Index(artist, ","); idx > 0 {
		return strings.TrimSpace(artist[:idx])
	}
	return artist
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SearchLyrics searches for lyrics by query
func (l *LRCLibClient) SearchLyrics(ctx context.Context, query string) ([]LyricsResponse, error) {
	params := url.Values{}
	params.Set("q", query)

	u := fmt.Sprintf("%s/search?%s", l.baseURL, params.Encode())

	resp, err := l.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("failed to search lyrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result []LyricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// SyncedLyric represents a single line of synced lyrics
type SyncedLyric struct {
	Time    float64 // Time in seconds
	Content string  // Lyric text
}

// ParseSyncedLyrics parses LRC format synced lyrics
func ParseSyncedLyrics(syncedLyrics string) []SyncedLyric {
	if syncedLyrics == "" {
		return nil
	}

	var result []SyncedLyric
	lines := strings.Split(syncedLyrics, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse [mm:ss.xx] or [mm:ss] format
		if len(line) > 0 && line[0] == '[' {
			endIdx := strings.Index(line, "]")
			if endIdx == -1 {
				continue
			}

			timeStr := line[1:endIdx]
			content := strings.TrimSpace(line[endIdx+1:])

			// Parse time
			timeVal, err := parseLRCTime(timeStr)
			if err != nil {
				continue
			}

			result = append(result, SyncedLyric{
				Time:    timeVal,
				Content: content,
			})
		}
	}

	return result
}

// parseLRCTime parses LRC time format [mm:ss.xx]
func parseLRCTime(timeStr string) (float64, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format: %s", timeStr)
	}

	minutes, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}

	return minutes*60 + seconds, nil
}

// FindCurrentLine finds the current lyric line based on playback position
func FindCurrentLine(syncedLyrics []SyncedLyric, position float64) string {
	if len(syncedLyrics) == 0 {
		return ""
	}

	// Find the last line that starts before or at the current position
	for i := len(syncedLyrics) - 1; i >= 0; i-- {
		if syncedLyrics[i].Time <= position {
			return syncedLyrics[i].Content
		}
	}

	// If position is before first line, return first line
	if len(syncedLyrics) > 0 {
		return syncedLyrics[0].Content
	}

	return ""
}
