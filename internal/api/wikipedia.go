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
	"strings"
	"time"

	"rptui-bubbletea/internal/loginit"
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
}

// NewWikipediaClient creates a new Wikipedia API client
func NewWikipediaClient() *WikipediaClient {
	return &WikipediaClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		userAgent: "rptui-go/1.0",
	}
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
		logger.Printf("Is musician: %v", w.isMusicianPage(directSummary))
	}

	if err == nil && directSummary != nil && w.isMusicianPage(directSummary) {
		logger.Printf("Found artist via direct summary: %s", directSummary.Title)

		// Fetch discography section
		discography := w.fetchDiscography(ctx, directSummary.Title)

		return &ArtistInfo{
			Summary:      cleanWikiText(directSummary.Extract),
			PageTitle:    directSummary.Title,
			PageURL:      directSummary.ContentUrls.Desktop.Page,
			ThumbnailURL: directSummary.Thumbnail.Source,
			Discography:  discography,
		}, nil
	}

	logger.Printf("Direct summary failed for %s, trying search...", artistName)

	// Search with various queries
	// Order: exact name first, then qualified searches
	// This ensures exact matches are found before qualified ones
	searchQueries := []string{
		artistName, // Try exact name FIRST
		fmt.Sprintf("%s musician", artistName),
		fmt.Sprintf("%s singer", artistName),
		fmt.Sprintf("%s band", artistName),
		fmt.Sprintf("%s music artist", artistName),
	}

	allResults := make(map[string]bool)
	var results []struct {
		title string
		score float64
	}

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

			// Skip disambiguation and discography pages
			if strings.Contains(strings.ToLower(r.Title), "disambiguation") {
				continue
			}
			if strings.Contains(strings.ToLower(r.Title), "discography") {
				continue
			}

			score := similarityScore(r.Title, artistName)
			logger.Printf("  Result: %s (score=%.2f)", r.Title, score)
			if score > 0.3 {
				results = append(results, struct {
					title string
					score float64
				}{r.Title, score})
			}
		}
	}

	logger.Printf("Total unique results: %d", len(results))

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Check top results for musician pages
	for _, r := range results {
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

		if w.isMusicianPage(summary) {
			logger.Printf("Found artist via search: %s (score=%.2f)", summary.Title, r.score)

			// Fetch discography section
			discography := w.fetchDiscography(ctx, summary.Title)

			return &ArtistInfo{
				Summary:      cleanWikiText(summary.Extract),
				PageTitle:    summary.Title,
				PageURL:      summary.ContentUrls.Desktop.Page,
				ThumbnailURL: summary.Thumbnail.Source,
				Discography:  discography,
			}, nil
		} else {
			logger.Printf("  Not a musician page")
		}
	}

	logger.Printf("Artist not found: %s", artistName)
	return nil, nil // Not found
}

// searchWikipedia searches Wikipedia API
func (w *WikipediaClient) searchWikipedia(ctx context.Context, query string) ([]struct {
	Title   string
	Snippet string
}, error) {
	// For search API, we need proper URL encoding
	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json",
		url.QueryEscape(query),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err := w.httpClient.Do(req)
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
	// Wikipedia URLs need special escaping: spaces become underscores, but parentheses stay literal
	// This matches Python's urllib.parse.quote behavior
	wikiTitle := strings.ReplaceAll(title, " ", "_")

	u := fmt.Sprintf(
		"https://en.wikipedia.org/api/rest_v1/page/summary/%s",
		wikiTitle,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
	// Remove bold/italic markers like ''' and ''
	text = strings.ReplaceAll(text, "'''", "")
	text = strings.ReplaceAll(text, "''", "")

	// Remove [[link|text]] style links, keep text
	linkRegex := regexp.MustCompile(`\[\[[^\]]*\|([^\]]*)\]\]`)
	text = linkRegex.ReplaceAllString(text, "$1")

	// Remove [[link]] style links, keep link name
	linkRegex2 := regexp.MustCompile(`\[\[([^\]]*)\]\]`)
	text = linkRegex2.ReplaceAllString(text, "$1")

	// Remove <ref>...</ref> tags
	refRegex := regexp.MustCompile(`<ref[^>]*>.*?</ref>`)
	text = refRegex.ReplaceAllString(text, "")

	// Remove {{...}} templates
	templateRegex := regexp.MustCompile(`\{\{[^}]*\}\}`)
	text = templateRegex.ReplaceAllString(text, "")

	// Decode HTML entities
	text = html.UnescapeString(text)

	// Clean up extra whitespace
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	return text
}

// fetchDiscography fetches studio albums from Wikipedia article
func (w *WikipediaClient) fetchDiscography(ctx context.Context, pageTitle string) string {
	// First get sections to find discography/studio albums section index
	// Wikipedia URLs need special escaping: spaces become underscores, but parentheses stay literal
	wikiTitle := strings.ReplaceAll(pageTitle, " ", "_")

	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&prop=sections&format=json",
		wikiTitle,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err := w.httpClient.Do(req)
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

	// Find studio albums or discography section
	var targetIndex string
	for _, s := range sectionsResult.Parse.Sections {
		lineLower := strings.ToLower(s.Line)
		if strings.Contains(lineLower, "studio album") {
			targetIndex = s.Index
			break
		}
		if targetIndex == "" && strings.Contains(lineLower, "discography") {
			targetIndex = s.Index
		}
	}

	if targetIndex == "" {
		logger.Printf("fetchDiscography: No discography/studio albums section found for %s", pageTitle)
		return ""
	}

	logger.Printf("fetchDiscography: Found section index %s for %s", targetIndex, pageTitle)

	// Fetch section HTML content
	u = fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&section=%s&prop=text&format=json",
		wikiTitle,
		url.QueryEscape(targetIndex),
	)

	req, err = http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err = w.httpClient.Do(req)
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

	// Parse HTML like Python version
	// Remove style blocks
	styleRegex := regexp.MustCompile(`<style[^>]*>.*?</style>`)
	htmlContent = styleRegex.ReplaceAllString(htmlContent, "")

	// Remove links but keep text
	linkRegex := regexp.MustCompile(`<a[^>]*>([^<]*)</a>`)
	htmlContent = linkRegex.ReplaceAllString(htmlContent, "$1")

	// Remove all other HTML tags
	tagRegex := regexp.MustCompile(`<[^>]+>`)
	htmlContent = tagRegex.ReplaceAllString(htmlContent, "")

	// Decode HTML entities
	htmlContent = html.UnescapeString(htmlContent)

	// Remove citation numbers
	citeRegex := regexp.MustCompile(`\[\d+\]`)
	htmlContent = citeRegex.ReplaceAllString(htmlContent, "")

	// Try to parse albums from tables FIRST (for Morcheeba, Big Sugar, etc.)
	albums := w.parseAlbumTable(htmlContent)

	// If no albums found in tables, try list format
	if len(albums) == 0 {
		albums = w.parseAlbumList(htmlContent)
	}

	// If still no albums, check if there's a link to a separate discography article
	if len(albums) == 0 {
		discogLink := w.extractDiscographyLink(htmlContent)
		if discogLink != "" {
			logger.Printf("fetchDiscography: Found separate discography article: %s", discogLink)
			return fmt.Sprintf("See: %s", discogLink)
		}
	}

	if len(albums) == 0 {
		logger.Printf("fetchDiscography: No albums parsed for %s", pageTitle)
		return ""
	}

	logger.Printf("fetchDiscography: Found %d albums for %s", len(albums), pageTitle)
	return strings.Join(albums, "\n")
}

// parseAlbumTable extracts albums from wikitable format
func (w *WikipediaClient) parseAlbumTable(htmlContent string) []string {
	var albums []string

	// Find table rows
	// Pattern: <tr>...<td>Album Name</td>...<td>Year</td>...</tr>
	rowRegex := regexp.MustCompile(`<tr[^>]*>(.*?)</tr>`)
	rows := rowRegex.FindAllStringSubmatch(htmlContent, -1)

	for _, rowMatch := range rows {
		row := rowMatch[1]

		// Skip header rows (contain <th> but no <td>)
		if strings.Contains(row, "<th") && !strings.Contains(row, "<td") {
			continue
		}

		// Extract all cell contents
		cellRegex := regexp.MustCompile(`<td[^>]*>(.*?)</td>`)
		cells := cellRegex.FindAllStringSubmatch(row, -1)

		if len(cells) < 2 {
			continue
		}

		// First cell should be album name (may contain <i> tags)
		albumCell := cells[0][1]
		// Remove all HTML tags from album name
		tagRegex := regexp.MustCompile(`<[^>]+>`)
		albumName := tagRegex.ReplaceAllString(albumCell, "")
		albumName = html.UnescapeString(albumName)
		albumName = strings.TrimSpace(albumName)

		// Look for year in other cells (usually 2nd or 3rd cell)
		var year string
		for i := 1; i < len(cells) && i < 4; i++ {
			cellContent := cells[i][1]
			// Look for 4-digit year
			yearRegex := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
			yearMatch := yearRegex.FindStringSubmatch(cellContent)
			if len(yearMatch) >= 2 {
				year = yearMatch[1]
				break
			}
		}

		// If we have both album name and year
		if albumName != "" && year != "" && len(albumName) < 100 {
			// Skip if album name looks like a header
			if strings.Contains(strings.ToLower(albumName), "title") ||
				strings.Contains(strings.ToLower(albumName), "album") && strings.Contains(strings.ToLower(albumName), "details") {
				continue
			}
			albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
		}
	}

	return albums
}

// parseAlbumList extracts albums from list format
func (w *WikipediaClient) parseAlbumList(htmlContent string) []string {
	var albums []string

	// Match album name followed by (YEAR), allowing trailing text like footnotes
	albumRegex := regexp.MustCompile(`^(.+?)\s*\((\d{4})\)`)
	lines := strings.Split(htmlContent, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(strings.ToLower(line), "studio album") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "discography") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "chart") {
			continue
		}

		matches := albumRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			albumName := strings.TrimSpace(matches[1])
			year := matches[2]
			// Clean up pipe in album names
			if idx := strings.Index(albumName, "|"); idx >= 0 {
				albumName = albumName[idx+1:]
			}
			albums = append(albums, fmt.Sprintf("%s (%s)", albumName, year))
		}

		if len(albums) >= 15 {
			break
		}
	}

	return albums
}

// extractDiscographyLink finds link to separate discography article
func (w *WikipediaClient) extractDiscographyLink(htmlContent string) string {
	// Look for "Main article: X discography" pattern
	// Pattern: <a href="/wiki/Artist_discography">Artist discography</a>
	linkRegex := regexp.MustCompile(`<a[^>]*href="/wiki/([^"]*discography[^"]*)"[^>]*>([^<]*discography[^<]*)</a>`)
	matches := linkRegex.FindStringSubmatch(htmlContent)

	if len(matches) >= 3 {
		// Return the link text (e.g., "Arctic Monkeys discography")
		return matches[2]
	}

	return ""
}

// isAlbumPage checks if Wikipedia page is about an album (not a musician)
func (w *WikipediaClient) isAlbumPage(summary *SummaryResponse) bool {
	if summary == nil {
		return false
	}

	desc := strings.ToLower(summary.Description)
	title := strings.ToLower(summary.Title)

	// Check description for album indicators
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

	// Check title for album indicators
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
	extract := strings.ToLower(summary.Extract)

	// MUST have a description
	if desc == "" {
		return false
	}

	// Skip disambiguation pages
	if strings.Contains(desc, "disambiguation") {
		return false
	}
	if strings.Contains(desc, "topics referred to by the same term") {
		return false
	}

	// Skip album pages (NEW - check this first)
	if w.isAlbumPage(summary) {
		return false
	}

	// STRICT: Must contain explicit musician keywords in DESCRIPTION
	// Not just in the extract (which can mention bands in passing)
	musicianDescKeywords := []string{
		"band", "singer", "musician", "rapper", "artist",
		"singer-songwriter", "songwriter", "dj", "producer",
		"pianist", "composer", "duo", "group", "vocalist",
		"rock band", "pop band", "metal band", "jazz",
		"american band", "british band", "american singer",
		"british singer", "american musician", "british musician",
	}

	for _, kw := range musicianDescKeywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}

	// Fallback: Check extract ONLY if description is very short
	// (some pages have minimal descriptions)
	if len(desc) < 30 {
		for _, kw := range musicianDescKeywords {
			if strings.Contains(extract, kw) {
				return true
			}
		}
	}

	return false
}

// similarityScore calculates similarity between Wikipedia title and artist name
func similarityScore(title, artist string) float64 {
	titleLower := strings.TrimSpace(strings.ToLower(title))
	artistLower := strings.TrimSpace(strings.ToLower(artist))

	// EXACT match (including disambiguators like "(band)")
	if titleLower == artistLower {
		return 1.0
	}

	// Title starts with artist name + disambiguator (e.g., "Ivy (band)" vs "Ivy")
	// This is the BEST kind of match after exact
	if strings.HasPrefix(titleLower, artistLower+" (") {
		// Bonus for common disambiguators
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

	// Artist starts with title (e.g., artist="The Beatles", title="Beatles")
	if strings.HasPrefix(artistLower, titleLower) {
		return 0.90
	}

	// Normalize and compare (removes "the", punctuation, etc.)
	titleNorm := normalizeArtistName(title)
	artistNorm := normalizeArtistName(artist)

	// Exact match after normalization
	if titleNorm == artistNorm {
		return 0.95
	}

	// Normalized prefix match - but penalize if title is MUCH longer
	if strings.HasPrefix(titleNorm, artistNorm) {
		// Calculate length ratio
		lengthRatio := float64(len(titleNorm)) / float64(len(artistNorm))

		// If title is more than 1.5x longer, it's probably a band name or different entity
		if lengthRatio > 1.5 {
			return 0.70 // Penalize
		}
		if lengthRatio > 1.2 {
			return 0.85 // Slight penalty
		}
		return 0.88
	}

	if strings.HasPrefix(artistNorm, titleNorm) {
		return 0.82
	}

	// Levenshtein distance for fuzzy matching
	maxLen := max(len(titleNorm), len(artistNorm))
	if maxLen == 0 {
		return 0.0
	}
	distance := levenshteinDistance(titleNorm, artistNorm)
	score := 1.0 - float64(distance)/float64(maxLen)

	if score > 0.3 {
		words := strings.Fields(titleNorm)

		// Check if artist name is contained in title words
		artistFound := false
		for _, word := range words {
			if word == artistNorm {
				score += 0.25
				artistFound = true
				// Bonus if artist name is the last word (common pattern)
				if words[len(words)-1] == artistNorm {
					score += 0.1
				}
				break
			}
		}

		// Check for partial word matches
		if !artistFound {
			for _, word := range words {
				if strings.HasPrefix(word, artistNorm) || strings.HasPrefix(artistNorm, word) {
					score += 0.08
				}
			}
		}

		// Bonus for explicit disambiguators
		if strings.Contains(titleLower, "(singer)") ||
			strings.Contains(titleLower, "(musician)") ||
			strings.Contains(titleLower, "(band)") ||
			strings.Contains(titleLower, "(rapper)") ||
			strings.Contains(titleLower, "(group)") {
			score += 0.08
		}

		// Small bonus for multi-word titles (more specific)
		score += float64(len(words)) * 0.02
	}

	return score
}

// normalizeArtistName normalizes artist name for comparison
func normalizeArtistName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	// Remove leading "the "
	name = regexp.MustCompile(`^the\s+`).ReplaceAllString(name, "")
	// Remove trailing ", the"
	name = regexp.MustCompile(`,?\s+the$`).ReplaceAllString(name, "")
	// Remove non-word characters
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

// FetchDiscography fetches discography from Wikipedia page
func (w *WikipediaClient) FetchDiscography(ctx context.Context, pageTitle string, maxAlbums int) (string, error) {
	// Get sections
	// Wikipedia URLs need special escaping: spaces become underscores, but parentheses stay literal
	wikiTitle := strings.ReplaceAll(pageTitle, " ", "_")

	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&prop=sections&format=json",
		wikiTitle,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var parseResult ParseResponse
	if err := json.NewDecoder(resp.Body).Decode(&parseResult); err != nil {
		return "", err
	}

	// Find studio album or discography section
	var targetIndex string
	for _, s := range parseResult.Parse.Sections {
		line := strings.ToLower(s.Line)
		if strings.Contains(line, "studio album") {
			targetIndex = s.Index
			break
		}
		if strings.Contains(line, "discography") && targetIndex == "" {
			targetIndex = s.Index
		}
	}

	if targetIndex == "" {
		return "", nil // No discography section found
	}

	// Get section content
	u = fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&section=%s&prop=text&format=json",
		wikiTitle,
		url.QueryEscape(targetIndex),
	)

	req, err = http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", w.userAgent)

	resp, err = w.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var sectionResult ParseResponse
	if err := json.NewDecoder(resp.Body).Decode(&sectionResult); err != nil {
		return "", err
	}

	htmlText := sectionResult.Parse.Text.Content

	// Clean HTML
	htmlText = cleanHTML(htmlText)

	// Extract albums
	albums := extractAlbums(htmlText, maxAlbums)

	if len(albums) > 0 {
		return strings.Join(albums, "\n"), nil
	}

	return "", nil
}

// cleanHTML removes HTML tags and cleans text
func cleanHTML(htmlText string) string {
	// Remove style blocks
	re := regexp.MustCompile(`<style[^>]*>.*?</style>`)
	htmlText = re.ReplaceAllString(htmlText, "")

	// Replace <br> with newlines
	htmlText = regexp.MustCompile(`<br\s*/?>`).ReplaceAllString(htmlText, "\n")

	// Replace links with text
	re = regexp.MustCompile(`<a[^>]*>([^<]*)</a>`)
	htmlText = re.ReplaceAllString(htmlText, "$1")

	// Remove italic tags
	htmlText = regexp.MustCompile(`<i>([^<]*)</i>`).ReplaceAllString(htmlText, "$1")
	htmlText = regexp.MustCompile(`</?i>`).ReplaceAllString(htmlText, "")

	// Remove all remaining tags
	htmlText = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(htmlText, "")

	// Unescape HTML entities
	htmlText = html.UnescapeString(htmlText)

	// Remove citation markers
	htmlText = regexp.MustCompile(`\[\d+\]`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\[nb\s*\d*\]`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\^\s*\w+.*`).ReplaceAllString(htmlText, "")
	htmlText = regexp.MustCompile(`\(edit\)`).ReplaceAllString(htmlText, "")

	// Normalize newlines
	htmlText = regexp.MustCompile(`\n\n+`).ReplaceAllString(htmlText, "\n")

	return htmlText
}

// extractAlbums extracts album lines from cleaned HTML
func extractAlbums(text string, maxAlbums int) []string {
	var albums []string
	lines := strings.Split(text, "\n")
	albumPattern := regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, "studio album") ||
			strings.Contains(lineLower, "discography") ||
			strings.Contains(lineLower, "chart") ||
			line == "Title" || line == "Album details" || line == "Peak" {
			continue
		}

		if albumPattern.MatchString(line) {
			albums = append(albums, line)
			if len(albums) >= maxAlbums {
				break
			}
		}
	}

	return albums
}
