// Package api provides clients for external API
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RatedSong represents a song from the user's profile favorites/ratings
type RatedSong struct {
	SongID                string  `json:"song_id"`
	Title                 string  `json:"title"`
	Artist                string  `json:"artist"`
	Album                 string  `json:"album"`
	AlbumID               string  `json:"album_id"`
	ASIN                  string  `json:"asin"`
	Year                  string  `json:"year"`
	Cover                 string  `json:"cover"`
	Rating                string  `json:"rating"`
	ListenerRating        float64 `json:"listener_rating"`
	ListenerRatingRounded int     `json:"listener_rating_rounded"`
	RatingsNum            string  `json:"ratings_num"`
}

// ProfileFavoritesResponse represents the account::profile-favorites API response
type ProfileFavoritesResponse struct {
	Songs    []RatedSong
	NumSongs int
}

// FavsCount represents the user's rating distribution
type FavsCount struct {
	R5  int `json:"r5"`
	R6  int `json:"r6"`
	R7  int `json:"r7"`
	R8  int `json:"r8"`
	R9  int `json:"r9"`
	R10 int `json:"r10"`
}

// FavsCountResponse represents the list_chan_favscount API response
type FavsCountResponse struct {
	FavsCount    FavsCount `json:"favsCount"`
	Chan99Cutoff int       `json:"chan_99_cutoff"`
}

// RatingResponse represents the /api/rating API response
type RatingResponse struct {
	Status string `json:"status"`
	SongID int64  `json:"song_id"`
	UserID string `json:"user_id"`
	Rating int    `json:"rating"`
}

// RPRatingsClient handles RP ratings and favorites API interactions
type RPRatingsClient struct {
	authClient *RPAuthClient
	httpClient *http.Client
}

// NewRPRatingsClient creates a new ratings/favorites API client
func NewRPRatingsClient(authClient *RPAuthClient) *RPRatingsClient {
	return &RPRatingsClient{
		authClient: authClient,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SubmitRating submits a rating (1-10) for a song
func (c *RPRatingsClient) SubmitRating(songID int64, rating int) (*RatingResponse, error) {
	if c.authClient == nil || !c.authClient.HasAuth() {
		return nil, fmt.Errorf("not authenticated")
	}

	if rating < 1 || rating > 10 {
		return nil, fmt.Errorf("rating must be between 1 and 10")
	}

	url := fmt.Sprintf("https://api.radioparadise.com/api/rating?song_id=%d&rating=%d", songID, rating)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Cookie", c.authClient.CookieString())
	req.Header.Set("User-Agent", "rptui/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit rating: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result RatingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode rating response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("rating submission failed: %s", result.Status)
	}

	return &result, nil
}

// GetProfileFavorites fetches the user's rated songs (paginated)
// mode: "High" for ratings >= lowerLimit, "Low" for ratings <= upperLimit
// lowerLimit/upperLimit: rating range filter (1-10)
// offset: pagination offset (returns 20 songs per page)
func (c *RPRatingsClient) GetProfileFavorites(userID, mode string, lowerLimit, upperLimit, offset int) (*ProfileFavoritesResponse, error) {
	if c.authClient == nil || !c.authClient.HasAuth() {
		return nil, fmt.Errorf("not authenticated")
	}

	url := fmt.Sprintf("https://api.radioparadise.com/siteapi.php?file=account%%3A%%3Aprofile-favorites&profile_user_id=%s&mode=%s&lower_limit=%d&upper_limit=%d&list_offset=%d",
		userID, mode, lowerLimit, upperLimit, offset)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Cookie", c.authClient.CookieString())
	req.Header.Set("User-Agent", "rptui/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profile favorites: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var raw struct {
		Songs    []RatedSong `json:"songs"`
		NumSongs int         `json:"num_songs"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ProfileFavoritesResponse{
		Songs:    raw.Songs,
		NumSongs: raw.NumSongs,
	}, nil
}

// GetAllProfileFavorites fetches all user-rated songs across all pages
func (c *RPRatingsClient) GetAllProfileFavorites(userID, mode string, lowerLimit, upperLimit int) ([]RatedSong, error) {
	var allSongs []RatedSong
	offset := 0

	for {
		resp, err := c.GetProfileFavorites(userID, mode, lowerLimit, upperLimit, offset)
		if err != nil {
			return nil, err
		}

		allSongs = append(allSongs, resp.Songs...)

		if len(resp.Songs) == 0 || len(allSongs) >= resp.NumSongs {
			break
		}

		offset += len(resp.Songs)
	}

	return allSongs, nil
}

// GetFavsCount fetches the user's rating distribution and channel 99 cutoff
func (c *RPRatingsClient) GetFavsCount() (*FavsCountResponse, error) {
	if c.authClient == nil || !c.authClient.HasAuth() {
		return nil, fmt.Errorf("not authenticated")
	}

	url := "https://api.radioparadise.com/api/list_chan_favscount?source=24"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Cookie", c.authClient.CookieString())
	req.Header.Set("User-Agent", "rptui/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch favs count: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result FavsCountResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
