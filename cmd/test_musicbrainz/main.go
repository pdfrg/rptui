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
	"strings"
	"time"

	"rptui-bubbletea/internal/api"
)

const userAgent = "rptui-test/1.0"

var httpClient = &http.Client{Timeout: 15 * time.Second}

var lastMBCall time.Time

func mbRateLimit() {
	if elapsed := time.Since(lastMBCall); elapsed < 1*time.Second {
		time.Sleep(1*time.Second - elapsed)
	}
	lastMBCall = time.Now()
}

type MBSearchResult struct {
	Name     string
	MBID     string
	Score    int
	Type     string
	Disambig string
}

type AlbumResult struct {
	Title string
	Year  string
}

type ArtistResult struct {
	RPArtist      string
	RPAlbums      []string // unique album names from RP for this artist
	SearchResults []MBSearchResult
	Pass          string // which pass matched: "exact", "normalized", "forward-contains", "first-person-group", "fallback", "none"
	MatchedName   string
	MatchedMBID   string
	Albums        []AlbumResult // MB albums
	TotalGroups   int
	Error         string
	// Debug fields
	QueryNorm   string // normalized form of the query
	MatchedNorm string // normalized form of the matched name
	IsMismatch  bool
	IsCollab    bool   // RP artist name contains comma (multi-artist listing)
	AlbumMatch  string // which RP album matched an MB album, or ""
	BadMatch    bool   // true if mismatch + no album match + not a collab
}

func main() {
	var (
		rpChannels string
		artist     string
		album      string
	)
	flag.StringVar(&rpChannels, "channels", "0,1,2,3,5", "RP channels to pull from (comma-separated)")
	flag.StringVar(&artist, "artist", "", "Test a single artist name")
	flag.StringVar(&album, "album", "", "RP album name for single-artist validation test")
	flag.Parse()

	ctx := context.Background()
	var results []ArtistResult

	if artist != "" {
		rpAlbums := []string{}
		if album != "" {
			rpAlbums = []string{album}
		}
		r := testArtist(ctx, artist, rpAlbums)
		results = append(results, r)
		printSingleResult(r)
		printSummary(results)
		return
	}

	// Fetch artists + albums from RP
	artistAlbums := fetchRPArtists(ctx, rpChannels)

	var artistList []string
	for a := range artistAlbums {
		artistList = append(artistList, a)
	}
	sort.Strings(artistList)

	fmt.Printf("Testing %d artists with MusicBrainz\n\n", len(artistList))

	for i, a := range artistList {
		albums := artistAlbums[a]
		fmt.Printf("[%d/%d] %s", i+1, len(artistList), a)
		r := testArtist(ctx, a, albums)
		results = append(results, r)
		if r.BadMatch {
			fmt.Printf("\n  ** BAD MATCH -> %s (pass: %s) — no album overlap\n", r.MatchedName, r.Pass)
		} else if r.IsMismatch && r.AlbumMatch != "" {
			fmt.Printf("\n  -> %s (album match: %q, pass: %s)\n", r.MatchedName, r.AlbumMatch, r.Pass)
		} else if r.IsMismatch && r.IsCollab {
			fmt.Printf("\n  -> %s (collab, pass: %s)\n", r.MatchedName, r.Pass)
		} else if r.IsMismatch {
			fmt.Printf("\n  -> %s (mismatch, pass: %s)\n", r.MatchedName, r.Pass)
		} else if r.Error != "" {
			fmt.Printf("\n  ** ERROR: %s\n", r.Error)
		} else {
			fmt.Printf(" -> %s\n", r.MatchedName)
		}
		fmt.Println()
	}

	printSummary(results)
}

func printSingleResult(r ArtistResult) {
	fmt.Printf("\nRP artist:   %s\n", r.RPArtist)
	fmt.Printf("RP albums:   %v\n", r.RPAlbums)
	fmt.Printf("Is collab:   %v\n", r.IsCollab)
	if r.Error != "" {
		fmt.Printf("ERROR: %s\n", r.Error)
		return
	}
	fmt.Printf("Matched:     %s (MBID: %s)\n", r.MatchedName, r.MatchedMBID)
	fmt.Printf("Pass:        %s\n", r.Pass)
	fmt.Printf("Mismatch:    %v\n", r.IsMismatch)
	fmt.Printf("Album match: %q\n", r.AlbumMatch)
	fmt.Printf("Bad match:   %v\n", r.BadMatch)
	fmt.Printf("MB albums:   %d\n", len(r.Albums))
	for _, a := range r.Albums {
		fmt.Printf("  - %s (%s)\n", a.Title, a.Year)
	}
}

func testArtist(ctx context.Context, rpArtist string, rpAlbums []string) ArtistResult {
	r := ArtistResult{RPArtist: rpArtist, RPAlbums: rpAlbums}
	r.IsCollab = strings.Contains(rpArtist, ",")

	// Step 1: Search for artist
	searchResult, err := searchMB(ctx, fmt.Sprintf("artistphrase:%s", rpArtist))
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if len(searchResult) == 0 {
		searchResult, err = searchMB(ctx, fmt.Sprintf("artist:\"%s\"", rpArtist))
		if err != nil {
			r.Error = err.Error()
			return r
		}
	} else {
		// artistphrase returned results — check if any is an exact match
		hasExact := false
		for _, a := range searchResult {
			if strings.EqualFold(a.Name, rpArtist) {
				hasExact = true
				break
			}
		}
		if !hasExact {
			// Also try quoted search (handles edge cases like "The The")
			quoted, err2 := searchMB(ctx, fmt.Sprintf("artist:\"%s\"", rpArtist))
			if err2 == nil && len(quoted) > 0 {
				// Prepend quoted results so exact match pass finds them first
				searchResult = append(quoted, searchResult...)
			}
		}
	}
	if len(searchResult) == 0 {
		r.Error = "no artist found in search"
		return r
	}

	for _, a := range searchResult {
		r.SearchResults = append(r.SearchResults, MBSearchResult{
			Name:     a.Name,
			MBID:     a.ID,
			Score:    a.Score,
			Type:     a.Type,
			Disambig: a.Disambiguation,
		})
	}

	rpLower := strings.ToLower(rpArtist)
	rpNorm := normalizeForCompare(rpLower)
	r.QueryNorm = rpNorm

	setMatch := func(name, id, pass string) {
		r.MatchedName = name
		r.MatchedMBID = id
		r.Pass = pass
		r.MatchedNorm = normalizeForCompare(strings.ToLower(name))
		r.IsMismatch = !strings.EqualFold(name, rpArtist)
	}

	// Pass 1: exact match
	for _, a := range searchResult {
		if strings.ToLower(a.Name) == rpLower {
			setMatch(a.Name, a.ID, "exact")
			r.Albums, r.TotalGroups, r.Error = fetchAlbums(ctx, a.ID)
			return r
		}
	}
	// Pass 2: normalized match
	for _, a := range searchResult {
		if normalizeForCompare(strings.ToLower(a.Name)) == rpNorm {
			setMatch(a.Name, a.ID, "normalized")
			r.Albums, r.TotalGroups, r.Error = fetchAlbums(ctx, a.ID)
			r.validateAlbumMatch()
			return r
		}
	}
	// Pass 3: forward contains
	for _, a := range searchResult {
		if strings.Contains(strings.ToLower(a.Name), rpLower) {
			setMatch(a.Name, a.ID, "forward-contains")
			r.Albums, r.TotalGroups, r.Error = fetchAlbums(ctx, a.ID)
			r.validateAlbumMatch()
			return r
		}
	}
	// Pass 4: first Person/Group
	for _, a := range searchResult {
		if a.Type == "Person" || a.Type == "Group" {
			setMatch(a.Name, a.ID, "first-person-group")
			r.Albums, r.TotalGroups, r.Error = fetchAlbums(ctx, a.ID)
			r.validateAlbumMatch()
			return r
		}
	}
	// Fallback
	setMatch(searchResult[0].Name, searchResult[0].ID, "fallback")
	r.Albums, r.TotalGroups, r.Error = fetchAlbums(ctx, searchResult[0].ID)
	r.validateAlbumMatch()
	return r
}

// validateAlbumMatch checks if any RP album fuzzy-matches any MB album.
// Flags BadMatch when: mismatch + no album overlap.
func (r *ArtistResult) validateAlbumMatch() {
	if !r.IsMismatch {
		return
	}
	// MB artist has no albums = match is wrong
	if len(r.Albums) == 0 {
		r.BadMatch = true
		return
	}
	// No RP albums to compare against = can't validate, keep match
	if len(r.RPAlbums) == 0 {
		return
	}
	mbTitles := make([]string, len(r.Albums))
	for i, a := range r.Albums {
		mbTitles[i] = a.Title
	}
	for _, rpAlbum := range r.RPAlbums {
		if rpAlbum == "" {
			continue
		}
		for _, mbTitle := range mbTitles {
			if albumNamesMatch(rpAlbum, mbTitle) {
				r.AlbumMatch = rpAlbum + " ~ " + mbTitle
				return
			}
		}
	}
	r.BadMatch = true
}

// albumNamesMatch does fuzzy comparison of album names.
func albumNamesMatch(a, b string) bool {
	aNorm := normalizeAlbumName(a)
	bNorm := normalizeAlbumName(b)
	if aNorm == "" || bNorm == "" {
		return false
	}
	if aNorm == bNorm {
		return true
	}
	if strings.Contains(aNorm, bNorm) || strings.Contains(bNorm, aNorm) {
		return true
	}
	return false
}

func normalizeAlbumName(s string) string {
	s = strings.ToLower(s)
	// Strip remaster/reissue suffixes
	s = regexp.MustCompile(`\s*\(?(remaster(ed)?|deluxe|expanded|reissue|anniversary|bonus tracks?|special edition)\s*\d*\)?`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\s*\[?(remaster(ed)?|deluxe|expanded|reissue|anniversary)\]?\s*\d*`).ReplaceAllString(s, "")
	// Strip accents, punctuation
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\'', '"', ',', '.', ':', ';', '!', '?', '(', ')', '[', ']', '-', '\u2019', '\u2018':
			return -1
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			return 'a'
		case 'è', 'é', 'ê', 'ë':
			return 'e'
		case 'ì', 'í', 'î', 'ï':
			return 'i'
		case 'ò', 'ó', 'ô', 'õ', 'ö':
			return 'o'
		case 'ù', 'ú', 'û', 'ü':
			return 'u'
		case 'ý', 'ÿ':
			return 'y'
		case 'ñ':
			return 'n'
		case 'ç', 'č', 'ć':
			return 'c'
		case 'š':
			return 's'
		case 'ž':
			return 'z'
		case 'ř':
			return 'r'
		case 'ď', 'đ':
			return 'd'
		case 'ť':
			return 't'
		case 'ň':
			return 'n'
		case 'ß':
			return 's'
		}
		return r
	}, s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func searchMB(ctx context.Context, query string) ([]struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Score          int    `json:"score"`
	Type           string `json:"type"`
	Disambiguation string `json:"disambiguation"`
}, error) {
	mbRateLimit()
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/artist/?query=%s&fmt=json&limit=10",
		url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Artists []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Score          int    `json:"score"`
			Type           string `json:"type"`
			Disambiguation string `json:"disambiguation"`
		} `json:"artists"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Artists, nil
}

func fetchAlbums(ctx context.Context, mbid string) ([]AlbumResult, int, string) {
	mbRateLimit()

	mbQuery := fmt.Sprintf("arid:%s AND primarytype:Album NOT secondarytype:*", mbid)
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&fmt=json&limit=100",
		strings.ReplaceAll(mbQuery, " ", "%20"))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, 0, err.Error()
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Sprintf("status %d", resp.StatusCode)
	}

	var rgResult struct {
		ReleaseGroups []struct {
			Title            string `json:"title"`
			FirstReleaseDate string `json:"first-release-date"`
		} `json:"release-groups"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rgResult); err != nil {
		return nil, 0, err.Error()
	}

	seen := make(map[string]bool)
	type entry struct {
		title string
		year  string
	}
	var entries []entry
	for _, rg := range rgResult.ReleaseGroups {
		key := strings.ToLower(rg.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		year := ""
		if len(rg.FirstReleaseDate) >= 4 {
			year = rg.FirstReleaseDate[:4]
		}
		entries = append(entries, entry{rg.Title, year})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].year == "" {
			return false
		}
		if entries[j].year == "" {
			return true
		}
		return entries[i].year < entries[j].year
	})

	var albums []AlbumResult
	for _, e := range entries {
		albums = append(albums, AlbumResult{Title: e.title, Year: e.year})
	}
	return albums, rgResult.Count, ""
}

func printSummary(results []ArtistResult) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	total := len(results)
	var found, failed, mismatches, badMatches int
	var byPass [5]int

	for _, r := range results {
		if r.Error != "" {
			failed++
			continue
		}
		found++
		switch r.Pass {
		case "exact":
			byPass[0]++
		case "normalized":
			byPass[1]++
		case "forward-contains":
			byPass[2]++
		case "first-person-group":
			byPass[3]++
		default:
			byPass[4]++
		}
		if r.IsMismatch {
			mismatches++
			if r.BadMatch {
				badMatches++
			}
		}
	}

	fmt.Printf("\nTotal artists: %d\n", total)
	fmt.Printf("Found:         %d (%.0f%%)\n", found, pct(found, total))
	fmt.Printf("Failed:        %d (%.0f%%)\n", failed, pct(failed, total))
	fmt.Printf("Name matches:  exact=%d, normalized=%d, forward-contains=%d, first-person-group=%d, fallback=%d\n",
		byPass[0], byPass[1], byPass[2], byPass[3], byPass[4])
	fmt.Printf("Mismatches:    %d (%.0f%%)\n", mismatches, pct(mismatches, total))
	fmt.Printf("  - bad matches:        %d\n", badMatches)
	fmt.Printf("  - album match:        %d\n", mismatches-badMatches)

	// Show bad matches with full detail
	if badMatches > 0 {
		fmt.Printf("\n%s\n", strings.Repeat("-", 80))
		fmt.Println("BAD MATCHES (no album overlap)")
		fmt.Println(strings.Repeat("-", 80))

		for _, r := range results {
			if !r.BadMatch {
				continue
			}
			fmt.Printf("\n  RP artist:      %s\n", r.RPArtist)
			fmt.Printf("  RP albums:      %v\n", r.RPAlbums)
			fmt.Printf("  Query norm:     %q\n", r.QueryNorm)
			fmt.Printf("  Matched:        %s (MBID: %s)\n", r.MatchedName, r.MatchedMBID)
			fmt.Printf("  Matched norm:   %q\n", r.MatchedNorm)
			fmt.Printf("  Pass:           %s\n", r.Pass)

			fmt.Printf("  Search results:\n")
			for i, sr := range r.SearchResults {
				marker := "  "
				if sr.MBID == r.MatchedMBID {
					marker = ">>"
				}
				fmt.Printf("    %s [%d] %s (MBID: %s, type: %s, score: %d",
					marker, i+1, sr.Name, sr.MBID, sr.Type, sr.Score)
				if sr.Disambig != "" {
					fmt.Printf(", disambig: %s", sr.Disambig)
				}
				fmt.Println(")")
			}

			fmt.Printf("  MB albums (%d):\n", len(r.Albums))
			for _, a := range r.Albums {
				if a.Year != "" {
					fmt.Printf("    - %s (%s)\n", a.Title, a.Year)
				} else {
					fmt.Printf("    - %s\n", a.Title)
				}
			}
		}
	}

	// Show other mismatches (album match accepted)
	otherMismatches := mismatches - badMatches
	if otherMismatches > 0 {
		fmt.Printf("\n%s\n", strings.Repeat("-", 80))
		fmt.Printf("ACCEPTED MISMATCHES (album match found: %d)\n", otherMismatches)
		fmt.Println(strings.Repeat("-", 80))

		for _, r := range results {
			if !r.IsMismatch || r.BadMatch {
				continue
			}
			collab := ""
			if r.IsCollab {
				collab = " [collab]"
			}
			reason := fmt.Sprintf("album: %s", r.AlbumMatch)
			fmt.Printf("  %s -> %s%s (%s, pass: %s)\n", r.RPArtist, r.MatchedName, collab, reason, r.Pass)
		}
	}

	// Show failures
	if failed > 0 {
		fmt.Printf("\n%s\n", strings.Repeat("-", 80))
		fmt.Println("FAILED (not found)")
		fmt.Println(strings.Repeat("-", 80))
		for _, r := range results {
			if r.Error != "" {
				fmt.Printf("  %s — %s\n", r.RPArtist, r.Error)
			}
		}
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func normalizeForCompare(s string) string {
	// Strip leading "the" but not if it would leave nothing useful (e.g., "The The" → "the")
	stripped := regexp.MustCompile(`^the\s+`).ReplaceAllString(s, "")
	if stripped != "" && stripped != "the" {
		s = stripped
	}
	s = strings.Map(func(r rune) rune {
		switch r {
		case '\'', '\u2018', '\u2019', '\u201A', '\u201B', '\u02BC':
			return -1
		case '-', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014':
			return -1
		case '&':
			return -1
		case ',':
			return -1
		case '.':
			return -1
		case 'à', 'á', 'â', 'ã', 'ä', 'å':
			return 'a'
		case 'è', 'é', 'ê', 'ë':
			return 'e'
		case 'ì', 'í', 'î', 'ï':
			return 'i'
		case 'ò', 'ó', 'ô', 'õ', 'ö':
			return 'o'
		case 'ù', 'ú', 'û', 'ü':
			return 'u'
		case 'ý', 'ÿ':
			return 'y'
		case 'ñ':
			return 'n'
		case 'ç':
			return 'c'
		case 'ß':
			return 's'
		case 'À', 'Á', 'Â', 'Ã', 'Ä', 'Å':
			return 'A'
		case 'È', 'É', 'Ê', 'Ë':
			return 'E'
		case 'Ì', 'Í', 'Î', 'Ï':
			return 'I'
		case 'Ò', 'Ó', 'Ô', 'Õ', 'Ö':
			return 'O'
		case 'Ù', 'Ú', 'Û', 'Ü':
			return 'U'
		case 'Ñ':
			return 'N'
		case 'Ç':
			return 'C'
		}
		return r
	}, s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func fetchRPArtists(ctx context.Context, channelsStr string) map[string][]string {
	channels := strings.Split(channelsStr, ",")
	artistAlbums := make(map[string]map[string]bool) // artist -> set of album names

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
			album, _ := song["album"].(string)
			if artist != "" && artist != "Unknown Artist" {
				if artistAlbums[artist] == nil {
					artistAlbums[artist] = make(map[string]bool)
				}
				if album != "" {
					artistAlbums[artist][album] = true
				}
			}
		}
	}

	// Convert sets to sorted slices
	result := make(map[string][]string)
	for artist, albumSet := range artistAlbums {
		var albums []string
		for a := range albumSet {
			albums = append(albums, a)
		}
		sort.Strings(albums)
		result[artist] = albums
	}
	return result
}
