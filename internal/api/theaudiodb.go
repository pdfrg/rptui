package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// TheAudioDBClient provides access to TheAudioDB API
type TheAudioDBClient struct {
	httpClient *http.Client
}

// TADBArtist holds artist data fetched from TheAudioDB
type TADBArtist struct {
	Name      string
	Bio       string
	Thumb     string
	FanArts   []string
	AlbumInfo *TADBAlbumInfo // Album info for disambiguation
}

// TADBAlbumInfo holds album info from album search
type TADBAlbumInfo struct {
	Description string // Album description/blurb
	Sales       string // Sales figures (e.g., "14300000")
}

// NewTheAudioDBClient creates a new TheAudioDB API client
func NewTheAudioDBClient() *TheAudioDBClient {
	return &TheAudioDBClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchArtist searches for an artist on TheAudioDB.
// If albumName is provided, it uses album search to disambiguate when multiple artists match.
// Returns nil if the artist is not found.
func (t *TheAudioDBClient) SearchArtist(ctx context.Context, artistName, albumName string) (*TADBArtist, error) {
	// Try album search first for disambiguation (if album provided)
	var albumInfo *TADBAlbumInfo
	if albumName != "" {
		reqURL := fmt.Sprintf("https://www.theaudiodb.com/api/v1/json/123/searchalbum.php?s=%s&a=%s",
			url.QueryEscape(artistName), url.QueryEscape(albumName))

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, err
		}

		resp, err := t.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var albumResult struct {
				Album []struct {
					IDArtist    string `json:"idArtist"`
					StrAlbum    string `json:"strAlbum"`
					Description string `json:"strDescription"`
					Sales       string `json:"intSales"`
				} `json:"album"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&albumResult); err == nil {
				if len(albumResult.Album) > 0 {
					alb := albumResult.Album[0]
					albumInfo = &TADBAlbumInfo{
						Description: alb.Description,
						Sales:       alb.Sales,
					}
					// Use artist ID from album search to get correct artist
					if alb.IDArtist != "" {
						return t.fetchArtistByID(ctx, alb.IDArtist, albumInfo)
					}
				}
			}
		}
	}

	// Fallback to standard artist search
	reqURL := fmt.Sprintf("https://www.theaudiodb.com/api/v1/json/123/search.php?s=%s",
		url.QueryEscape(artistName))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("theaudiodb status %d", resp.StatusCode)
	}

	var result struct {
		Artists []struct {
			IDArtist         string `json:"idArtist"`
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
		return nil, err
	}

	if len(result.Artists) == 0 {
		return nil, nil // not found
	}

	art := result.Artists[0]
	var fanArts []string
	for _, fa := range []string{art.StrArtistFanart, art.StrArtistFanart2, art.StrArtistFanart3, art.StrArtistFanart4} {
		if fa != "" {
			fanArts = append(fanArts, fa)
		}
	}

	return &TADBArtist{
		Name:      art.StrArtist,
		Bio:       art.StrBiography,
		Thumb:     art.StrArtistThumb,
		FanArts:   fanArts,
		AlbumInfo: albumInfo,
	}, nil
}

// fetchArtistByID fetches artist details by ID, optionally with album info
func (t *TheAudioDBClient) fetchArtistByID(ctx context.Context, artistID string, albumInfo *TADBAlbumInfo) (*TADBArtist, error) {
	reqURL := fmt.Sprintf("https://www.theaudiodb.com/api/v1/json/123/artist.php?i=%s", artistID)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("theaudiodb artist lookup status %d", resp.StatusCode)
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
		return nil, err
	}

	if len(result.Artists) == 0 {
		return nil, nil
	}

	art := result.Artists[0]
	var fanArts []string
	for _, fa := range []string{art.StrArtistFanart, art.StrArtistFanart2, art.StrArtistFanart3, art.StrArtistFanart4} {
		if fa != "" {
			fanArts = append(fanArts, fa)
		}
	}

	return &TADBArtist{
		Name:      art.StrArtist,
		Bio:       art.StrBiography,
		Thumb:     art.StrArtistThumb,
		FanArts:   fanArts,
		AlbumInfo: albumInfo,
	}, nil
}
