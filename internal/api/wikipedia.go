package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pdfrg/rptui/internal/loginit"
)

// Logger for API
var logger *log.Logger

func init() {
	logger = loginit.InitLogger("[API] ")
}

// ArtistInfo represents Wikipedia artist information
type ArtistInfo struct {
	Summary      string
	PageTitle    string
	PageURL      string
	ThumbnailURL string
	Discography  string // Studio albums list
}

// WikipediaClient provides access to Wikipedia API
type WikipediaClient struct {
	httpClient *http.Client
	userAgent  string
	lastCall   time.Time
	mu         sync.Mutex
}

const wikiMinDelay = 1 * time.Second

// NewWikipediaClient creates a new Wikipedia API client
func NewWikipediaClient() *WikipediaClient {
	return &WikipediaClient{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		userAgent: "rptui-go/1.0 (https://github.com/user/rptui)",
	}
}

// wikiGet makes a rate-limited GET request, handling 429 retries.
func (w *WikipediaClient) wikiGet(ctx context.Context, urlStr string) (*http.Response, error) {
	w.mu.Lock()
	elapsed := time.Since(w.lastCall)
	if elapsed < wikiMinDelay {
		time.Sleep(wikiMinDelay - elapsed)
	}
	w.lastCall = time.Now()
	w.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		logger.Printf("Rate limited (429), waiting %v", retryAfter)
		resp.Body.Close()
		time.Sleep(retryAfter)

		req2, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return nil, err
		}
		req2.Header.Set("User-Agent", w.userAgent)
		return w.httpClient.Do(req2)
	}

	return resp, nil
}

// MusicianKeywords for identifying musician pages
var musicianKeywords = []string{
	"singer",
	"musician",
	"rapper",
	"band",
	"artist",
	"singer-songwriter",
	"songwriter",
	"dj",
	"music group",
	"rock band",
	"pop band",
	"heavy metal",
	"metal band",
	"metal solo",
	"hip hop",
	"r&b",
	"country singer",
	"jazz",
	"american musician",
	"british musician",
	"american band",
	"british band",
	"american singer",
	"british singer",
	"american rapper",
	"producer",
	"pianist",
	"composer",
	"duo",
	"group",
	"vocalist",
	"guitarist",
	"drummer",
	"bassist",
	"violinist",
	"saxophonist",
	"flautist",
	"cellist",
	"percussionist",
	"trumpeter",
	"harmonica",
}

// AlbumIndicators for filtering out album pages
var albumIndicators = []string{
	"(album)",
	"(discography)",
	"soundtrack",
	"compilation",
	" studio album",
	"debut album",
	"EP) by",
	"album by",
	"live album",
	"greatest hits",
	"best of",
	"cover album",
	"tribute album",
}

// SearchResponse represents Wikipedia search API response
type SearchResponse struct {
	Query struct {
		Search []struct {
			Title   string `json:"title"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"query"`
}

// SummaryResponse represents Wikipedia summary API response
type SummaryResponse struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Extract     string `json:"extract"`
	Thumbnail   struct {
		Source string `json:"source"`
	} `json:"thumbnail"`
	ContentUrls struct {
		Desktop struct {
			Page string `json:"page"`
		} `json:"desktop"`
	} `json:"content_urls"`
}

// ParseResponse represents Wikipedia parse API response
type ParseResponse struct {
	Parse struct {
		Sections []struct {
			Line  string `json:"line"`
			Index string `json:"index"`
		} `json:"sections"`
		Text struct {
			Content string `json:"*"`
		} `json:"text"`
	} `json:"parse"`
}

// FindArtist finds Wikipedia article for an artist
func (w *WikipediaClient) FindArtist(ctx context.Context, artistName string) (*ArtistInfo, error) {
	logger.Printf("=== FindArtist START: %s ===", artistName)

	// Try direct summary first (simplest approach)
	directSummary, err := w.getSummary(ctx, artistName)
	if err != nil {
		logger.Printf("Direct summary error: %v", err)
	}
	if err == nil && directSummary != nil {
		logger.Printf("Direct summary: Title=%s, Desc=%s", directSummary.Title, directSummary.Description)
	}

	var directResult *ArtistInfo
	if err == nil && directSummary != nil && w.isMusicianPage(directSummary) {
		logger.Printf("Found artist via direct summary: %s", directSummary.Title)
		directResult = &ArtistInfo{
			Summary:      cleanWikiText(directSummary.Extract),
			PageTitle:    directSummary.Title,
			PageURL:      directSummary.ContentUrls.Desktop.Page,
			ThumbnailURL: directSummary.Thumbnail.Source,
		}
	}

	// Always search - even if we have a direct hit, a disambiguated variant
	// (e.g., "Sam Phillips (musician)") might have discography when the
	// direct match doesn't.
	searchQueries := []string{
		artistName,
		fmt.Sprintf("%s musician", artistName),
		fmt.Sprintf("%s singer", artistName),
		fmt.Sprintf("%s band", artistName),
		fmt.Sprintf("%s music artist", artistName),
	}

	allResults := make(map[string]bool)
	type scoredResult struct {
		title string
		score float64
	}
	var results []scoredResult

	for _, query := range searchQueries {
		logger.Printf("Searching: %s", query)
		searchResults, err := w.searchWikipedia(ctx, query)
		if err != nil {
			logger.Printf("Search error for '%s': %v", query, err)
			continue
		}
		logger.Printf("Search '%s' returned %d results", query, len(searchResults))

		for _, r := range searchResults {
			if allResults[r.Title] {
				continue
			}
			allResults[r.Title] = true

			if strings.Contains(strings.ToLower(r.Title), "disambiguation") {
				continue
			}
			if strings.Contains(strings.ToLower(r.Title), "discography") {
				continue
			}

			score := similarityScore(r.Title, artistName)
			logger.Printf("  Result: %s (score=%.2f)", r.Title, score)
			if score > 0.9 {
				results = append(results, scoredResult{r.Title, score})
			}
		}
	}

	logger.Printf("Total unique results: %d", len(results))

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Check search results - if we have a direct hit, look for one with discography
	for _, r := range results {
		// Skip if same as direct hit
		if directResult != nil && r.title == directResult.PageTitle {
			continue
		}

		logger.Printf("Checking: %s (score=%.2f)", r.title, r.score)
		summary, err := w.getSummary(ctx, r.title)
		if err != nil {
			logger.Printf("  Summary error: %v", err)
			continue
		}
		if summary == nil {
			logger.Printf("  Summary is nil")
			continue
		}
		logger.Printf("  Summary: Title=%s, Desc=%s", summary.Title, summary.Description)

		if !w.isMusicianPage(summary) {
			logger.Printf("  Not a musician page")
			continue
		}

		// If we already have a direct hit, check if this candidate has discography
		if directResult != nil && directResult.Discography == "" {
			discog := w.fetchDiscography(ctx, summary.Title)
			if discog != "" {
				logger.Printf("  Replacing direct hit - found discography: %s", summary.Title)
				directResult.PageTitle = summary.Title
				directResult.PageURL = summary.ContentUrls.Desktop.Page
				directResult.Summary = cleanWikiText(summary.Extract)
				directResult.ThumbnailURL = summary.Thumbnail.Source
				directResult.Discography = discog
				return directResult, nil
			}
			logger.Printf("  No discography either, skipping")
			continue
		}

		// No direct hit - accept first musician match
		logger.Printf("Found artist via search: %s (score=%.2f)", summary.Title, r.score)
		discog := w.fetchDiscography(ctx, summary.Title)
		return &ArtistInfo{
			Summary:      cleanWikiText(summary.Extract),
			PageTitle:    summary.Title,
			PageURL:      summary.ContentUrls.Desktop.Page,
			ThumbnailURL: summary.Thumbnail.Source,
			Discography:  discog,
		}, nil
	}

	// If we had a direct hit but search didn't find anything better, use it
	if directResult != nil {
		if directResult.Discography == "" {
			directResult.Discography = w.fetchDiscography(ctx, directResult.PageTitle)
		}
		return directResult, nil
	}

	logger.Printf("Artist not found: %s", artistName)
	return nil, nil
}

// searchWikipedia searches Wikipedia API
func (w *WikipediaClient) searchWikipedia(ctx context.Context, query string) ([]struct {
	Title   string
	Snippet string
}, error) {
	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json",
		url.QueryEscape(query),
	)

	resp, err := w.wikiGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var results []struct {
		Title   string
		Snippet string
	}
	for _, s := range result.Query.Search {
		results = append(results, struct {
			Title   string
			Snippet string
		}{s.Title, s.Snippet})
	}

	return results, nil
}

// getSummary gets Wikipedia page summary
func (w *WikipediaClient) getSummary(ctx context.Context, title string) (*SummaryResponse, error) {
	wikiTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	u := fmt.Sprintf(
		"https://en.wikipedia.org/api/rest_v1/page/summary/%s",
		wikiTitle,
	)

	resp, err := w.wikiGet(ctx, u)
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

	var result SummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// cleanWikiText removes Wikipedia markup from text
func cleanWikiText(text string) string {
	text = strings.ReplaceAll(text, "'''", "")
	text = strings.ReplaceAll(text, "''", "")

	linkRegex := regexp.MustCompile(`\[\[[^\]]*\|([^\]]*)\]\]`)
	text = linkRegex.ReplaceAllString(text, "$1")

	linkRegex2 := regexp.MustCompile(`\[\[([^\]]*)\]\]`)
	text = linkRegex2.ReplaceAllString(text, "$1")

	refRegex := regexp.MustCompile(`<ref[^>]*>.*?</ref>`)
	text = refRegex.ReplaceAllString(text, "")

	templateRegex := regexp.MustCompile(`\{\{[^}]*\}\}`)
	text = templateRegex.ReplaceAllString(text, "")

	text = html.UnescapeString(text)
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return text
}

// fetchDiscography fetches studio albums from Wikipedia article
func (w *WikipediaClient) fetchDiscography(ctx context.Context, pageTitle string) string {
	wikiTitle := url.QueryEscape(strings.ReplaceAll(pageTitle, " ", "_"))

	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&prop=sections&format=json",
		wikiTitle,
	)

	resp, err := w.wikiGet(ctx, u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var sectionsResult struct {
		Parse struct {
			Sections []struct {
				Index string `json:"index"`
				Line  string `json:"line"`
			} `json:"sections"`
		} `json:"parse"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&sectionsResult); err != nil {
		return ""
	}

	// Find section - priority: discography > studio album(s) > albums > cds
	// Skip "upcoming", "future", "compilation", "live album", "EPs", "singles" subsections
	var targetIndex string
	for _, s := range sectionsResult.Parse.Sections {
		lineLower := strings.ToLower(s.Line)
		if strings.Contains(lineLower, "upcoming") || strings.Contains(lineLower, "future") {
			continue
		}
		if strings.Contains(lineLower, "compilation") || strings.Contains(lineLower, "live album") ||
			strings.Contains(lineLower, "EPs") || strings.Contains(lineLower, "singles") {
			continue
		}
		if strings.Contains(lineLower, "discography") && targetIndex == "" {
			targetIndex = s.Index
		}
		if strings.Contains(lineLower, "studio album") && targetIndex == "" {
			targetIndex = s.Index
		}
		if lineLower == "albums" && targetIndex == "" {
			targetIndex = s.Index
		}
		if lineLower == "cds" && targetIndex == "" {
			targetIndex = s.Index
		}
	}

	if targetIndex == "" {
		logger.Printf("fetchDiscography: No discography section found for %s", pageTitle)
		return ""
	}

	logger.Printf("fetchDiscography: Found section index %s for %s", targetIndex, pageTitle)

	u = fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&section=%s&prop=text&format=json",
		wikiTitle,
		url.QueryEscape(targetIndex),
	)

	resp, err = w.wikiGet(ctx, u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var htmlResult struct {
		Parse struct {
			Text struct {
				Content string `json:"*"`
			} `json:"text"`
		} `json:"parse"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&htmlResult); err != nil {
		return ""
	}

	htmlContent := htmlResult.Parse.Text.Content
	cleaned := cleanHTML(htmlContent)

	// Try table parsing first (on raw HTML for structure)
	albums := parseAlbumTable(htmlContent)

	// Fallback to list parsing (on cleaned text)
	if len(albums) == 0 {
		albums = parseAlbumList(cleaned)
	}

	if len(albums) == 0 {
		logger.Printf("fetchDiscography: No albums parsed for %s", pageTitle)
		return ""
	}

	logger.Printf("fetchDiscography: Found %d albums for %s", len(albums), pageTitle)
	return strings.Join(albums, "\n")
}

// parseAlbumTable extracts albums from wikitable format
func parseAlbumTable(htmlContent string) []string {
	var albums []string

	rowRegex := regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	rows := rowRegex.FindAllStringSubmatch(htmlContent, -1)

	citationRegex := regexp.MustCompile(`\[\d+\]`)
	tagRegex := regexp.MustCompile(`(?s)<[^>]+>`)
	yearRegex := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)

	for _, rowMatch := range rows {
		row := rowMatch[1]

		if strings.Contains(row, "<th") && !strings.Contains(row, "<td") {
			continue
		}

		// Extract ALL cells (both <th> and <td>) - Wikipedia uses <th> for row headers
		cellRegex := regexp.MustCompile(`(?s)<(th|td)[^>]*>(.*?)</(?:th|td)>`)
		cells := cellRegex.FindAllStringSubmatch(row, -1)

		if len(cells) < 2 {
			continue
		}

		cell0Text := strings.TrimSpace(tagRegex.ReplaceAllString(cells[0][2], ""))
		cell0Text = html.UnescapeString(citationRegex.ReplaceAllString(cell0Text, ""))
		cell1Text := strings.TrimSpace(tagRegex.ReplaceAllString(cells[1][2], ""))
		cell1Text = html.UnescapeString(citationRegex.ReplaceAllString(cell1Text, ""))

		var albumName, year string

		cell0IsYear := yearRegex.MatchString(cell0Text) && len(cell0Text) <= 6
		cell1IsYear := yearRegex.MatchString(cell1Text) && len(cell1Text) <= 6

		if cell0IsYear && !cell1IsYear {
			year = yearRegex.FindString(cell0Text)
			albumName = cell1Text
		} else {
			albumName = cell0Text
			for i := 1; i < len(cells) && i < 4; i++ {
				cellText := tagRegex.ReplaceAllString(cells[i][2], "")
				yearMatch := yearRegex.FindString(cellText)
				if yearMatch != "" {
					year = yearMatch
					break
				}
			}
		}

		if albumName != "" && year != "" && len(albumName) < 100 {
			lower := strings.ToLower(albumName)
			if strings.Contains(lower, "title") || strings.Contains(lower, "album details") {
				continue
			}
			albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
		}
	}

	return albums
}

// parseAlbumList extracts albums from list format
func parseAlbumList(cleaned string) []string {
	var albums []string

	albumPattern := regexp.MustCompile(`^(.+?)\s*\((\d{4})\)`)
	albumLabelYearPattern := regexp.MustCompile(`^(.+?)\s*\([^)]*,\s*(\d{4})\)`)
	yearOnlyPattern := regexp.MustCompile(`^\s*(\d{4})\s*$`)
	citationRegex := regexp.MustCompile(`\[\d+\]`)

	skipWords := map[string]bool{
		"title": true, "album details": true, "peak": true,
		"studio album": true, "studio albums": true, "discography": true,
		"chart": true, "released": true, "label": true,
	}

	lines := strings.Split(cleaned, "\n")
	var prevLine string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineLower := strings.ToLower(line)

		if skipWords[lineLower] {
			continue
		}
		if strings.HasPrefix(line, "^ ") {
			prevLine = ""
			continue
		}
		if line == "—" || regexp.MustCompile(`^\d{1,3}$`).MatchString(line) {
			continue
		}

		// Strip citation markers
		line = citationRegex.ReplaceAllString(line, "")

		// Try "Name (Year)" pattern
		matches := albumPattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			albumName := strings.TrimSpace(matches[1])
			year := matches[2]
			if idx := strings.Index(albumName, "|"); idx >= 0 {
				albumName = albumName[idx+1:]
			}
			albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
			prevLine = ""
			if len(albums) >= 20 {
				break
			}
			continue
		}

		// Try "Name (Label, Year)" format
		matches = albumLabelYearPattern.FindStringSubmatch(line)
		if len(matches) >= 3 {
			albumName := strings.TrimSpace(matches[1])
			year := matches[2]
			albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
			prevLine = ""
			if len(albums) >= 20 {
				break
			}
			continue
		}

		// Try "Year: Album Name" format
		yearColonPattern := regexp.MustCompile(`^\s*(\d{4})\s*:\s*(.+)$`)
		if m := yearColonPattern.FindStringSubmatch(line); len(m) >= 3 {
			year := m[1]
			albumName := strings.TrimSpace(m[2])
			if idx := strings.Index(albumName, "("); idx > 0 {
				albumName = strings.TrimSpace(albumName[:idx])
			}
			if albumName != "" && len(albumName) < 100 {
				albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
				prevLine = ""
				if len(albums) >= 20 {
					break
				}
				continue
			}
		}

		// Try "Year - Album Name" format
		yearDashPattern := regexp.MustCompile(`^\s*(\d{4})\s*[-–]\s*(.+)$`)
		if m := yearDashPattern.FindStringSubmatch(line); len(m) >= 3 {
			year := m[1]
			albumName := strings.TrimSpace(m[2])
			if idx := strings.Index(albumName, "("); idx > 0 {
				albumName = strings.TrimSpace(albumName[:idx])
			}
			if albumName != "" && len(albumName) < 100 {
				albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
				prevLine = ""
				if len(albums) >= 20 {
					break
				}
				continue
			}
		}

		// Check if bare year follows an album name
		if yearMatch := yearOnlyPattern.FindStringSubmatch(line); len(yearMatch) >= 2 && prevLine != "" {
			year := yearMatch[1]
			prevLower := strings.ToLower(prevLine)
			if !skipWords[prevLower] && !regexp.MustCompile(`^\d+$`).MatchString(prevLine) && prevLine != "—" {
				albums = append(albums, fmt.Sprintf("%s (%s)", prevLine, year))
				prevLine = ""
				if len(albums) >= 20 {
					break
				}
				continue
			}
		}

		prevLine = line
	}

	return albums
}

// cleanHTML removes HTML tags and cleans text
func cleanHTML(htmlText string) string {
	re := regexp.MustCompile(`<style[^>]*>.*?</style>`)
	htmlText = re.ReplaceAllString(htmlText, "")

	htmlText = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(htmlText, "\n")

	re = regexp.MustCompile(`<a[^>]*>([^<]*)</a>`)
	htmlText = re.ReplaceAllString(htmlText, "$1")

	htmlText = regexp.MustCompile(`<i>([^<]*)</i>`).ReplaceAllString(htmlText, "$1")
	htmlText = regexp.MustCompile(`</?i>`).ReplaceAllString(htmlText, "")

	htmlText = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(htmlText, "")

	htmlText = html.UnescapeString(htmlText)

	htmlText = regexp.MustCompile(`\[\d+\]`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\[nb\s*\d*\]`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\^\s*\w+.*`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\(edit\)`).ReplaceAllString(htmlText, "")

	htmlText = regexp.MustCompile(`\n\n+`).ReplaceAllString(htmlText, "\n")

	return htmlText
}

// isAlbumPage checks if Wikipedia page is about an album (not a musician)
func (w *WikipediaClient) isAlbumPage(summary *SummaryResponse) bool {
	if summary == nil {
		return false
	}

	desc := strings.ToLower(summary.Description)
	title := strings.ToLower(summary.Title)

	if strings.Contains(desc, "album by") {
		return true
	}
	if strings.Contains(desc, "studio album") {
		return true
	}
	if strings.Contains(desc, "live album") {
		return true
	}
	if strings.Contains(desc, "compilation album") {
		return true
	}
	if strings.Contains(desc, "EP by") {
		return true
	}

	for _, indicator := range albumIndicators {
		if strings.Contains(title, indicator) {
			return true
		}
	}

	return false
}

// isMusicianPage checks if Wikipedia page is about a musician
func (w *WikipediaClient) isMusicianPage(summary *SummaryResponse) bool {
	if summary == nil {
		return false
	}

	desc := strings.ToLower(summary.Description)

	if desc == "" {
		return false
	}
	if strings.Contains(desc, "disambiguation") {
		return false
	}
	if strings.Contains(desc, "topics referred to by the same term") {
		return false
	}
	if w.isAlbumPage(summary) {
		return false
	}
	// Reject song/soundtrack pages
	if strings.Contains(desc, "song by") ||
		strings.Contains(desc, "single by") ||
		strings.Contains(desc, "soundtrack") {
		return false
	}

	for _, kw := range musicianKeywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}

	// No extract fallback - words like "band", "group", "singer" appear
	// in unrelated contexts causing false positives.

	return false
}

// similarityScore calculates similarity between Wikipedia title and artist name
func similarityScore(title, artist string) float64 {
	titleLower := strings.TrimSpace(strings.ToLower(title))
	artistLower := strings.TrimSpace(strings.ToLower(artist))

	if titleLower == artistLower {
		return 1.0
	}

	if strings.HasPrefix(titleLower, artistLower+" (") {
		if strings.Contains(titleLower, "(band)") ||
			strings.Contains(titleLower, "(musician)") ||
			strings.Contains(titleLower, "(singer)") ||
			strings.Contains(titleLower, "(rapper)") ||
			strings.Contains(titleLower, "(guitarist)") ||
			strings.Contains(titleLower, "(group)") {
			return 0.99
		}
		return 0.95
	}

	if strings.HasPrefix(artistLower, titleLower) {
		return 0.90
	}

	titleNorm := normalizeArtistName(title)
	artistNorm := normalizeArtistName(artist)

	if titleNorm == artistNorm {
		return 0.95
	}

	if strings.HasPrefix(titleNorm, artistNorm) {
		lengthRatio := float64(len(titleNorm)) / float64(len(artistNorm))
		if lengthRatio > 1.5 {
			return 0.70
		}
		if lengthRatio > 1.2 {
			return 0.85
		}
		return 0.88
	}

	if strings.HasPrefix(artistNorm, titleNorm) {
		return 0.82
	}

	maxLen := max(len(titleNorm), len(artistNorm))
	if maxLen == 0 {
		return 0.0
	}
	distance := levenshteinDistance(titleNorm, artistNorm)
	score := 1.0 - float64(distance)/float64(maxLen)

	if score > 0.5 {
		words := strings.Fields(titleNorm)

		artistFound := false
		for _, word := range words {
			if word == artistNorm {
				score += 0.25
				artistFound = true
				if words[len(words)-1] == artistNorm {
					score += 0.1
				}
				break
			}
		}

		if !artistFound {
			for _, word := range words {
				if strings.HasPrefix(word, artistNorm) || strings.HasPrefix(artistNorm, word) {
					score += 0.08
				}
			}
		}

		if strings.Contains(titleLower, "(singer)") ||
			strings.Contains(titleLower, "(musician)") ||
			strings.Contains(titleLower, "(band)") ||
			strings.Contains(titleLower, "(rapper)") ||
			strings.Contains(titleLower, "(group)") {
			score += 0.08
		}

		score += float64(len(words)) * 0.02
	}

	return score
}

// normalizeArtistName normalizes artist name for comparison
func normalizeArtistName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`^the\s+`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`,?\s+the$`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

// levenshteinDistance calculates Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) < len(s2) {
		return levenshteinDistance(s2, s1)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	previousRow := make([]int, len(s2)+1)
	for i := range previousRow {
		previousRow[i] = i
	}

	for i, c1 := range s1 {
		currentRow := []int{i + 1}
		for j, c2 := range s2 {
			insertions := previousRow[j+1] + 1
			deletions := currentRow[j] + 1
			substitutions := previousRow[j]
			if c1 != c2 {
				substitutions++
			}
			currentRow = append(currentRow, min(insertions, deletions, substitutions))
		}
		previousRow = currentRow
	}

	return previousRow[len(previousRow)-1]
}
