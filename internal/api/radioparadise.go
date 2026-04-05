// Package api provides clients for external APIs
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"rptui-bubbletea/internal/models"
)

// ConnErrorType classifies connection errors for display
type ConnErrorType int

const (
	ConnErrorNetwork ConnErrorType = iota // No internet, DNS failure, etc.
	ConnErrorTimeout                      // Request timed out
	ConnErrorServer                       // RP server error (5xx)
	ConnErrorOther                        // Unknown error
)

// ClassifyConnError determines the type of connection error for user-facing messages
func ClassifyConnError(err error) ConnErrorType {
	if err == nil {
		return ConnErrorOther
	}

	// Check for timeout via net.Error interface
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ConnErrorTimeout
	}

	errStr := err.Error()

	// Network-level failures: DNS, unreachable, refused, reset
	for _, s := range []string{
		"no such host",
		"network is unreachable",
		"connection refused",
		"connection reset",
		"no route to host",
		"temporary failure in name resolution",
	} {
		if strings.Contains(errStr, s) {
			return ConnErrorNetwork
		}
	}

	// HTTP 5xx server errors
	if strings.Contains(errStr, "status 5") {
		return ConnErrorServer
	}

	return ConnErrorOther
}

// RadioParadiseAPI handles all Radio Paradise API interactions
type RadioParadiseAPI struct {
	channel    int
	bitrate    int
	baseURL    string
	imageBase  string
	httpClient *http.Client
}

// NewRadioParadiseAPI creates a new Radio Paradise API client
func NewRadioParadiseAPI(channel, bitrate int) *RadioParadiseAPI {
	return &RadioParadiseAPI{
		channel:   channel,
		bitrate:   bitrate,
		baseURL:   "https://api.radioparadise.com/api",
		imageBase: "https://img.radioparadise.com/",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NowPlayingResponse represents the /now_playing API response
type NowPlayingResponse struct {
	Song        map[string]any `json:"song"`
	ImageBase   string         `json:"image_base"`
	Bitrate     string         `json:"bitrate"`
	Channel     string         `json:"channel"`
	PlayTime    string         `json:"play_time"`
	Elapsed     int            `json:"elapsed"`
	TrackTotal  int            `json:"track_total"`
	TrackNumber int            `json:"track_number"`
}

// GetNowPlaying fetches current song info
func (r *RadioParadiseAPI) GetNowPlaying(ctx context.Context) (*NowPlayingResponse, error) {
	url := fmt.Sprintf("%s/now_playing?chan=%d", r.baseURL, r.channel)

	resp, err := r.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch now playing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result NowPlayingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// PlaylistResponse represents the /nowplaying_list_v2022 API response
type PlaylistResponse struct {
	Songs     []map[string]any `json:"song"`
	ImageBase string           `json:"image_base"`
	Bitrate   string           `json:"bitrate"`
	Channel   string           `json:"channel"`
}

// GetPlaylist fetches extended playlist with timing
func (r *RadioParadiseAPI) GetPlaylist(ctx context.Context) (*PlaylistResponse, error) {
	url := fmt.Sprintf("%s/nowplaying_list_v2022?chan=%d", r.baseURL, r.channel)

	resp, err := r.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result PlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ChannelInfo represents channel information from the API
type ChannelInfo struct {
	Chan       string `json:"chan"`
	Title      string `json:"title"`
	StreamName string `json:"stream_name"`
	IsER       bool   `json:"isER"`
}

// BlockResponse represents the /play API response
type BlockResponse struct {
	Song      map[string]map[string]any `json:"song"`
	ImageBase string                    `json:"image_base"`
	Bitrate   string                    `json:"bitrate"`
	Channel   ChannelInfo               `json:"channel"`
	Event     string                    `json:"event"`
	Elapsed   int                       `json:"elapsed"`
	BlockID   string                    `json:"block_id"`
	SliceNum  string                    `json:"slice_num"`
	SchedTime int64                     `json:"sched_time_millis"`
}

// GetBlock fetches audio block with playback URLs
func (r *RadioParadiseAPI) GetBlock(ctx context.Context) (*BlockResponse, error) {
	url := fmt.Sprintf(
		"%s/play?event=0&elapsed=1&bitrate=%d&action=start&info=true&chan=%d",
		r.baseURL, r.bitrate, r.channel,
	)

	resp, err := r.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result BlockResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetFileExtension returns file extension based on bitrate setting
func (r *RadioParadiseAPI) GetFileExtension() string {
	if r.bitrate <= 3 {
		return "m4a"
	} else if r.bitrate == 4 {
		return "flac"
	}
	return "mp3"
}

// ParseBlockSongs parses block data to extract songs with gapless URLs and timing
// Returns songs in play order (songs[0] = currently playing/next song)
func (r *RadioParadiseAPI) ParseBlockSongs(blockData *BlockResponse) ([]*models.Song, string) {
	var songs []*models.Song
	imageBase := blockData.ImageBase
	if imageBase == "" {
		imageBase = r.imageBase
	}

	blockSongs := blockData.Song

	// Sort keys numerically and extract songs
	keys := make([]int, 0, len(blockSongs))
	for key := range blockSongs {
		var k int
		fmt.Sscanf(key, "%d", &k)
		keys = append(keys, k)
	}

	// Simple bubble sort for small arrays
	for i := 0; i < len(keys)-1; i++ {
		for j := 0; j < len(keys)-i-1; j++ {
			if keys[j] > keys[j+1] {
				keys[j], keys[j+1] = keys[j+1], keys[j]
			}
		}
	}

	for _, key := range keys {
		songData := blockSongs[fmt.Sprintf("%d", key)]
		song := models.NewSong(songData, imageBase)
		songs = append(songs, song)
	}

	return songs, imageBase
}

// SetChannel sets the channel (station)
func (r *RadioParadiseAPI) SetChannel(channel int) {
	r.channel = channel
}

// SetBitrate sets the bitrate
func (r *RadioParadiseAPI) SetBitrate(bitrate int) {
	r.bitrate = bitrate
}

// GetChannel returns the current channel
func (r *RadioParadiseAPI) GetChannel() int {
	return r.channel
}

// ListChannelsResponse represents the /list_chan API response
type ListChannelsResponse []struct {
	Chan         string `json:"chan"`
	Title        string `json:"title"`
	StreamName   string `json:"stream_name"`
	Downloadable bool   `json:"downloadable"`
}

// ListChannels fetches all available channels from RP
func (r *RadioParadiseAPI) ListChannels(ctx context.Context) (ListChannelsResponse, error) {
	url := fmt.Sprintf("%s/list_chan", r.baseURL)

	resp, err := r.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch channels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result ListChannelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// GetBitrate returns the current bitrate
func (r *RadioParadiseAPI) GetBitrate() int {
	return r.bitrate
}
