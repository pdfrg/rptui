package api

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/models"
)

var scrobbleLogger *log.Logger

func init() {
	scrobbleLogger = loginit.InitLogger("[SCROBBLE] ")
}

// LastFMAPIKey and LastFMSharedSecret are injected at build time via -ldflags.
// For development, register your own Last.fm API app and set these.
var LastFMAPIKey = ""
var LastFMSharedSecret = ""

// ScrobbleClient defines the interface for scrobble service clients.
type ScrobbleClient interface {
	// SendNowPlaying notifies the service that a track has started playing.
	SendNowPlaying(ctx context.Context, song models.Song) error
	// Scrobble submits a completed listen to the service.
	Scrobble(ctx context.Context, song models.Song, startTime time.Time) error
	// Enabled returns true if this client is configured and ready.
	Enabled() bool
	// Name returns the display name of this service (e.g. "last.fm", "listenbrainz").
	Name() string
	// ShortName returns a short display name (e.g. "fm", "lb").
	ShortName() string
}

// Scrobbler manages all configured scrobble clients.
type Scrobbler struct {
	clients []ScrobbleClient
	cache   *ScrobbleCache
}

// NewScrobbler creates a new Scrobbler from the given config.
func NewScrobbler(cfg *config.Config) *Scrobbler {
	cacheDir := config.GetScrobbleCacheDir()
	cache := NewScrobbleCache(cacheDir)

	var clients []ScrobbleClient

	if cfg.LastFM.Enabled {
		client := NewLastFMClient(LastFMAPIKey, LastFMSharedSecret, cfg.LastFM.SessionKey)
		if client.Enabled() {
			clients = append(clients, client)
			scrobbleLogger.Printf("Last.fm enabled")
		} else {
			scrobbleLogger.Printf("Last.fm configured but API key/secret missing (rebuild with -ldflags)")
		}
	}
	if cfg.ListenBrainz.Enabled && cfg.ListenBrainz.Token != "" {
		client := NewListenBrainzClient(cfg.ListenBrainz.Token)
		clients = append(clients, client)
		scrobbleLogger.Printf("ListenBrainz enabled")
	}

	return &Scrobbler{
		clients: clients,
		cache:   cache,
	}
}

// Enabled returns true if any scrobble client is configured.
func (s *Scrobbler) Enabled() bool {
	return len(s.clients) > 0
}

// ServiceNames returns display names of all enabled services.
func (s *Scrobbler) ServiceNames() []string {
	var names []string
	for _, c := range s.clients {
		names = append(names, c.ShortName())
	}
	return names
}

// SendNowPlaying sends now-playing notification to all enabled clients.
func (s *Scrobbler) SendNowPlaying(ctx context.Context, song models.Song) {
	for _, c := range s.clients {
		go func(client ScrobbleClient) {
			if err := client.SendNowPlaying(ctx, song); err != nil {
				scrobbleLogger.Printf("NowPlaying %s failed: %v", client.Name(), err)
			}
		}(c)
	}
}

// Scrobble submits a listen to all enabled clients. Failed submissions are cached.
func (s *Scrobbler) Scrobble(ctx context.Context, song models.Song, startTime time.Time) {
	// First, drain any cached scrobbles
	s.drainCache(ctx)

	for _, c := range s.clients {
		go func(client ScrobbleClient) {
			if err := client.Scrobble(ctx, song, startTime); err != nil {
				scrobbleLogger.Printf("Scrobble %s failed for %q: %v", client.Name(), song.Title, err)
				// Cache the failed scrobble
				entry := ScrobbleEntry{
					Artist:       song.Artist,
					Track:        song.Title,
					Album:        song.Album,
					DurationSecs: int(song.Duration / 1000),
					Timestamp:    startTime.Unix(),
					Service:      client.ShortName(),
				}
				if cacheErr := s.cache.Add(entry); cacheErr != nil {
					scrobbleLogger.Printf("Failed to cache scrobble: %v", cacheErr)
				}
			} else {
				scrobbleLogger.Printf("Scrobbled %q to %s", song.Title, client.Name())
			}
		}(c)
	}
}

// ScrobbleResult holds the outcome of a scrobble attempt for UI display.
type ScrobbleResult struct {
	Service string
	Success bool
}

// ScrobbleWithResult is like Scrobble but returns results for each service.
func (s *Scrobbler) ScrobbleWithResult(ctx context.Context, song models.Song, startTime time.Time) []ScrobbleResult {
	s.drainCache(ctx)

	results := make([]ScrobbleResult, len(s.clients))
	for i, c := range s.clients {
		err := c.Scrobble(ctx, song, startTime)
		results[i] = ScrobbleResult{
			Service: c.ShortName(),
			Success: err == nil,
		}
		if err != nil {
			scrobbleLogger.Printf("Scrobble %s failed for %q: %v", c.Name(), song.Title, err)
			entry := ScrobbleEntry{
				Artist:       song.Artist,
				Track:        song.Title,
				Album:        song.Album,
				DurationSecs: int(song.Duration / 1000),
				Timestamp:    startTime.Unix(),
				Service:      c.ShortName(),
			}
			if cacheErr := s.cache.Add(entry); cacheErr != nil {
				scrobbleLogger.Printf("Failed to cache scrobble: %v", cacheErr)
			}
		} else {
			scrobbleLogger.Printf("Scrobbled %q to %s", song.Title, c.Name())
		}
	}
	return results
}

// drainCache attempts to send any cached scrobbles to their respective services.
func (s *Scrobbler) drainCache(ctx context.Context) {
	entries := s.cache.Load()
	if len(entries) == 0 {
		return
	}

	scrobbleLogger.Printf("Draining %d cached scrobbles", len(entries))
	var remaining []ScrobbleEntry

	for _, entry := range entries {
		sent := false
		for _, c := range s.clients {
			if c.ShortName() != entry.Service {
				continue
			}
			startTime := time.Unix(entry.Timestamp, 0).UTC()
			song := models.Song{
				Artist:   entry.Artist,
				Title:    entry.Track,
				Album:    entry.Album,
				Duration: int64(entry.DurationSecs) * 1000,
			}
			if err := c.Scrobble(ctx, song, startTime); err != nil {
				scrobbleLogger.Printf("Cached scrobble retry failed for %s: %v", c.Name(), err)
			} else {
				sent = true
			}
			break
		}
		if !sent {
			remaining = append(remaining, entry)
		}
	}

	if err := s.cache.Replace(remaining); err != nil {
		scrobbleLogger.Printf("Failed to update scrobble cache: %v", err)
	}
}

// --- Last.fm Client ---

// LastFMClient implements ScrobbleClient for Last.fm.
type LastFMClient struct {
	apiKey       string
	sharedSecret string
	sessionKey   string
	httpClient   *http.Client
}

// NewLastFMClient creates a new Last.fm scrobble client.
func NewLastFMClient(apiKey, sharedSecret, sessionKey string) *LastFMClient {
	return &LastFMClient{
		apiKey:       apiKey,
		sharedSecret: sharedSecret,
		sessionKey:   sessionKey,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *LastFMClient) Enabled() bool {
	return c.apiKey != "" && c.sharedSecret != "" && c.sessionKey != ""
}

func (c *LastFMClient) Name() string      { return "last.fm" }
func (c *LastFMClient) ShortName() string { return "fm" }

func (c *LastFMClient) SendNowPlaying(ctx context.Context, song models.Song) error {
	if !c.Enabled() {
		return nil
	}
	params := map[string]string{
		"method":  "track.updateNowPlaying",
		"artist":  song.Artist,
		"track":   song.Title,
		"album":   song.Album,
		"api_key": c.apiKey,
		"sk":      c.sessionKey,
	}
	if song.Duration > 0 {
		params["duration"] = strconv.FormatInt(song.Duration/1000, 10)
	}
	return c.postSigned(params)
}

func (c *LastFMClient) Scrobble(ctx context.Context, song models.Song, startTime time.Time) error {
	if !c.Enabled() {
		return nil
	}
	params := map[string]string{
		"method":       "track.scrobble",
		"artist":       song.Artist,
		"track":        song.Title,
		"album":        song.Album,
		"timestamp":    strconv.FormatInt(startTime.Unix(), 10),
		"chosenByUser": "0", // radio stream, not user-chosen
		"api_key":      c.apiKey,
		"sk":           c.sessionKey,
	}
	if song.Duration > 0 {
		params["duration"] = strconv.FormatInt(song.Duration/1000, 10)
	}
	return c.postSigned(params)
}

// postSigned signs the params and POSTs to the Last.fm API.
func (c *LastFMClient) postSigned(params map[string]string) error {
	sig := c.sign(params)
	params["api_sig"] = sig
	params["format"] = "json"

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}

	resp, err := c.httpClient.PostForm("https://ws.audioscrobbler.com/2.0/", form)
	if err != nil {
		return fmt.Errorf("last.fm request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read last.fm response: %w", err)
	}

	// Check for error response
	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Error != 0 {
		// Error 9 = invalid session key, re-auth needed
		return fmt.Errorf("last.fm error %d: %s", result.Error, result.Message)
	}

	scrobbleLogger.Printf("Last.fm %s response: %s", params["method"], string(body))
	return nil
}

// sign creates an API method signature for Last.fm.
func (c *LastFMClient) sign(params map[string]string) string {
	// Sort parameter names alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the signature string: key1value1key2value2...secret
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(c.sharedSecret)

	hash := md5.Sum([]byte(sb.String()))
	return fmt.Sprintf("%x", hash)
}

// --- ListenBrainz Client ---

// ListenBrainzClient implements ScrobbleClient for ListenBrainz.
type ListenBrainzClient struct {
	token      string
	httpClient *http.Client
}

// NewListenBrainzClient creates a new ListenBrainz scrobble client.
func NewListenBrainzClient(token string) *ListenBrainzClient {
	return &ListenBrainzClient{
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ListenBrainzClient) Enabled() bool {
	return c.token != ""
}

func (c *ListenBrainzClient) Name() string      { return "listenbrainz" }
func (c *ListenBrainzClient) ShortName() string { return "lb" }

func (c *ListenBrainzClient) SendNowPlaying(ctx context.Context, song models.Song) error {
	if !c.Enabled() {
		return nil
	}
	payload := lbPayload{
		ListenType: "playing_now",
		Payload: []lbListen{
			{
				TrackMetadata: lbTrackMetadata{
					ArtistName:  song.Artist,
					TrackName:   song.Title,
					ReleaseName: song.Album,
				},
			},
		},
	}
	return c.submit(payload)
}

func (c *ListenBrainzClient) Scrobble(ctx context.Context, song models.Song, startTime time.Time) error {
	if !c.Enabled() {
		return nil
	}
	payload := lbPayload{
		ListenType: "single",
		Payload: []lbListen{
			{
				ListenedAt: startTime.Unix(),
				TrackMetadata: lbTrackMetadata{
					ArtistName:  song.Artist,
					TrackName:   song.Title,
					ReleaseName: song.Album,
				},
			},
		},
	}
	return c.submit(payload)
}

func (c *ListenBrainzClient) submit(payload lbPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal listenbrainz payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.listenbrainz.org/1/submit-listens", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create listenbrainz request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("listenbrainz request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("listenbrainz: invalid token")
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("listenbrainz status %d: %s", resp.StatusCode, string(respBody))
	}

	scrobbleLogger.Printf("ListenBrainz %s submitted ok", payload.ListenType)
	return nil
}

// ListenBrainz JSON types
type lbPayload struct {
	ListenType string     `json:"listen_type"`
	Payload    []lbListen `json:"payload"`
}

type lbListen struct {
	ListenedAt    int64           `json:"listened_at,omitempty"`
	TrackMetadata lbTrackMetadata `json:"track_metadata"`
}

type lbTrackMetadata struct {
	ArtistName  string `json:"artist_name"`
	TrackName   string `json:"track_name"`
	ReleaseName string `json:"release_name,omitempty"`
}
