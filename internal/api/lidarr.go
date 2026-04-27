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

// LidarrClient provides access to the Lidarr API.
// Requires base URL and API key from config.
type LidarrClient struct {
	baseURL    string
	apiKey     string
	enabled    bool
	httpClient *http.Client
}

// LidarrArtistStatus represents an artist's status in Lidarr
type LidarrArtistStatus struct {
	InLidarr   bool   // true if artist exists in Lidarr
	Monitored  bool   // true if artist is monitored
	ArtistID   int    // Lidarr's internal artist ID
	ArtistName string // Lidarr's matched artist name
	Error      string // error message if lookup failed
}

// LidarrAlbumStatus represents an album's status in Lidarr
type LidarrAlbumStatus struct {
	InLidarr        bool    // true if album exists in Lidarr
	Monitored       bool    // true if album is monitored
	HasFiles        bool    // true if any track files exist on disk
	PercentOfTracks float64 // percentage of tracks downloaded (0–100)
}

// NewLidarrClient creates a new Lidarr API client.
// baseURL should be something like "http://localhost:8686"
// apiKey is the Lidarr API key from Settings > General
func NewLidarrClient(baseURL, apiKey string, enabled bool) *LidarrClient {
	// Normalize URL (remove trailing slash)
	if strings.HasSuffix(baseURL, "/") {
		baseURL = strings.TrimSuffix(baseURL, "/")
	}

	return &LidarrClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		enabled:    enabled,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// IsConfigured returns true if the client is enabled and has URL and API key set
func (lc *LidarrClient) IsConfigured() bool {
	return lc.enabled && lc.baseURL != "" && lc.apiKey != ""
}

// makeRequest creates an authenticated HTTP request
func (lc *LidarrClient) makeRequest(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, lc.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", lc.apiKey)
	req.Header.Set("Accept", "application/json")

	return lc.httpClient.Do(req)
}

// GetArtistByMBID looks up an artist in Lidarr by MusicBrainz ID.
// Returns artist status with ID and monitored flag.
func (lc *LidarrClient) GetArtistByMBID(ctx context.Context, mbid string) (*LidarrArtistStatus, error) {
	if !lc.IsConfigured() {
		return &LidarrArtistStatus{InLidarr: false}, nil
	}

	// Use the /api/v1/artist endpoint with mbId query parameter
	reqURL := fmt.Sprintf("/api/v1/artist?mbId=%s", url.PathEscape(mbid))
	resp, err := lc.makeRequest(ctx, "GET", reqURL)
	if err != nil {
		return &LidarrArtistStatus{Error: err.Error()}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return &LidarrArtistStatus{Error: "invalid API key"}, fmt.Errorf("lidarr: unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return &LidarrArtistStatus{Error: fmt.Sprintf("status %d", resp.StatusCode)}, fmt.Errorf("lidarr: status %d", resp.StatusCode)
	}

	var artists []struct {
		ID         int    `json:"id"`
		ArtistName string `json:"artistName"`
		Monitored  bool   `json:"monitored"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&artists); err != nil {
		return &LidarrArtistStatus{Error: err.Error()}, err
	}

	if len(artists) == 0 {
		return &LidarrArtistStatus{InLidarr: false}, nil
	}

	// Return first match (should be exactly one for MBID lookup)
	return &LidarrArtistStatus{
		InLidarr:   true,
		Monitored:  artists[0].Monitored,
		ArtistID:   artists[0].ID,
		ArtistName: artists[0].ArtistName,
	}, nil
}

// GetArtistAlbums returns a map of album titles to their Lidarr status.
// Album titles are matched against MusicBrainz release group titles.
func (lc *LidarrClient) GetArtistAlbums(ctx context.Context, artistID int, mbAlbumTitles []string) (map[string]*LidarrAlbumStatus, error) {
	if !lc.IsConfigured() || artistID == 0 {
		return nil, nil
	}

	// Fetch all albums for this artist
	reqURL := fmt.Sprintf("/api/v1/album?artistId=%d&includeAllArtistAlbums=true", artistID)
	resp, err := lc.makeRequest(ctx, "GET", reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lidarr: album status %d", resp.StatusCode)
	}

	var albums []struct {
		Title      string `json:"title"`
		Monitored  bool   `json:"monitored"`
		Statistics struct {
			TrackFileCount  int     `json:"trackFileCount"`
			TrackCount      int     `json:"trackCount"`
			PercentOfTracks float64 `json:"percentOfTracks"`
		} `json:"statistics"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, err
	}

	// Build lookup map from Lidarr album titles (lowercase for comparison)
	type albumInfo struct {
		monitored       bool
		hasFiles        bool
		percentOfTracks float64
	}
	lidarrLookup := make(map[string]albumInfo)
	for _, a := range albums {
		key := strings.ToLower(strings.TrimSpace(a.Title))
		lidarrLookup[key] = albumInfo{
			monitored:       a.Monitored,
			hasFiles:        a.Statistics.TrackFileCount > 0,
			percentOfTracks: a.Statistics.PercentOfTracks,
		}
	}

	// Match against MB album titles
	result := make(map[string]*LidarrAlbumStatus)
	for _, title := range mbAlbumTitles {
		key := strings.ToLower(strings.TrimSpace(title))
		info, inLidarr := lidarrLookup[key]
		result[title] = &LidarrAlbumStatus{
			InLidarr:        inLidarr,
			Monitored:       info.monitored,
			HasFiles:        info.hasFiles,
			PercentOfTracks: info.percentOfTracks,
		}
	}

	return result, nil
}

// OpenArtistURL returns the URL to open an artist page in Lidarr's web UI.
// Lidarr v3+ uses the MusicBrainz ID (foreignArtistId) in web UI routes,
// not the internal numeric database ID.
func (lc *LidarrClient) OpenArtistURL(mbid string) string {
	return fmt.Sprintf("%s/artist/%s", lc.baseURL, url.PathEscape(mbid))
}

// OpenSearchURL returns the URL to open Lidarr's add/search page
func (lc *LidarrClient) OpenSearchURL(searchTerm string) string {
	return fmt.Sprintf("%s/add/search?term=%s", lc.baseURL, url.PathEscape(searchTerm))
}

// OpenSearchByMBID returns the URL to open Lidarr's add/search page with MBID prefix
func (lc *LidarrClient) OpenSearchByMBID(mbid string) string {
	return lc.OpenSearchURL("mbid:" + mbid)
}
