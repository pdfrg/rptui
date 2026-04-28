package main

import (
	"context"
	"encoding/json"
	"flag"
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

	"github.com/pdfrg/rptui/internal/api"
)

const userAgent = "rptui-test/1.0 (https://github.com/user/rptui; test@example.com)"

var flagDumpHTML bool

// Wikipedia API rate limiting
var (
	wikiHTTP     = &http.Client{Timeout: 15 * time.Second}
	lastWikiCall time.Time
	wikiMu       sync.Mutex
)

const minWikiDelay = 1 * time.Second // Wikipedia asks for ≤200 req/s but we're conservative

// wikiGet makes a rate-limited GET request to Wikipedia, handling 429 retries.
func wikiGet(ctx context.Context, urlStr string) (*http.Response, error) {
	wikiMu.Lock()
	// Enforce minimum delay between requests
	elapsed := time.Since(lastWikiCall)
	if elapsed < minWikiDelay {
		time.Sleep(minWikiDelay - elapsed)
	}
	lastWikiCall = time.Now()
	wikiMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := wikiHTTP.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle rate limiting
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Second // default backoff
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				retryAfter = time.Duration(secs) * time.Second
			}
		}
		log.Printf("  Rate limited (429), waiting %v", retryAfter)
		resp.Body.Close()
		time.Sleep(retryAfter)

		// Retry once
		req2, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return nil, err
		}
		req2.Header.Set("User-Agent", userAgent)
		return wikiHTTP.Do(req2)
	}

	return resp, nil
}

// Problem artists from fixes.txt
var problemArtists = []string{
	"Gone Gone Beyond",
	"The The",
	"Manassas",
	"Thievery Corporation",
	"Medeski Martin & Wood",
	"Cracker",
	"Tool",
	"Camera Obscura",
	"Blind Pilot",
	"Puscifer",
	"The Georgia Satellites",
	"Lucho Bermudez",
}

func main() {
	var (
		rpChannels   string
		testProblems bool
		verbose      bool
		artist       string
		discogOnly   bool
		dumpHTML     bool
	)
	flag.StringVar(&rpChannels, "channels", "", "RP channels to pull from (comma-separated, e.g. '0,1,3')")
	flag.BoolVar(&testProblems, "problems", false, "Test the known problem artists from fixes.txt")
	flag.BoolVar(&verbose, "v", false, "Verbose output with step-by-step diagnostics")
	flag.StringVar(&artist, "artist", "", "Test a single artist name")
	flag.BoolVar(&discogOnly, "discog", false, "Only test discography parsing (requires -artist)")
	flag.BoolVar(&dumpHTML, "dump-html", false, "Dump raw cleaned HTML for discography sections")
	flag.Parse()
	flagDumpHTML = dumpHTML

	ctx := context.Background()
	var artists []string

	switch {
	case artist != "":
		artists = []string{artist}
	case testProblems:
		artists = problemArtists
	case rpChannels != "":
		artists = fetchRPArtists(ctx, rpChannels)
	default:
		// Default: test problems + a sampling from RP
		artists = problemArtists
		fmt.Println("No mode specified. Testing problem artists + RP Main Mix sampling.")
		fmt.Println("Use -h for options.")
		rpArtists := fetchRPArtists(ctx, "0")
		// Add RP artists not already in problem list
		seen := make(map[string]bool)
		for _, a := range problemArtists {
			seen[a] = true
		}
		for _, a := range rpArtists {
			if !seen[a] {
				artists = append(artists, a)
			}
		}
	}

	fmt.Printf("Testing %d artists\n\n", len(artists))

	results := make(map[string]*TestResult)
	for i, a := range artists {
		fmt.Printf("[%d/%d] %s\n", i+1, len(artists), a)
		r := testArtist(ctx, a, verbose, discogOnly)
		results[a] = r
		fmt.Println()
	}

	printSummary(results)
}

type TestResult struct {
	Artist        string
	DirectHit     bool // got summary directly by name
	PageTitle     string
	PageURL       string
	Description   string
	HasSummary    bool
	SummaryLen    int
	HasThumb      bool
	ThumbURL      string
	Discography   string
	DiscogSection string // which section was found
	DiscogAlbums  int
	Error         string
	Steps         []string // diagnostic steps
}

func testArtist(ctx context.Context, artist string, verbose, discogOnly bool) *TestResult {
	r := &TestResult{Artist: artist}

	// Step 1: Try direct summary
	r.Steps = append(r.Steps, fmt.Sprintf("GET /page/summary/%s", strings.ReplaceAll(artist, " ", "_")))
	summary, err := getSummary(ctx, artist)
	if err != nil {
		r.Steps = append(r.Steps, fmt.Sprintf("  -> error: %v", err))
	} else if summary == nil {
		r.Steps = append(r.Steps, "  -> nil response")
	} else {
		r.Steps = append(r.Steps, fmt.Sprintf("  -> title=%q desc=%q", summary.Title, summary.Description))
		musician := isMusicianPage(summary)
		r.Steps = append(r.Steps, fmt.Sprintf("  -> isMusician=%v", musician))
		if musician {
			r.DirectHit = true
			r.PageTitle = summary.Title
			r.PageURL = summary.ContentUrls.Desktop.Page
			r.Description = summary.Description
			r.HasSummary = summary.Extract != ""
			r.SummaryLen = len(summary.Extract)
			r.HasThumb = summary.Thumbnail.Source != ""
			r.ThumbURL = summary.Thumbnail.Source
		}
	}

	// Step 2: If no direct hit, try search.
	// Also try search if direct hit has no discography section - there may be
	// a better match (e.g., "Sam Phillips (musician)" vs "Sam Phillips" producer)
	if !r.DirectHit || r.PageTitle != "" {
		searchQueries := []string{
			artist,
			fmt.Sprintf("%s musician", artist),
			fmt.Sprintf("%s singer", artist),
			fmt.Sprintf("%s band", artist),
			fmt.Sprintf("%s music artist", artist),
		}

		if !r.DirectHit {
			r.Steps = append(r.Steps, "Direct summary failed, trying search...")
		} else {
			r.Steps = append(r.Steps, "Direct hit found, checking if search has better match...")
		}

		allResults := make(map[string]bool)
		type scoredResult struct {
			title string
			score float64
		}
		var results []scoredResult

		for _, query := range searchQueries {
			r.Steps = append(r.Steps, fmt.Sprintf("  search: %q", query))
			sr, err := searchWikipedia(ctx, query)
			if err != nil {
				r.Steps = append(r.Steps, fmt.Sprintf("    -> error: %v", err))
				continue
			}
			r.Steps = append(r.Steps, fmt.Sprintf("    -> %d results", len(sr)))
			for _, item := range sr {
				if allResults[item.Title] {
					continue
				}
				allResults[item.Title] = true
				if strings.Contains(strings.ToLower(item.Title), "disambiguation") {
					continue
				}
				if strings.Contains(strings.ToLower(item.Title), "discography") {
					continue
				}
				score := similarityScore(item.Title, artist)
				if score > 0.9 {
					results = append(results, scoredResult{item.Title, score})
					if verbose {
						r.Steps = append(r.Steps, fmt.Sprintf("      %s score=%.2f", item.Title, score))
					}
				}
			}
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].score > results[j].score
		})

		r.Steps = append(r.Steps, fmt.Sprintf("  %d candidates after scoring", len(results)))

		// If we already have a direct hit, check if search candidates have a better
		// match with discography (e.g., "Sam Phillips (musician)" vs "Sam Phillips")
		needDiscogCheck := r.DirectHit

		for _, candidate := range results {
			// Skip if this is the same page as our direct hit
			if r.DirectHit && candidate.title == r.PageTitle {
				continue
			}

			r.Steps = append(r.Steps, fmt.Sprintf("  checking %q (score=%.2f)", candidate.title, candidate.score))
			s, err := getSummary(ctx, candidate.title)
			if err != nil {
				r.Steps = append(r.Steps, fmt.Sprintf("    -> error: %v", err))
				continue
			}
			if s == nil {
				r.Steps = append(r.Steps, "    -> nil")
				continue
			}
			r.Steps = append(r.Steps, fmt.Sprintf("    -> title=%q desc=%q", s.Title, s.Description))
			if !isMusicianPage(s) {
				r.Steps = append(r.Steps, "    -> not musician")
				continue
			}

			// If we have a direct hit but no discography yet, check if this candidate has one
			if needDiscogCheck {
				discog, section, count := fetchDiscographyDebug(ctx, s.Title)
				if count > 0 {
					r.Steps = append(r.Steps, fmt.Sprintf("    -> MATCH with discography (%d albums), replacing direct hit", count))
					r.PageTitle = s.Title
					r.PageURL = s.ContentUrls.Desktop.Page
					r.Description = s.Description
					r.HasSummary = s.Extract != ""
					r.SummaryLen = len(s.Extract)
					r.HasThumb = s.Thumbnail.Source != ""
					r.ThumbURL = s.Thumbnail.Source
					r.Discography = discog
					r.DiscogSection = section
					r.DiscogAlbums = count
					break
				}
				r.Steps = append(r.Steps, "    -> musician but no discography either")
			} else {
				// No direct hit - accept first musician match
				r.PageTitle = s.Title
				r.PageURL = s.ContentUrls.Desktop.Page
				r.Description = s.Description
				r.HasSummary = s.Extract != ""
				r.SummaryLen = len(s.Extract)
				r.HasThumb = s.Thumbnail.Source != ""
				r.ThumbURL = s.Thumbnail.Source
				r.Steps = append(r.Steps, "    -> MATCH")
				break
			}
		}
	}

	if r.PageTitle == "" {
		r.Error = "not found"
		r.Steps = append(r.Steps, "  -> no match found")
		if verbose {
			printResult(r, verbose)
		}
		return r
	}

	// Step 3: Discography (skip if already fetched during candidate comparison)
	if r.DiscogAlbums == 0 && (!discogOnly || artist != "") {
		r.Steps = append(r.Steps, fmt.Sprintf("Fetching discography for %q...", r.PageTitle))
		discog, section, albums := fetchDiscographyDebug(ctx, r.PageTitle)
		r.Discography = discog
		r.DiscogSection = section
		r.DiscogAlbums = albums
	}

	printResult(r, verbose)
	return r
}

func printResult(r *TestResult, verbose bool) {
	fmt.Printf("  Page: %s\n", r.PageTitle)
	if r.PageURL != "" {
		fmt.Printf("  URL:  %s\n", r.PageURL)
	}
	fmt.Printf("  Desc: %s\n", r.Description)
	fmt.Printf("  Summary: %s (%d chars)\n", boolToYN(r.HasSummary), r.SummaryLen)
	fmt.Printf("  Thumb:   %s", boolToYN(r.HasThumb))
	if r.HasThumb {
		fmt.Printf(" %s", r.ThumbURL)
	}
	fmt.Println()
	if r.DiscogSection != "" {
		fmt.Printf("  Discog section: %s\n", r.DiscogSection)
	}
	fmt.Printf("  Discography: %s (%d albums)\n", boolToYN(r.Discography != ""), r.DiscogAlbums)
	if r.Discography != "" {
		lines := strings.Split(r.Discography, "\n")
		show := lines
		if len(lines) > 5 && !verbose {
			show = lines[:5]
		}
		for _, line := range show {
			fmt.Printf("    %s\n", line)
		}
		if len(lines) > 5 && !verbose {
			fmt.Printf("    ... (%d more)\n", len(lines)-5)
		}
	}
	if r.Error != "" {
		fmt.Printf("  ERROR: %s\n", r.Error)
	}
	if verbose {
		fmt.Println("  Steps:")
		for _, s := range r.Steps {
			fmt.Printf("    %s\n", s)
		}
	}
}

func boolToYN(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// --- Wikipedia API calls (testbed versions) ---

type summaryResponse struct {
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

func getSummary(ctx context.Context, title string) (*summaryResponse, error) {
	wikiTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	u := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", wikiTitle)

	resp, err := wikiGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result summaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

type searchResult struct {
	Title   string
	Snippet string
}

func searchWikipedia(ctx context.Context, query string) ([]searchResult, error) {
	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json",
		url.QueryEscape(query),
	)

	resp, err := wikiGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Query struct {
			Search []struct {
				Title   string `json:"title"`
				Snippet string `json:"snippet"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var results []searchResult
	for _, s := range result.Query.Search {
		results = append(results, searchResult{s.Title, s.Snippet})
	}
	return results, nil
}

// fetchDiscographyDebug returns (album list, section name, album count)
func fetchDiscographyDebug(ctx context.Context, pageTitle string) (string, string, int) {
	// URL-encode the page title (handles &, parentheses, etc.)
	wikiTitle := url.QueryEscape(strings.ReplaceAll(pageTitle, " ", "_"))

	// Get sections
	u := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&prop=sections&format=json",
		wikiTitle,
	)

	resp, err := wikiGet(ctx, u)
	if err != nil {
		log.Printf("  discog sections error: %v", err)
		return "", "", 0
	}
	defer resp.Body.Close()

	var sectionsResult struct {
		Parse struct {
			Sections []struct {
				Index string `json:"index"`
				Line  string `json:"line"`
				Level string `json:"level"`
			} `json:"sections"`
		} `json:"parse"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sectionsResult); err != nil {
		log.Printf("  discog sections decode error: %v", err)
		return "", "", 0
	}

	// Find section - try multiple patterns
	// Priority: discography > studio album(s) > albums > cds
	// Skip "upcoming" or "future" sections
	var targetIndex, targetLine string
	for _, s := range sectionsResult.Parse.Sections {
		lineLower := strings.ToLower(s.Line)
		// Skip "upcoming" or "future" sections - they don't contain actual album listings
		if strings.Contains(lineLower, "upcoming") || strings.Contains(lineLower, "future") {
			continue
		}
		// Skip "compilation", "live album", "EP" subsections - we want the main discography
		if strings.Contains(lineLower, "compilation") || strings.Contains(lineLower, "live album") ||
			strings.Contains(lineLower, "EPs") || strings.Contains(lineLower, "singles") {
			continue
		}
		// Prefer discography sections
		if strings.Contains(lineLower, "discography") && targetIndex == "" {
			targetIndex = s.Index
			targetLine = s.Line
		}
		// "Studio album(s)" sections with actual listings
		if strings.Contains(lineLower, "studio album") && targetIndex == "" {
			targetIndex = s.Index
			targetLine = s.Line
		}
		// "Albums" as a fallback section name
		if lineLower == "albums" && targetIndex == "" {
			targetIndex = s.Index
			targetLine = s.Line
		}
		// "CDs" as a fallback section name
		if lineLower == "cds" && targetIndex == "" {
			targetIndex = s.Index
			targetLine = s.Line
		}
	}

	if targetIndex == "" {
		// Log available sections for debugging
		fmt.Printf("  No discog section found. Available sections:\n")
		for _, s := range sectionsResult.Parse.Sections {
			fmt.Printf("    [%s] %s (level %s)\n", s.Index, s.Line, s.Level)
		}
		return "", "", 0
	}

	fmt.Printf("  Found section: %s (index %s)\n", targetLine, targetIndex)

	// Fetch section content
	u = fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=parse&page=%s&section=%s&prop=text&format=json",
		wikiTitle,
		url.QueryEscape(targetIndex),
	)

	resp, err = wikiGet(ctx, u)
	if err != nil {
		log.Printf("  discog content error: %v", err)
		return "", targetLine, 0
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
		log.Printf("  discog content decode error: %v", err)
		return "", targetLine, 0
	}

	htmlContent := htmlResult.Parse.Text.Content

	// Clean HTML for display/debugging
	cleaned := cleanHTML(htmlContent)
	if flagDumpHTML {
		fmt.Printf("\n  === RAW CLEANED HTML ===\n")
		lines := strings.Split(cleaned, "\n")
		for i, line := range lines {
			fmt.Printf("  [%3d] %s\n", i+1, line)
		}
		fmt.Printf("  === END HTML (%d lines) ===\n\n", len(lines))
	}

	// Note if section links to a separate discography page (informational only)
	mainArticlePattern := regexp.MustCompile(`(?i)Main article:\s*(.+?discography)`)
	mainArticle := ""
	if ma := mainArticlePattern.FindStringSubmatch(cleaned); len(ma) >= 2 {
		mainArticle = ma[1]
	}

	// Try table parsing first
	albums := parseAlbumTable(htmlContent)
	source := "table"

	// Fallback to list parsing
	if len(albums) == 0 {
		albums = parseAlbumList(cleaned)
		source = "list"
	}

	if len(albums) == 0 {
		if mainArticle != "" {
			fmt.Printf("  No albums parsed; section links to: %s\n", mainArticle)
		} else {
			fmt.Printf("  No albums parsed from %s format\n", source)
		}
		return "", targetLine, 0
	}

	fmt.Printf("  Parsed %d albums from %s\n", len(albums), source)
	return strings.Join(albums, "\n"), targetLine, len(albums)
}

func parseAlbumTable(htmlContent string) []string {
	var albums []string

	// Find table rows
	rowRegex := regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	rows := rowRegex.FindAllStringSubmatch(htmlContent, -1)

	// Regex to strip citation markers like [11], [1]
	citationRegex := regexp.MustCompile(`\[\d+\]`)
	// Regex to strip HTML tags
	tagRegex := regexp.MustCompile(`(?s)<[^>]+>`)
	// Year detection
	yearRegex := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)

	for _, rowMatch := range rows {
		row := rowMatch[1]

		// Skip pure header rows (only <th>, no <td>)
		if strings.Contains(row, "<th") && !strings.Contains(row, "<td") {
			continue
		}

		// Extract ALL cells (both <th> and <td>) - Wikipedia uses <th> for row headers
		// Need (?s) because cell content often spans multiple lines
		cellRegex := regexp.MustCompile(`(?s)<(th|td)[^>]*>(.*?)</(?:th|td)>`)
		cells := cellRegex.FindAllStringSubmatch(row, -1)

		if len(cells) < 2 {
			continue
		}

		// Extract text from first two cells
		cell0Text := strings.TrimSpace(tagRegex.ReplaceAllString(cells[0][2], ""))
		cell0Text = html.UnescapeString(citationRegex.ReplaceAllString(cell0Text, ""))
		cell1Text := strings.TrimSpace(tagRegex.ReplaceAllString(cells[1][2], ""))
		cell1Text = html.UnescapeString(citationRegex.ReplaceAllString(cell1Text, ""))

		var albumName, year string

		// Detect column order: some tables have Year first, Album second
		// Others have Album first, Year second
		cell0IsYear := yearRegex.MatchString(cell0Text) && len(cell0Text) <= 6
		cell1IsYear := yearRegex.MatchString(cell1Text) && len(cell1Text) <= 6

		if cell0IsYear && !cell1IsYear {
			// Year | Album format (like The The)
			year = yearRegex.FindString(cell0Text)
			albumName = cell1Text
		} else {
			// Album | Year format (default)
			albumName = cell0Text
			// Find year in cells 1..3
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

func parseAlbumList(cleaned string) []string {
	var albums []string

	// First try: "Album Name (Year)" on single line
	albumPattern := regexp.MustCompile(`^(.+?)\s*\((\d{4})\)`)

	// Also try: "Album Name (Label, Year)" format common in jazz discographies
	albumLabelYearPattern := regexp.MustCompile(`^(.+?)\s*\([^)]*,\s*(\d{4})\)`)

	// Second: detect album name and year on consecutive lines
	yearOnlyPattern := regexp.MustCompile(`^\s*(\d{4})\s*$`)
	// Lines to skip as album names
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

		// Skip known header/junk lines
		if skipWords[lineLower] {
			continue
		}
		// Skip citation lines
		if strings.HasPrefix(line, "^ ") {
			prevLine = ""
			continue
		}
		// Skip lines that look like chart positions (single number or dash)
		if line == "—" || regexp.MustCompile(`^\d{1,3}$`).MatchString(line) {
			continue
		}

		// Try "Name (Year)" pattern first
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

		// Try "Name (Label, Year)" format (e.g., "Migration (CAM Jazz, 2007)")
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

		// Try "Year: Album Name" format (e.g., "1993: Les Djelys")
		yearColonPattern := regexp.MustCompile(`^\s*(\d{4})\s*:\s*(.+)$`)
		if m := yearColonPattern.FindStringSubmatch(line); len(m) >= 3 {
			year := m[1]
			albumName := strings.TrimSpace(m[2])
			// Strip trailing notes in parens like "(cassette only)"
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

		// Try "Year - Album Name" format (e.g., "1991 - Till We Have Faces")
		yearDashPattern := regexp.MustCompile(`^\s*(\d{4})\s*[-–]\s*(.+)$`)
		if m := yearDashPattern.FindStringSubmatch(line); len(m) >= 3 {
			year := m[1]
			albumName := strings.TrimSpace(m[2])
			// Strip trailing label/notes in parens
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

		// Check if this line is a bare year following an album name
		if yearMatch := yearOnlyPattern.FindStringSubmatch(line); len(yearMatch) >= 2 && prevLine != "" {
			year := yearMatch[1]
			// Verify previous line looks like an album name (not a number, not a skip word)
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

		// Remember this line as potential album name for next iteration
		prevLine = line
	}

	return albums
}

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

// --- Page classification ---

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

var albumIndicators = []string{
	"(album)", "(discography)", "soundtrack", "compilation",
	" studio album", "debut album", "EP) by", "album by",
	"live album", "greatest hits", "best of", "cover album", "tribute album",
}

func isMusicianPage(summary *summaryResponse) bool {
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
	if isAlbumPage(summary) {
		return false
	}
	// Reject song/soundtrack/EP pages - their descriptions contain "song by", "single by", etc.
	// These pages often mention bands in their extract text but aren't artist pages
	if strings.Contains(desc, "song by") ||
		strings.Contains(desc, "single by") ||
		strings.Contains(desc, "EP by") ||
		strings.Contains(desc, "soundtrack") {
		return false
	}
	for _, kw := range musicianKeywords {
		if strings.Contains(desc, kw) {
			return true
		}
	}

	// No extract fallback - words like "band", "group", "singer" appear in
	// unrelated contexts (e.g. "group of family names", "Solvay group",
	// "Irish rock band U2" in song descriptions) causing false positives.

	return false
}

func isAlbumPage(summary *summaryResponse) bool {
	if summary == nil {
		return false
	}
	desc := strings.ToLower(summary.Description)
	title := strings.ToLower(summary.Title)
	if strings.Contains(desc, "album by") ||
		strings.Contains(desc, "studio album") ||
		strings.Contains(desc, "live album") ||
		strings.Contains(desc, "compilation album") {
		return true
	}
	for _, indicator := range albumIndicators {
		if strings.Contains(title, indicator) {
			return true
		}
	}
	return false
}

// --- Similarity scoring ---

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

func normalizeArtistName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = regexp.MustCompile(`^the\s+`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`,?\s+the$`).ReplaceAllString(name, "")
	name = regexp.MustCompile(`[^\w\s]`).ReplaceAllString(name, "")
	return strings.TrimSpace(name)
}

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

// --- RP API song fetching ---

func fetchRPArtists(ctx context.Context, channelsStr string) []string {
	channels := strings.Split(channelsStr, ",")
	artists := make(map[string]bool)

	for _, chStr := range channels {
		chStr = strings.TrimSpace(chStr)
		var ch int
		_, _ = fmt.Sscanf(chStr, "%d", &ch)

		rpAPI := api.NewRadioParadiseAPI(ch, 3)
		playlist, err := rpAPI.GetPlaylist(ctx)
		if err != nil {
			log.Printf("Failed to fetch playlist for channel %d: %v", ch, err)
			continue
		}

		fmt.Printf("Channel %d: %d songs\n", ch, len(playlist.Songs))
		for _, song := range playlist.Songs {
			artist, _ := song["artist"].(string)
			if artist != "" && artist != "Unknown Artist" {
				artists[artist] = true
			}
		}
	}

	var list []string
	for a := range artists {
		list = append(list, a)
	}
	sort.Strings(list)
	return list
}

// --- Summary ---

func printSummary(results map[string]*TestResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	total := len(results)
	var found, directHit, hasSummary, hasThumb, hasDiscog int
	var noDiscogSection, noDiscogAlbums []string

	for _, r := range results {
		if r.PageTitle != "" {
			found++
			if r.DirectHit {
				directHit++
			}
			if r.HasSummary {
				hasSummary++
			}
			if r.HasThumb {
				hasThumb++
			}
			if r.Discography != "" {
				hasDiscog++
			} else if r.DiscogSection == "" {
				noDiscogSection = append(noDiscogSection, r.Artist)
			} else {
				noDiscogAlbums = append(noDiscogAlbums, r.Artist)
			}
		}
	}

	pct := func(n int) float64 { return float64(n) / float64(total) * 100 }

	fmt.Printf("\nTotal artists: %d\n", total)
	fmt.Printf("Found:         %d (%.0f%%)\n", found, pct(found))
	fmt.Printf("Direct hit:    %d (%.0f%%)\n", directHit, pct(directHit))
	fmt.Printf("Has summary:   %d (%.0f%%)\n", hasSummary, pct(hasSummary))
	fmt.Printf("Has thumb:     %d (%.0f%%)\n", hasThumb, pct(hasThumb))
	fmt.Printf("Has discog:    %d (%.0f%%)\n", hasDiscog, pct(hasDiscog))

	if len(noDiscogSection) > 0 {
		fmt.Printf("\nNo discography SECTION found (%d):\n", len(noDiscogSection))
		for _, a := range noDiscogSection {
			fmt.Printf("  - %s\n", a)
		}
	}
	if len(noDiscogAlbums) > 0 {
		fmt.Printf("\nDiscography section found but NO albums parsed (%d):\n", len(noDiscogAlbums))
		for _, a := range noDiscogAlbums {
			fmt.Printf("  - %s\n", a)
		}
	}

	// Artists not found
	var notFound []string
	for _, r := range results {
		if r.PageTitle == "" {
			notFound = append(notFound, r.Artist)
		}
	}
	if len(notFound) > 0 {
		fmt.Printf("\nNot found on Wikipedia (%d):\n", len(notFound))
		for _, a := range notFound {
			fmt.Printf("  - %s\n", a)
		}
	}
}
