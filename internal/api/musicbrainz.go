package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MusicBrainzClient provides access to the MusicBrainz API
type MusicBrainzClient struct {
	httpClient *http.Client
	userAgent  string
}

// MBAlbum represents a studio album from MusicBrainz
type MBAlbum struct {
	Title string
	Year  string
}

// NewMusicBrainzClient creates a new MusicBrainz API client
func NewMusicBrainzClient() *MusicBrainzClient {
	return &MusicBrainzClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		userAgent:  "rptui/1.0",
	}
}

// GetDiscography searches for an artist and returns their official studio albums.
// rpAlbum is the current song's album name from Radio Paradise, used to validate
// that the matched MB artist actually has this album (catches false positives).
// Returns the artist's MusicBrainz ID, albums, and error.
// Returns nil albums if the artist is not found or the match cannot be validated.
func (mb *MusicBrainzClient) GetDiscography(ctx context.Context, artistName string, rpAlbum string) (string, []MBAlbum, error) {
	// Step 1: Search for artist
	mbID, matchedName, err := mb.SearchArtist(ctx, artistName)
	if err != nil {
		return "", nil, err
	}
	if mbID == "" {
		return "", nil, nil // not found
	}

	// Respect MB rate limit between calls
	time.Sleep(1 * time.Second)

	// Step 2: Fetch release groups — official studio albums only
	// primarytype:Album matches the MB website "Album" filter
	// NOT secondarytype:* excludes anything with a secondary type (compilation, live, remix, etc.)
	// AND status:official excludes bootlegs — bootleg is a release-level status, not a secondary type,
	// so bootleg release groups (e.g. "Ellie" by The White Stripes) pass the primarytype/secondarytype
	// filters but have status=Bootleg on their releases.
	mbQuery := fmt.Sprintf("arid:%s AND primarytype:Album NOT secondarytype:* AND status:official", mbID)
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&fmt=json&limit=100",
		strings.ReplaceAll(mbQuery, " ", "%20"))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", mb.userAgent)

	resp, err := mb.httpClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("musicbrainz release-groups status %d", resp.StatusCode)
	}

	var rgResult struct {
		ReleaseGroups []struct {
			Title            string `json:"title"`
			FirstReleaseDate string `json:"first-release-date"`
		} `json:"release-groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rgResult); err != nil {
		return "", nil, err
	}

	// Deduplicate by title and sort by year
	type entry struct {
		title string
		year  string
	}
	seen := make(map[string]bool)
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

	// Sort oldest to newest
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			if entries[j].year == "" {
				continue
			}
			if entries[j-1].year == "" || entries[j].year < entries[j-1].year {
				entries[j], entries[j-1] = entries[j-1], entries[j]
			}
		}
	}

	// Album validation: if the matched name differs from the query and we have
	// an RP album, verify the MB artist actually has a matching album.
	// This catches false positives like "Hieronymus Dros" → "Hieronymus".
	if !strings.EqualFold(matchedName, artistName) && rpAlbum != "" && rpAlbum != "—" {
		hasMatch := false
		for _, rg := range rgResult.ReleaseGroups {
			if AlbumNamesMatch(rpAlbum, rg.Title) {
				hasMatch = true
				break
			}
		}
		if !hasMatch {
			return "", nil, nil // match can't be validated, treat as not found
		}
	}

	var albums []MBAlbum
	for _, e := range entries {
		albums = append(albums, MBAlbum{Title: e.title, Year: e.year})
	}

	return mbID, albums, nil
}

func (mb *MusicBrainzClient) SearchArtist(ctx context.Context, artistName string) (mbid string, matchedName string, err error) {
	type artistEntry struct {
		ID             string
		Name           string
		Score          int
		Type           string
		Disambiguation string
	}

	doSearch := func(query string) ([]artistEntry, error) {
		reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/artist/?query=%s&fmt=json&limit=10",
			url.QueryEscape(query))
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", mb.userAgent)
		resp, err := mb.httpClient.Do(req)
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
		var entries []artistEntry
		for _, a := range result.Artists {
			entries = append(entries, artistEntry{a.ID, a.Name, a.Score, a.Type, a.Disambiguation})
		}
		return entries, nil
	}

	// Try artistphrase first
	artists, err := doSearch(fmt.Sprintf("artistphrase:%s", artistName))
	if err != nil {
		return "", "", err
	}
	if len(artists) == 0 {
		// Fallback to quoted search
		artists, err = doSearch(fmt.Sprintf("artist:\"%s\"", artistName))
		if err != nil {
			return "", "", err
		}
	} else {
		// artistphrase returned results — check if any is an exact match
		hasExact := false
		for _, a := range artists {
			if strings.EqualFold(a.Name, artistName) {
				hasExact = true
				break
			}
		}
		if !hasExact {
			// Also try quoted search (handles edge cases like "The The")
			quoted, err2 := doSearch(fmt.Sprintf("artist:\"%s\"", artistName))
			if err2 == nil && len(quoted) > 0 {
				artists = append(quoted, artists...)
			}
		}
	}
	if len(artists) == 0 {
		return "", "", nil
	}

	artistLower := strings.ToLower(artistName)
	artistNorm := normalizeForCompare(artistLower)

	// Pass 1: exact match
	for _, a := range artists {
		if strings.ToLower(a.Name) == artistLower {
			return a.ID, a.Name, nil
		}
	}

	// Pass 2: normalized match
	for _, a := range artists {
		if normalizeForCompare(strings.ToLower(a.Name)) == artistNorm {
			return a.ID, a.Name, nil
		}
	}

	// Pass 3: forward contains (result name contains query)
	for _, a := range artists {
		nameLower := strings.ToLower(a.Name)
		if strings.Contains(nameLower, artistLower) {
			return a.ID, a.Name, nil
		}
	}

	// Pass 4: first Person/Group
	for _, a := range artists {
		if a.Type == "Person" || a.Type == "Group" {
			return a.ID, a.Name, nil
		}
	}

	// Fallback to first result
	return artists[0].ID, artists[0].Name, nil
}
