package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var discogsLogger *log.Logger

func SetDiscogsLogger(l *log.Logger) {
	discogsLogger = l
}

// DiscogsClient provides access to the Discogs API.
// Supports three auth methods (none required, all optional):
//   - Personal access token (simplest — one value from discogs.com/settings/developers)
//   - Consumer key + secret (developer app credentials)
//   - Environment variables: DISCOGS_TOKEN, or DISCOGS_KEY + DISCOGS_SECRET
//
// Without auth: low rate limit (25/min), no image URLs.
// With any auth: high rate limit (60/min), image URLs included.
type DiscogsClient struct {
	httpClient *http.Client
	token      string // personal access token
	key        string // consumer key
	secret     string // consumer secret

	idCache   map[string]string // "a:123456" → "Artist Name"
	idCacheMu sync.Mutex
	sem       chan struct{} // rate-limit semaphore (max 4 concurrent lookups)
}

// DiscogsArtist holds artist data fetched from Discogs
type DiscogsArtist struct {
	Name         string
	Profile      string
	PrimaryImage string
	GalleryURLs  []string
}

// NewDiscogsClient creates a new Discogs API client.
// Config values take priority over environment variables.
// Auth is optional — the client works without it (limited rate, no images).
func NewDiscogsClient(token, key, secret string) *DiscogsClient {
	// Config values override env vars
	if token == "" {
		token = os.Getenv("DISCOGS_TOKEN")
	}
	if key == "" {
		key = os.Getenv("DISCOGS_KEY")
	}
	if secret == "" {
		secret = os.Getenv("DISCOGS_SECRET")
	}

	return &DiscogsClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
		key:        key,
		secret:     secret,
		idCache:    make(map[string]string),
		sem:        make(chan struct{}, 4),
	}
}

// HasAuth returns true if any authentication is configured
func (d *DiscogsClient) HasAuth() bool {
	return d.token != "" || (d.key != "" && d.secret != "")
}

func (d *DiscogsClient) setAuth(req *http.Request) {
	if d.token != "" {
		req.Header.Set("Authorization", "Discogs token="+d.token)
	} else if d.key != "" && d.secret != "" {
		req.Header.Set("Authorization", "Discogs key="+d.key+", secret="+d.secret)
	}
}

// fetchDiscogsName looks up a Discogs entity by type and ID, returning its display name.
// Uses the cache to avoid redundant API calls. Rate-limited by the semaphore.
func (d *DiscogsClient) fetchDiscogsName(ctx context.Context, entityType, idStr string) (string, bool) {
	cacheKey := entityType + ":" + idStr

	d.idCacheMu.Lock()
	if name, ok := d.idCache[cacheKey]; ok {
		d.idCacheMu.Unlock()
		return name, name != ""
	}
	d.idCacheMu.Unlock()

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return "", false
	}

	// Rate-limit: acquire semaphore slot
	select {
	case d.sem <- struct{}{}:
		defer func() { <-d.sem }()
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
		// Release tags: try releases first, fall back to masters
		endpoint = fmt.Sprintf("https://api.discogs.com/releases/%d", id)
	case "m":
		// Master tags: try masters first, fall back to releases
		endpoint = fmt.Sprintf("https://api.discogs.com/masters/%d", id)
	default:
		return "", false
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try alternate endpoint for r/m tags
		if entityType == "r" {
			// Release failed, try master
			return d.fetchMasterName(ctx, id)
		}
		if entityType == "m" {
			// Master failed, try release
			return d.fetchReleaseName(ctx, id)
		}
		d.idCacheMu.Lock()
		d.idCache[cacheKey] = ""
		d.idCacheMu.Unlock()
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

	d.idCacheMu.Lock()
	d.idCache[cacheKey] = name
	d.idCacheMu.Unlock()

	return name, name != ""
}

// fetchReleaseName tries the releases endpoint as fallback for r/m tags.
func (d *DiscogsClient) fetchReleaseName(ctx context.Context, id int) (string, bool) {
	endpoint := fmt.Sprintf("https://api.discogs.com/releases/%d", id)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var result struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false
	}
	return result.Title, result.Title != ""
}

// fetchMasterName tries the masters endpoint as fallback for r tags.
func (d *DiscogsClient) fetchMasterName(ctx context.Context, id int) (string, bool) {
	endpoint := fmt.Sprintf("https://api.discogs.com/masters/%d", id)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	var result struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false
	}
	return result.Title, result.Title != ""
}

// resolveDiscogsBio resolves numeric Discogs IDs in profile text to human-readable names.
// It extracts all unique IDs, fetches names concurrently (respecting rate limits and cache),
// then replaces the tags with resolved names.
func (d *DiscogsClient) resolveDiscogsBio(ctx context.Context, text string) string {
	// Find all unique (type, id) pairs
	type idEntry struct {
		fullMatch string // the complete tag, e.g. "[a3406779]"
		typ       string // a, r, l, m
		idStr     string // the numeric ID
		pos       int    // position in text (for replacement order)
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

	// Fetch all names concurrently
	type result struct {
		idx  int
		name string
		ok   bool
	}
	results := make(chan result, len(entries))
	for i, e := range entries {
		go func(idx int, typ, id string) {
			name, ok := d.fetchDiscogsName(ctx, typ, id)
			results <- result{idx, name, ok}
		}(i, e.typ, e.idStr)
	}

	// Collect results
	nameMap := make(map[string]string) // fullMatch → resolved name
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

// discogsIDTagRegex matches numeric ID tags: [a123], [a=123], [r123], [r=123], etc.
var discogsIDTagRegex = regexp.MustCompile(`\[(a|r|l|m)(=?)(\d+)\]`)

// discogsTagRegex matches Discogs profile formatting tags.
var (
	discogsNamedTagRegex      = regexp.MustCompile(`\[(a|r|l)=([^\]]+)\]`)
	discogsURLTagRegex        = regexp.MustCompile(`\[url=[^\]]+\](.*?)\[/url\]`)
	discogsBoldItalicTagRegex = regexp.MustCompile(`\[/?(?:b|i)\]`)
	discogsUnderlineTagRegex  = regexp.MustCompile(`\[/?(?:u)\]`)
	discogsCapitalTagRegex    = regexp.MustCompile(`\[[A-Z]=[^\]]*\]`)
)

// cleanDiscogsProfile removes Discogs wiki-style formatting from profile text.
// Tags like [a=Artist Name] are replaced with the name; numeric-only tags
// like [a3406779] or [r=817186] are resolved via resolveID if provided,
// otherwise removed entirely; formatting markers are stripped.
func cleanDiscogsProfile(text string, resolveID func(string) string) string {
	// Resolve or strip numeric ID tags: [a=123456], [r=817186], [l=14624], [m=58357], [a3406779], etc.
	text = discogsIDTagRegex.ReplaceAllStringFunc(text, func(tag string) string {
		if resolveID != nil {
			if name := resolveID(tag); name != "" {
				return name
			}
		}
		return ""
	})

	// [a=Name], [r=Name], [l=Name] → keep the name (non-numeric values already handled above)
	text = discogsNamedTagRegex.ReplaceAllString(text, "$2")

	// [url=...]text[/url] → text (keep link text, drop URL)
	text = discogsURLTagRegex.ReplaceAllString(text, "$1")

	// [b], [/b], [i], [/i] → remove (strip bold/italic markers)
	text = discogsBoldItalicTagRegex.ReplaceAllString(text, "")

	// [u], [/u] → remove (strip underline markers)
	text = discogsUnderlineTagRegex.ReplaceAllString(text, "")

	// [A=stuff], [X=stuff] → remove (strip capital-letter tags)
	text = discogsCapitalTagRegex.ReplaceAllString(text, "")

	// Clean up whitespace artifacts
	text = strings.TrimSpace(text)
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\s([,.;:!?])`).ReplaceAllString(text, "$1")

	return text
}

// SearchArtist searches for an artist and fetches their details.
// If albumName is provided, it will be used to disambiguate when multiple artists match
// by checking if the artist has a release matching that album.
func (d *DiscogsClient) SearchArtist(ctx context.Context, artistName, albumName string) (*DiscogsArtist, error) {
	// Step 1: Search for artist
	reqURL := fmt.Sprintf("https://api.discogs.com/database/search?q=%s&type=artist",
		url.QueryEscape(artistName))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discogs search status %d", resp.StatusCode)
	}

	var searchResult struct {
		Results []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResult); err != nil {
		return nil, err
	}

	if len(searchResult.Results) == 0 {
		return nil, nil // not found
	}

	// Find best match using multiple passes
	artistLower := strings.ToLower(artistName)
	artistNorm := normalizeForCompare(artistLower)
	albumNorm := ""
	if albumName != "" {
		albumNorm = normalizeForCompare(strings.ToLower(albumName))
	}

	// Collect candidates for album-based disambiguation
	type candidate struct {
		id    int
		title string
	}
	var exactMatches []candidate
	var normMatches []candidate
	var allMatches []candidate

	for _, r := range searchResult.Results {
		entry := candidate{id: r.ID, title: r.Title}
		allMatches = append(allMatches, entry)
		if strings.ToLower(r.Title) == artistLower {
			exactMatches = append(exactMatches, entry)
		}
		if normalizeForCompare(strings.ToLower(r.Title)) == artistNorm {
			normMatches = append(normMatches, entry)
		}
	}

	artistID := 0

	// Strategy (since RP always provides album):
	// 1. Check top 5 artists from search results
	// 2. For each, check if they have the album in their releases
	// 3. First match wins
	// 4. If none match, fallback to exact/normalized match
	//
	// This handles the "Jack White" edge case where exact match is wrong
	// while minimizing API calls (max 5 release lookups instead of 50)
	if albumNorm != "" && len(allMatches) > 1 {
		if discogsLogger != nil {
			discogsLogger.Printf("Discogs: album disambiguation for '%s' (%d candidates)", albumNorm, len(allMatches))
		}
		// Only check first 5 candidates to limit API calls
		for i := 0; i < min(5, len(allMatches)); i++ {
			cand := allMatches[i]
			if discogsLogger != nil {
				discogsLogger.Printf("Discogs: checking artist ID %d (%s) for album '%s'", cand.id, cand.title, albumNorm)
			}
			if d.hasReleaseWithAlbum(ctx, cand.id, albumNorm) {
				artistID = cand.id
				if discogsLogger != nil {
					discogsLogger.Printf("Discogs: album match found, selected ID=%d", artistID)
				}
				break
			}
		}
	}

	// Fallback: exact match
	if artistID == 0 && len(exactMatches) > 0 {
		artistID = exactMatches[0].id
	}

	// Fallback: normalized match
	if artistID == 0 && len(normMatches) > 0 {
		artistID = normMatches[0].id
	}

	// Pass 4: forward contains match (result name contains query)
	if artistID == 0 {
		for _, r := range searchResult.Results {
			if strings.Contains(strings.ToLower(r.Title), artistLower) {
				artistID = r.ID
				break
			}
		}
	}

	// Fallback to first result
	if artistID == 0 {
		artistID = searchResult.Results[0].ID
	}

	// Step 2: Fetch artist details
	reqURL = fmt.Sprintf("https://api.discogs.com/artists/%d", artistID)
	req, err = http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err = d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discogs artist status %d", resp.StatusCode)
	}

	var artistResult struct {
		Name    string `json:"name"`
		Profile string `json:"profile"`
		Images  []struct {
			Type string `json:"type"`
			URI  string `json:"uri"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&artistResult); err != nil {
		return nil, err
	}

	artist := &DiscogsArtist{
		Name:    artistResult.Name,
		Profile: d.resolveDiscogsBio(ctx, artistResult.Profile),
	}

	for _, img := range artistResult.Images {
		if img.Type == "primary" && artist.PrimaryImage == "" {
			artist.PrimaryImage = img.URI
		} else if img.URI != "" {
			artist.GalleryURLs = append(artist.GalleryURLs, img.URI)
		}
	}

	return artist, nil
}

// hasReleaseWithAlbum checks if an artist has a release matching the given album name.
// Uses a release search with the artist to find the album - more efficient than fetching all releases.
func (d *DiscogsClient) hasReleaseWithAlbum(ctx context.Context, artistID int, albumNorm string) bool {
	// First try the direct release search (more efficient)
	// Use artist ID as filter to confirm this artist has the album
	reqURL := fmt.Sprintf("https://api.discogs.com/database/search?artist=%d&q=%s&type=release&per_page=5",
		artistID, url.QueryEscape(albumNorm))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var result struct {
			Results []struct {
				Title string `json:"title"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.Results) > 0 {
			return true
		}
	}

	// Fallback: check artist's releases if release search didn't work
	// Only fetch first 50 releases - most artists have their main albums in first page
	reqURL = fmt.Sprintf("https://api.discogs.com/artists/%d/releases?per_page=50", artistID)
	req, err = http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "rptui/1.0")
	d.setAuth(req)

	resp, err = d.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var result struct {
		Releases []struct {
			Title string `json:"title"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	for _, rel := range result.Releases {
		relNorm := normalizeForCompare(strings.ToLower(rel.Title))
		if strings.Contains(relNorm, albumNorm) || strings.Contains(albumNorm, relNorm) {
			return true
		}
	}
	return false
}
