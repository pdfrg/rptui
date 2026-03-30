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
	Name    string
	Bio     string
	Thumb   string
	FanArts []string
}

// NewTheAudioDBClient creates a new TheAudioDB API client
func NewTheAudioDBClient() *TheAudioDBClient {
	return &TheAudioDBClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchArtist searches for an artist on TheAudioDB.
// Returns nil if the artist is not found.
func (t *TheAudioDBClient) SearchArtist(ctx context.Context, artistName string) (*TADBArtist, error) {
	reqURL := fmt.Sprintf("https://theaudiodb.com/api/v1/json/123/search.php?s=%s",
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
		Name:    art.StrArtist,
		Bio:     art.StrBiography,
		Thumb:   art.StrArtistThumb,
		FanArts: fanArts,
	}, nil
}
