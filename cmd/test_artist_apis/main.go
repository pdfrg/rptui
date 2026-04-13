package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pdfrg/rptui/internal/api"
)

var (
	discogsIDCache   = make(map[string]string)
	discogsIDCacheMu sync.Mutex
	discogsSem       = make(chan struct{}, 4)
)

type ArtistResult struct {
	Artist      string
	Wikipedia   *WikiResult
	TheAudioDB  *TADBResult
	MusicBrainz *MBResult
	Discogs     *DiscogsResult
}

type WikiResult struct {
	PageTitle    string
	PageURL      string
	Summary      string
	ThumbnailURL string
	Discography  string
	Error        string
}

type TADBResult struct {
	ArtistName string
	Bio        string
	Thumb      string
	FanArts    []string
	Error      string
}

type MBSearchResult struct {
	Name     string
	MBID     string
	Score    int
	Type     string
	Disambig string
}

type MBResult struct {
	SearchResults []MBSearchResult
	MatchedName   string
	MatchedMBID   string
	Albums        []string
	TotalGroups   int
	Error         string
}

type DiscogsSearchResult struct {
	ID    int
	Title string
	Type  string
}

type DiscogsResult struct {
	SearchResults []DiscogsSearchResult
	Bio           string
	Images        []string
	Error         string
}

func main() {
	ctx := context.Background()

	// Collect unique artists across all channels
	channels := []int{0, 1, 2, 3, 5}
	artists := make(map[string]bool)
	artistSongs := make(map[string][]string) // artist -> list of "title (album, year)"

	for _, ch := range channels {
		rpAPI := api.NewRadioParadiseAPI(ch, 3)
		playlist, err := rpAPI.GetPlaylist(ctx)
		if err != nil {
			log.Printf("Failed to fetch playlist for channel %d: %v", ch, err)
			continue
		}

		fmt.Printf("Channel %d: %d songs\n", ch, len(playlist.Songs))
		for _, song := range playlist.Songs {
			artist, _ := song["artist"].(string)
			title, _ := song["title"].(string)
			album, _ := song["album"].(string)
			year, _ := song["year"].(string)
			if artist != "" && artist != "Unknown Artist" {
				artists[artist] = true
				desc := title
				if album != "" {
					desc += " (" + album
					if year != "" {
						desc += ", " + year
					}
					desc += ")"
				}
				artistSongs[artist] = append(artistSongs[artist], desc)
			}
		}
	}

	var artistList []string
	for artist := range artists {
		artistList = append(artistList, artist)
	}
	sort.Strings(artistList)

	fmt.Printf("\nTotal unique artists across all channels: %d\n", len(artistList))
	fmt.Printf("Testing all %d artists with 1s delay between queries\n\n", len(artistList))

	wikiClient := api.NewWikipediaClient()
	httpClient := &http.Client{Timeout: 15 * time.Second}

	var results []ArtistResult

	for i, artist := range artistList {
		fmt.Printf("[%d/%d] %s\n", i+1, len(artistList), artist)

		result := ArtistResult{Artist: artist}

		// Wikipedia
		result.Wikipedia = testWikipedia(ctx, wikiClient, artist)
		time.Sleep(1 * time.Second)

		// TheAudioDB
		result.TheAudioDB = testTheAudioDB(ctx, httpClient, artist)
		time.Sleep(1 * time.Second)

		// MusicBrainz
		result.MusicBrainz = testMusicBrainz(ctx, httpClient, artist)
		time.Sleep(1 * time.Second)

		// Discogs
		result.Discogs = testDiscogs(ctx, httpClient, artist)
		time.Sleep(1 * time.Second)

		results = append(results, result)
	}

	// Print full report
	printReport(results, artistSongs)

	// Print summary statistics
	printSummary(results)
}

func testWikipedia(ctx context.Context, client *api.WikipediaClient, artist string) *WikiResult {
	info, err := client.FindArtist(ctx, artist)
	if err != nil {
		return &WikiResult{Error: err.Error()}
	}
	if info == nil {
		return &WikiResult{Error: "not found"}
	}
	return &WikiResult{
		PageTitle:    info.PageTitle,
		PageURL:      info.PageURL,
		Summary:      info.Summary,
		ThumbnailURL: info.ThumbnailURL,
		Discography:  info.Discography,
	}
}

func testTheAudioDB(ctx context.Context, client *http.Client, artist string) *TADBResult {
	reqURL := fmt.Sprintf("https://theaudiodb.com/api/v1/json/123/search.php?s=%s",
		url.QueryEscape(artist))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return &TADBResult{Error: err.Error()}
	}

	resp, err := client.Do(req)
	if err != nil {
		return &TADBResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &TADBResult{Error: fmt.Sprintf("status %d", resp.StatusCode)}
	}

	var result struct {
		Artists []struct {
			StrArtist        string `json:"strArtist"`
			StrBiography     string `json:"strBiography"`
			StrArtistThumb   string `json:"strArtistThumb"`
			StrArtistFanart  string `json:"strArtistFanart"`
			StrArtistFanart2 string `json:"strArtistFanart2"`
			StrArtistFanart3 string `json:"strArtistFanart3"`
			StrArtistFanart4 string `json:"strArtistFanart4"`
		} `json:"artists"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &TADBResult{Error: err.Error()}
	}

	if len(result.Artists) == 0 {
		return &TADBResult{Error: "no artist found"}
	}

	art := result.Artists[0]
	var fanArts []string
	for _, fa := range []string{art.StrArtistFanart, art.StrArtistFanart2, art.StrArtistFanart3, art.StrArtistFanart4} {
		if fa != "" {
			fanArts = append(fanArts, fa)
		}
	}

	return &TADBResult{
		ArtistName: art.StrArtist,
		Bio:        art.StrBiography,
		Thumb:      art.StrArtistThumb,
		FanArts:    fanArts,
	}
}

func testMusicBrainz(ctx context.Context, client *http.Client, artist string) *MBResult {
	// Step 1: Search for artist using artistphrase (better for multi-word names)
	// MB's artist field ignores diacritics server-side, so "Noël" matches "Noel"
	var searchResult struct {
		Artists []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Score          int    `json:"score"`
			Type           string `json:"type"`
			Disambiguation string `json:"disambiguation"`
		} `json:"artists"`
	}

	searchMB := func(query string) error {
		reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/artist/?query=%s&fmt=json&limit=10",
			url.QueryEscape(query))
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "rptui-test/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(&searchResult)
	}

	// Try artistphrase first (scores full names higher)
	if err := searchMB(fmt.Sprintf("artistphrase:%s", artist)); err != nil {
		return &MBResult{Error: err.Error()}
	}

	// If no results, fall back to quoted artist:"name"
	if len(searchResult.Artists) == 0 {
		if err := searchMB(fmt.Sprintf("artist:\"%s\"", artist)); err != nil {
			return &MBResult{Error: err.Error()}
		}
	}

	if len(searchResult.Artists) == 0 {
		return &MBResult{Error: "no artist found in search"}
	}

	// Record all search results
	var searchResults []MBSearchResult
	for _, a := range searchResult.Artists {
		searchResults = append(searchResults, MBSearchResult{
			Name:     a.Name,
			MBID:     a.ID,
			Score:    a.Score,
			Type:     a.Type,
			Disambig: a.Disambiguation,
		})
	}

	// Pick best match: prefer exact name match, then normalized, then contains, then score
	artistLower := strings.ToLower(artist)
	artistNorm := normalizeForCompare(artistLower)
	artistID := searchResult.Artists[0].ID
	matchedName := searchResult.Artists[0].Name

	// Pass 1: exact name match
	for _, a := range searchResult.Artists {
		if strings.ToLower(a.Name) == artistLower {
			artistID, matchedName = a.ID, a.Name
			break
		}
	}

	// Pass 2: normalized match (strip "the", accents)
	if strings.ToLower(matchedName) != artistLower {
		for _, a := range searchResult.Artists {
			if normalizeForCompare(strings.ToLower(a.Name)) == artistNorm {
				artistID, matchedName = a.ID, a.Name
				break
			}
		}
	}

	// Pass 3: result name contains query (catches "Christian Scott aTunde Adjuah")
	if strings.ToLower(matchedName) != artistLower && normalizeForCompare(strings.ToLower(matchedName)) != artistNorm {
		for _, a := range searchResult.Artists {
			nameLower := strings.ToLower(a.Name)
			if strings.Contains(nameLower, artistLower) {
				artistID, matchedName = a.ID, a.Name
				break
			}
		}
	}

	// Pass 4: fall back to first Person/Group result
	if strings.ToLower(matchedName) != artistLower && normalizeForCompare(strings.ToLower(matchedName)) != artistNorm {
		for _, a := range searchResult.Artists {
			if a.Type == "Person" || a.Type == "Group" {
				artistID, matchedName = a.ID, a.Name
				break
			}
		}
	}

	time.Sleep(1 * time.Second) // MB rate limit between calls

	// Step 2: Fetch release groups — use search API with correct field names
	// primarytype:Album matches the MB website "Album" filter
	// NOT secondarytype:* excludes anything with a secondary type (compilation, live, remix, etc.)
	mbQuery := fmt.Sprintf("arid:%s AND primarytype:Album NOT secondarytype:*", artistID)
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&fmt=json&limit=100",
		strings.ReplaceAll(mbQuery, " ", "%20"))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return &MBResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("release-groups fetch error: %v", err),
		}
	}
	req.Header.Set("User-Agent", "rptui-test/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return &MBResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("release-groups fetch error: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &MBResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("release-groups status %d", resp.StatusCode),
		}
	}

	var rgResult struct {
		ReleaseGroups []struct {
			Title            string `json:"title"`
			FirstReleaseDate string `json:"first-release-date"`
			ID               string `json:"id"`
		} `json:"release-groups"`
		Count int `json:"count"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rgResult); err != nil {
		return &MBResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("release-groups decode error: %v", err),
		}
	}

	// Deduplicate by title and sort by year
	type albumEntry struct {
		title string
		year  string
	}
	seen := make(map[string]bool)
	var entries []albumEntry
	for _, rg := range rgResult.ReleaseGroups {
		titleLower := strings.ToLower(rg.Title)
		if seen[titleLower] {
			continue
		}
		seen[titleLower] = true

		year := ""
		if len(rg.FirstReleaseDate) >= 4 {
			year = rg.FirstReleaseDate[:4]
		}
		entries = append(entries, albumEntry{title: rg.Title, year: year})
	}

	// Sort oldest to newest
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].year == "" {
			return false
		}
		if entries[j].year == "" {
			return true
		}
		return entries[i].year < entries[j].year
	})

	var albums []string
	for _, e := range entries {
		if e.year != "" {
			albums = append(albums, fmt.Sprintf("%s (%s)", e.title, e.year))
		} else {
			albums = append(albums, e.title)
		}
	}

	return &MBResult{
		SearchResults: searchResults,
		MatchedName:   matchedName,
		MatchedMBID:   artistID,
		Albums:        albums,
		TotalGroups:   rgResult.Count,
	}
}

func cleanDiscogsProfile(text string, resolveID func(string) string) string {
	discogsIDTagRegex := regexp.MustCompile(`\[(a|r|l|m)(=?)(\d+)\]`)
	discogsNamedTagRegex := regexp.MustCompile(`\[(a|r|l)=([^\]]+)\]`)
	discogsURLTagRegex := regexp.MustCompile(`\[url=[^\]]+\](.*?)\[/url\]`)
	discogsBoldItalicTagRegex := regexp.MustCompile(`\[/?(?:b|i)\]`)

	text = discogsIDTagRegex.ReplaceAllStringFunc(text, func(tag string) string {
		if resolveID != nil {
			if name := resolveID(tag); name != "" {
				return name
			}
		}
		return ""
	})
	text = discogsNamedTagRegex.ReplaceAllString(text, "$2")
	text = discogsURLTagRegex.ReplaceAllString(text, "$1")
	text = discogsBoldItalicTagRegex.ReplaceAllString(text, "")

	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\s([,.;:!?])`).ReplaceAllString(text, "$1")

	return text
}

func fetchDiscogsName(ctx context.Context, client *http.Client, entityType, idStr string) (string, bool) {
	cacheKey := entityType + ":" + idStr

	discogsIDCacheMu.Lock()
	if name, ok := discogsIDCache[cacheKey]; ok {
		discogsIDCacheMu.Unlock()
		return name, name != ""
	}
	discogsIDCacheMu.Unlock()

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return "", false
	}

	select {
	case discogsSem <- struct{}{}:
		defer func() { <-discogsSem }()
	case <-ctx.Done():
		return "", false
	}

	var endpoint string
	switch entityType {
	case "a":
		endpoint = fmt.Sprintf("https://api.discogs.com/artists/%d", id)
	case "l":
		endpoint = fmt.Sprintf("https://api.discogs.com/labels/%d", id)
	case "r":
		endpoint = fmt.Sprintf("https://api.discogs.com/releases/%d", id)
	case "m":
		endpoint = fmt.Sprintf("https://api.discogs.com/masters/%d", id)
	default:
		return "", false
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "rptui-test/1.0")
	if token := os.Getenv("DISCOGS_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Discogs token="+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try alternate endpoint for r/m tags
		var altEndpoint string
		if entityType == "r" {
			altEndpoint = fmt.Sprintf("https://api.discogs.com/masters/%d", id)
		} else if entityType == "m" {
			altEndpoint = fmt.Sprintf("https://api.discogs.com/releases/%d", id)
		} else {
			discogsIDCacheMu.Lock()
			discogsIDCache[cacheKey] = ""
			discogsIDCacheMu.Unlock()
			return "", false
		}
		req2, _ := http.NewRequestWithContext(ctx, "GET", altEndpoint, nil)
		if req2 != nil {
			req2.Header.Set("User-Agent", "rptui-test/1.0")
			if token := os.Getenv("DISCOGS_TOKEN"); token != "" {
				req2.Header.Set("Authorization", "Discogs token="+token)
			}
			resp2, err := client.Do(req2)
			if err == nil {
				defer resp2.Body.Close()
				if resp2.StatusCode == http.StatusOK {
					var r struct {
						Title string `json:"title"`
					}
					if json.NewDecoder(resp2.Body).Decode(&r) == nil && r.Title != "" {
						discogsIDCacheMu.Lock()
						discogsIDCache[cacheKey] = r.Title
						discogsIDCacheMu.Unlock()
						return r.Title, true
					}
				}
			}
		}
		discogsIDCacheMu.Lock()
		discogsIDCache[cacheKey] = ""
		discogsIDCacheMu.Unlock()
		return "", false
	}

	var result struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false
	}

	name := result.Name
	if name == "" {
		name = result.Title
	}

	discogsIDCacheMu.Lock()
	discogsIDCache[cacheKey] = name
	discogsIDCacheMu.Unlock()

	return name, name != ""
}

func resolveDiscogsBio(ctx context.Context, client *http.Client, text string) string {
	discogsIDTagRegex := regexp.MustCompile(`\[(a|r|l|m)(=?)(\d+)\]`)

	type idEntry struct {
		fullMatch string
		typ       string
		idStr     string
		pos       int
	}

	seen := make(map[string]bool)
	var entries []idEntry
	for _, m := range discogsIDTagRegex.FindAllStringSubmatchIndex(text, -1) {
		typ := text[m[2]:m[3]]
		idStr := text[m[6]:m[7]]
		key := typ + ":" + idStr
		if !seen[key] {
			seen[key] = true
			entries = append(entries, idEntry{
				fullMatch: text[m[0]:m[1]],
				typ:       typ,
				idStr:     idStr,
				pos:       m[0],
			})
		}
	}

	if len(entries) == 0 {
		return cleanDiscogsProfile(text, nil)
	}

	type result struct {
		idx  int
		name string
		ok   bool
	}
	results := make(chan result, len(entries))
	for i, e := range entries {
		go func(idx int, typ, id string) {
			name, ok := fetchDiscogsName(ctx, client, typ, id)
			results <- result{idx, name, ok}
		}(i, e.typ, e.idStr)
	}

	nameMap := make(map[string]string)
	for i := 0; i < len(entries); i++ {
		r := <-results
		if r.ok {
			nameMap[entries[r.idx].fullMatch] = r.name
		}
	}

	return cleanDiscogsProfile(text, func(tag string) string {
		return nameMap[tag]
	})
}

func testDiscogs(ctx context.Context, client *http.Client, artist string) *DiscogsResult {
	hasToken := os.Getenv("DISCOGS_TOKEN") != ""

	// Step 1: Search for artist
	reqURL := fmt.Sprintf("https://api.discogs.com/database/search?q=%s&type=artist",
		url.QueryEscape(artist))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return &DiscogsResult{Error: err.Error()}
	}
	req.Header.Set("User-Agent", "rptui-test/1.0")
	if token := os.Getenv("DISCOGS_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Discogs token="+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return &DiscogsResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		return &DiscogsResult{Error: fmt.Sprintf("search status %d: %s", resp.StatusCode, string(body[:n]))}
	}

	var searchResult struct {
		Results []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return &DiscogsResult{Error: err.Error()}
	}

	if len(searchResult.Results) == 0 {
		return &DiscogsResult{Error: "no artist found"}
	}

	// Record search results
	var searchResults []DiscogsSearchResult
	for _, r := range searchResult.Results {
		if len(searchResults) >= 5 {
			break
		}
		searchResults = append(searchResults, DiscogsSearchResult{
			ID:    r.ID,
			Title: r.Title,
			Type:  r.Type,
		})
	}

	// Find best match: exact match first, then try normalized comparison
	artistLower := strings.ToLower(artist)
	artistID := searchResult.Results[0].ID
	for _, r := range searchResult.Results {
		if strings.ToLower(r.Title) == artistLower {
			artistID = r.ID
			break
		}
	}
	// If no exact match, try normalized comparison (strip "the", accents)
	if artistID == searchResult.Results[0].ID {
		queryNorm := normalizeForCompare(artistLower)
		for _, r := range searchResult.Results {
			titleNorm := normalizeForCompare(strings.ToLower(r.Title))
			if titleNorm == queryNorm {
				artistID = r.ID
				break
			}
		}
	}

	time.Sleep(1 * time.Second)

	// Step 2: Fetch artist details
	reqURL = fmt.Sprintf("https://api.discogs.com/artists/%d", artistID)
	req, err = http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return &DiscogsResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("artist fetch error: %v", err),
		}
	}
	req.Header.Set("User-Agent", "rptui-test/1.0")

	resp, err = client.Do(req)
	if err != nil {
		return &DiscogsResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("artist fetch error: %v", err),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &DiscogsResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("artist status %d", resp.StatusCode),
		}
	}

	var artistResult struct {
		Name    string `json:"name"`
		Profile string `json:"profile"`
		Images  []struct {
			Type   string `json:"type"`
			URI    string `json:"uri"`
			URI150 string `json:"uri150"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&artistResult); err != nil {
		return &DiscogsResult{
			SearchResults: searchResults,
			Error:         fmt.Sprintf("artist decode error: %v", err),
		}
	}

	var images []string
	for _, img := range artistResult.Images {
		typeLabel := img.Type
		if typeLabel == "" {
			typeLabel = "unknown"
		}
		images = append(images, fmt.Sprintf("[%s] %s (%dx%d)", typeLabel, img.URI, img.Width, img.Height))
	}

	_ = hasToken

	return &DiscogsResult{
		SearchResults: searchResults,
		Bio:           resolveDiscogsBio(ctx, client, artistResult.Profile),
		Images:        images,
	}
}

func printReport(results []ArtistResult, artistSongs map[string][]string) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("FULL ARTIST REPORT")
	fmt.Println(strings.Repeat("=", 80))

	for i, r := range results {
		fmt.Printf("\n%s\n", strings.Repeat("-", 80))
		fmt.Printf("ARTIST %d: %s\n", i+1, r.Artist)
		fmt.Printf("%s\n", strings.Repeat("-", 80))

		// Songs from RP
		if songs, ok := artistSongs[r.Artist]; ok {
			fmt.Printf("\nRP Songs:\n")
			for _, s := range songs {
				fmt.Printf("  - %s\n", s)
			}
		}

		// Wikipedia
		fmt.Printf("\n[Wikipedia]\n")
		if r.Wikipedia == nil {
			fmt.Printf("  No result\n")
		} else if r.Wikipedia.Error != "" {
			fmt.Printf("  ERROR: %s\n", r.Wikipedia.Error)
		} else {
			fmt.Printf("  Page: %s\n", r.Wikipedia.PageTitle)
			fmt.Printf("  URL: %s\n", r.Wikipedia.PageURL)
			fmt.Printf("  Thumbnail: %s\n", r.Wikipedia.ThumbnailURL)
			fmt.Printf("  Summary:\n%s\n", indent(r.Wikipedia.Summary, "    "))
			if r.Wikipedia.Discography != "" {
				fmt.Printf("  Discography:\n%s\n", indent(r.Wikipedia.Discography, "    "))
			} else {
				fmt.Printf("  Discography: (none)\n")
			}
		}

		// TheAudioDB
		fmt.Printf("\n[TheAudioDB]\n")
		if r.TheAudioDB == nil {
			fmt.Printf("  No result\n")
		} else if r.TheAudioDB.Error != "" {
			fmt.Printf("  ERROR: %s\n", r.TheAudioDB.Error)
		} else {
			fmt.Printf("  Matched name: %s\n", r.TheAudioDB.ArtistName)
			fmt.Printf("  Thumb: %s\n", r.TheAudioDB.Thumb)
			fmt.Printf("  Fan arts (%d):\n", len(r.TheAudioDB.FanArts))
			for j, fa := range r.TheAudioDB.FanArts {
				fmt.Printf("    [%d] %s\n", j+1, fa)
			}
			fmt.Printf("  Bio:\n%s\n", indent(r.TheAudioDB.Bio, "    "))
		}

		// MusicBrainz
		fmt.Printf("\n[MusicBrainz]\n")
		if r.MusicBrainz == nil {
			fmt.Printf("  No result\n")
		} else if r.MusicBrainz.Error != "" {
			fmt.Printf("  ERROR: %s\n", r.MusicBrainz.Error)
		} else {
			fmt.Printf("  Search results:\n")
			for j, sr := range r.MusicBrainz.SearchResults {
				marker := "  "
				if sr.MBID == r.MusicBrainz.MatchedMBID {
					marker = ">>"
				}
				fmt.Printf("    %s [%d] %s (MBID: %s, type: %s, score: %d, disambig: %s)\n",
					marker, j+1, sr.Name, sr.MBID, sr.Type, sr.Score, sr.Disambig)
			}
			fmt.Printf("  Matched: %s (MBID: %s)\n", r.MusicBrainz.MatchedName, r.MusicBrainz.MatchedMBID)
			fmt.Printf("  Albums (%d displayed, %d total groups):\n", len(r.MusicBrainz.Albums), r.MusicBrainz.TotalGroups)
			for _, a := range r.MusicBrainz.Albums {
				fmt.Printf("    - %s\n", a)
			}
		}

		// Discogs
		fmt.Printf("\n[Discogs]\n")
		if r.Discogs == nil {
			fmt.Printf("  No result\n")
		} else {
			if len(r.Discogs.SearchResults) > 0 {
				fmt.Printf("  Search results:\n")
				for j, sr := range r.Discogs.SearchResults {
					fmt.Printf("    [%d] %s (ID: %d, type: %s)\n", j+1, sr.Title, sr.ID, sr.Type)
				}
			}
			if r.Discogs.Error != "" {
				fmt.Printf("  ERROR: %s\n", r.Discogs.Error)
			} else {
				fmt.Printf("  Images (%d):\n", len(r.Discogs.Images))
				for _, img := range r.Discogs.Images {
					fmt.Printf("    - %s\n", img)
				}
				fmt.Printf("  Bio:\n%s\n", indent(r.Discogs.Bio, "    "))
			}
		}
	}
}

func printSummary(results []ArtistResult) {
	fmt.Printf("\n\n%s\n", strings.Repeat("=", 80))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	total := len(results)
	pct := func(n int) float64 { return float64(n) / float64(total) * 100 }

	// Wikipedia counters
	var wikiOK, wikiFail int
	var wikiSummary, wikiDiscog, wikiThumb int

	// TheAudioDB counters
	var tadbOK, tadbFail int
	var tadbBio, tadbThumb, tadbFanart int
	var tadbFanartTotal, tadbFanartArtists int
	var tadbBioLenTotal int

	// MusicBrainz counters
	var mbOK, mbFail, mbWrongMatch int

	// Discogs counters
	var discogsOK, discogsFail int
	var discogsBio, discogsPrimaryImg, discogsSecondaryImg int
	var discogsSecondaryTotal, discogsSecondaryArtists int
	var discogsBioLenTotal int

	for _, r := range results {
		// Wikipedia
		if r.Wikipedia == nil || r.Wikipedia.Error != "" {
			wikiFail++
		} else {
			wikiOK++
			if r.Wikipedia.Summary != "" {
				wikiSummary++
			}
			if r.Wikipedia.Discography != "" {
				wikiDiscog++
			}
			if r.Wikipedia.ThumbnailURL != "" {
				wikiThumb++
			}
		}

		// TheAudioDB
		if r.TheAudioDB == nil || r.TheAudioDB.Error != "" {
			tadbFail++
		} else {
			tadbOK++
			if r.TheAudioDB.Bio != "" {
				tadbBio++
				tadbBioLenTotal += len(r.TheAudioDB.Bio)
			}
			if r.TheAudioDB.Thumb != "" {
				tadbThumb++
			}
			if len(r.TheAudioDB.FanArts) > 0 {
				tadbFanart++
				tadbFanartTotal += len(r.TheAudioDB.FanArts)
				tadbFanartArtists++
			}
		}

		// MusicBrainz
		if r.MusicBrainz == nil || r.MusicBrainz.Error != "" {
			mbFail++
		} else {
			mbOK++
			if strings.ToLower(r.MusicBrainz.MatchedName) != strings.ToLower(r.Artist) {
				mbWrongMatch++
			}
		}

		// Discogs
		if r.Discogs == nil || r.Discogs.Error != "" {
			discogsFail++
		} else {
			discogsOK++
			if r.Discogs.Bio != "" {
				discogsBio++
				discogsBioLenTotal += len(r.Discogs.Bio)
			}
			// Parse image type from strings like "[primary] URL (600x398)"
			for _, img := range r.Discogs.Images {
				if strings.HasPrefix(img, "[primary]") {
					discogsPrimaryImg++
				} else {
					discogsSecondaryImg++
				}
			}
			secCount := 0
			for _, img := range r.Discogs.Images {
				if !strings.HasPrefix(img, "[primary]") {
					secCount++
				}
			}
			if secCount > 0 {
				discogsSecondaryTotal += secCount
				discogsSecondaryArtists++
			}
		}
	}

	fmt.Printf("\nTotal artists tested: %d\n\n", total)
	fmt.Println(strings.Repeat("-", 80))

	// Wikipedia
	fmt.Printf("WIKIPEDIA  (%d found, %d failed — %.0f%% coverage)\n", wikiOK, wikiFail, pct(wikiOK))
	fmt.Printf("  Summary:     %d / %d (%.0f%%)\n", wikiSummary, total, pct(wikiSummary))
	fmt.Printf("  Discography: %d / %d (%.0f%%)\n", wikiDiscog, total, pct(wikiDiscog))
	fmt.Printf("  Thumbnail:   %d / %d (%.0f%%)\n", wikiThumb, total, pct(wikiThumb))
	if wikiSummary > 0 {
		fmt.Printf("  (note: discography often fails due to wiki page structure inconsistencies)\n")
	}

	// TheAudioDB
	fmt.Printf("\nTHEAUDIODB (%d found, %d failed — %.0f%% coverage)\n", tadbOK, tadbFail, pct(tadbOK))
	fmt.Printf("  Bio:              %d / %d (%.0f%%)\n", tadbBio, total, pct(tadbBio))
	fmt.Printf("  Primary image:    %d / %d (%.0f%%)\n", tadbThumb, total, pct(tadbThumb))
	fmt.Printf("  Fan art (any):    %d / %d (%.0f%%)\n", tadbFanart, total, pct(tadbFanart))
	if tadbFanartArtists > 0 {
		avgFan := float64(tadbFanartTotal) / float64(tadbFanartArtists)
		fmt.Printf("  Avg fan art imgs: %.1f (among %d artists with fan art)\n", avgFan, tadbFanartArtists)
	}
	if tadbBio > 0 {
		avgBio := tadbBioLenTotal / tadbBio
		fmt.Printf("  Avg bio length:   %d chars (among %d artists with bio)\n", avgBio, tadbBio)
	}

	// MusicBrainz
	fmt.Printf("\nMUSICBRAINZ (%d found, %d failed — %.0f%% coverage)\n", mbOK, mbFail, pct(mbOK))
	fmt.Printf("  Name mismatches:  %d / %d (%.0f%%)\n", mbWrongMatch, mbOK, float64(mbWrongMatch)/float64(max(mbOK, 1))*100)

	// Discogs
	fmt.Printf("\nDISCOGS    (%d found, %d failed — %.0f%% coverage)\n", discogsOK, discogsFail, pct(discogsOK))
	fmt.Printf("  Bio:              %d / %d (%.0f%%)\n", discogsBio, total, pct(discogsBio))
	fmt.Printf("  Primary image:    %d / %d (%.0f%%)\n", discogsPrimaryImg, total, pct(discogsPrimaryImg))
	fmt.Printf("  Secondary images: %d / %d (%.0f%%)\n", discogsSecondaryArtists, total, pct(discogsSecondaryArtists))
	if discogsSecondaryArtists > 0 {
		avgSec := float64(discogsSecondaryTotal) / float64(discogsSecondaryArtists)
		fmt.Printf("  Avg secondary:    %.1f (among %d artists with secondary images)\n", avgSec, discogsSecondaryArtists)
	}
	if discogsBio > 0 {
		avgBio := discogsBioLenTotal / discogsBio
		fmt.Printf("  Avg bio length:   %d chars (among %d artists with bio)\n", avgBio, discogsBio)
	}

	// Cross-source comparison
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("CROSS-SOURCE COMPARISON")

	var tadbNotWiki, wikiNotTadb, discogsNotTadb, tadbNotDiscogs int
	var bioBothSources, bioTadbOnly, bioDiscogsOnly int
	for _, r := range results {
		tadbFound := r.TheAudioDB != nil && r.TheAudioDB.Error == ""
		wikiFound := r.Wikipedia != nil && r.Wikipedia.Error == ""
		discogsFound := r.Discogs != nil && r.Discogs.Error == ""

		if tadbFound && !wikiFound {
			tadbNotWiki++
		}
		if wikiFound && !tadbFound {
			wikiNotTadb++
		}
		if discogsFound && !tadbFound {
			discogsNotTadb++
		}
		if tadbFound && !discogsFound {
			tadbNotDiscogs++
		}

		tadbHasBio := tadbFound && r.TheAudioDB.Bio != ""
		discogsHasBio := discogsFound && r.Discogs.Bio != ""
		if tadbHasBio && discogsHasBio {
			bioBothSources++
		} else if tadbHasBio {
			bioTadbOnly++
		} else if discogsHasBio {
			bioDiscogsOnly++
		}
	}
	fmt.Printf("  TheAudioDB found but Wikipedia didn't:  %d\n", tadbNotWiki)
	fmt.Printf("  Wikipedia found but TheAudioDB didn't:  %d\n", wikiNotTadb)
	fmt.Printf("  Discogs found but TheAudioDB didn't:    %d\n", discogsNotTadb)
	fmt.Printf("  TheAudioDB found but Discogs didn't:    %d\n", tadbNotDiscogs)
	fmt.Printf("\n  Bio available from both TADB+Discogs:   %d\n", bioBothSources)
	fmt.Printf("  Bio from TADB only:                     %d\n", bioTadbOnly)
	fmt.Printf("  Bio from Discogs only:                  %d\n", bioDiscogsOnly)
	fmt.Printf("  Bio from neither:                       %d\n", total-bioBothSources-bioTadbOnly-bioDiscogsOnly)

	// Coverage distribution: how many sources found each artist
	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("COVERAGE DISTRIBUTION")
	var byCount [5]int // artists found by 0, 1, 2, 3, 4 sources
	var wikiExactTitle int
	for _, r := range results {
		sources := 0
		if r.Wikipedia != nil && r.Wikipedia.Error == "" {
			sources++
			if strings.EqualFold(r.Wikipedia.PageTitle, r.Artist) ||
				strings.HasPrefix(strings.ToLower(r.Wikipedia.PageTitle), strings.ToLower(r.Artist)+" (") {
				wikiExactTitle++
			}
		}
		if r.TheAudioDB != nil && r.TheAudioDB.Error == "" {
			sources++
		}
		if r.MusicBrainz != nil && r.MusicBrainz.Error == "" {
			sources++
		}
		if r.Discogs != nil && r.Discogs.Error == "" {
			sources++
		}
		byCount[sources]++
	}
	fmt.Printf("  Found by 4/4 sources: %d\n", byCount[4])
	fmt.Printf("  Found by 3/4 sources: %d\n", byCount[3])
	fmt.Printf("  Found by 2/4 sources: %d\n", byCount[2])
	fmt.Printf("  Found by 1/4 sources: %d\n", byCount[1])
	fmt.Printf("  Found by 0/4 sources: %d\n", byCount[0])

	fmt.Printf("\n  Wikipedia page title matches artist name: %d / %d (%.0f%% of wiki matches)\n",
		wikiExactTitle, wikiOK, float64(wikiExactTitle)/float64(max(wikiOK, 1))*100)
}

func indent(text, prefix string) string {
	if text == "" {
		return prefix + "(empty)"
	}
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		result = append(result, prefix+line)
	}
	return strings.Join(result, "\n")
}

// normalizeForCompare strips "the" prefix, diacritical marks, and normalizes punctuation
func normalizeForCompare(s string) string {
	// Strip leading "the"
	s = regexp.MustCompile(`^the\s+`).ReplaceAllString(s, "")
	// Normalize punctuation: various apostrophes, hyphens, dashes to nothing or standard form
	s = strings.Map(func(r rune) rune {
		switch r {
		// Apostrophes and quotes — strip them
		case '\'', '\u2018', '\u2019', '\u201A', '\u201B', '\u02BC':
			return -1
		// Hyphens and dashes — strip them
		case '-', '\u2010', '\u2011', '\u2012', '\u2013', '\u2014':
			return -1
		// Ampersand — strip
		case '&':
			return -1
		// Comma — strip
		case ',':
			return -1
		// Period — strip
		case '.':
			return -1
		// Accented characters
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
	// Collapse whitespace
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
