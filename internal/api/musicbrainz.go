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
// Returns nil if the artist is not found.
func (mb *MusicBrainzClient) GetDiscography(ctx context.Context, artistName string) ([]MBAlbum, error) {
	// Step 1: Search for artist
	mbID, _, err := mb.searchArtist(ctx, artistName)
	if err != nil {
		return nil, err
	}
	if mbID == "" {
		return nil, nil // not found
	}

	// Respect MB rate limit between calls
	time.Sleep(1 * time.Second)

	// Step 2: Fetch release groups — official studio albums only
	// primarytype:Album matches the MB website "Album" filter
	// NOT secondarytype:* excludes anything with a secondary type (compilation, live, remix, etc.)
	mbQuery := fmt.Sprintf("arid:%s AND primarytype:Album NOT secondarytype:*", mbID)
	reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/release-group/?query=%s&fmt=json&limit=100",
		strings.ReplaceAll(mbQuery, " ", "%20"))

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
		return nil, fmt.Errorf("musicbrainz release-groups status %d", resp.StatusCode)
	}

	var rgResult struct {
		ReleaseGroups []struct {
			Title            string `json:"title"`
			FirstReleaseDate string `json:"first-release-date"`
		} `json:"release-groups"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rgResult); err != nil {
		return nil, err
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

	var albums []MBAlbum
	for _, e := range entries {
		albums = append(albums, MBAlbum{Title: e.title, Year: e.year})
	}

	return albums, nil
}

func (mb *MusicBrainzClient) searchArtist(ctx context.Context, artistName string) (mbid string, matchedName string, err error) {
	var searchResult struct {
		Artists []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			Score          int    `json:"score"`
			Type           string `json:"type"`
			Disambiguation string `json:"disambiguation"`
		} `json:"artists"`
	}

	doSearch := func(query string) error {
		reqURL := fmt.Sprintf("https://musicbrainz.org/ws/2/artist/?query=%s&fmt=json&limit=10",
			url.QueryEscape(query))
		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", mb.userAgent)
		resp, err := mb.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(&searchResult)
	}

	// Try artistphrase first
	if err := doSearch(fmt.Sprintf("artistphrase:%s", artistName)); err != nil {
		return "", "", err
	}
	if len(searchResult.Artists) == 0 {
		// Fallback to quoted search
		if err := doSearch(fmt.Sprintf("artist:\"%s\"", artistName)); err != nil {
			return "", "", err
		}
	}
	if len(searchResult.Artists) == 0 {
		return "", "", nil
	}

	artistLower := strings.ToLower(artistName)
	artistNorm := normalizeForCompare(artistLower)

	// Pass 1: exact match
	for _, a := range searchResult.Artists {
		if strings.ToLower(a.Name) == artistLower {
			return a.ID, a.Name, nil
		}
	}

	// Pass 2: normalized match
	for _, a := range searchResult.Artists {
		if normalizeForCompare(strings.ToLower(a.Name)) == artistNorm {
			return a.ID, a.Name, nil
		}
	}

	// Pass 3: contains
	for _, a := range searchResult.Artists {
		nameLower := strings.ToLower(a.Name)
		if strings.Contains(nameLower, artistLower) || strings.Contains(artistLower, nameLower) {
			return a.ID, a.Name, nil
		}
	}

	// Pass 4: first Person/Group
	for _, a := range searchResult.Artists {
		if a.Type == "Person" || a.Type == "Group" {
			return a.ID, a.Name, nil
		}
	}

	// Fallback to first result
	return searchResult.Artists[0].ID, searchResult.Artists[0].Name, nil
}
