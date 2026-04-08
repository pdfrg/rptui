// Package api provides clients for external API
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// RPAuthClient handles Radio Paradise authentication
type RPAuthClient struct {
	username     string
	password     string
	cPasswd      string
	userID       string
	cValidated   string
	chan99Cutoff int
	httpClient   *http.Client
}

// RPAuthResponse represents the /api/auth JSON response
type RPAuthResponse struct {
	Status      string `json:"status"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
	Passwd      string `json:"passwd"`
	Level       string `json:"level"`
	CountryCode string `json:"country_code"`
}

// RPAuthState represents persisted session tokens
type RPAuthState struct {
	CPasswd      string `toml:"c_passwd"`
	UserID       string `toml:"c_user_id"`
	CValidated   string `toml:"c_validated"`
	Chan99Cutoff int    `toml:"chan_99_cutoff"`
}

// NewRPAuthClient creates a new auth client (unauthenticated until Login or LoadState)
func NewRPAuthClient() *RPAuthClient {
	return &RPAuthClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Login authenticates with RP using username/password and stores session tokens
func (a *RPAuthClient) Login(username, password string) error {
	url := fmt.Sprintf("https://api.radioparadise.com/api/auth?username=%s&passwd=%s",
		username, password)

	resp, err := a.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	var authResp RPAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	if authResp.Status != "success" {
		return fmt.Errorf("authentication failed: %s", authResp.Status)
	}

	a.username = authResp.Username
	a.password = password
	a.cPasswd = authResp.Passwd
	a.userID = authResp.UserID
	a.cValidated = "yes"

	return nil
}

// HasAuth returns true if valid session tokens are loaded
func (a *RPAuthClient) HasAuth() bool {
	return a.cPasswd != "" && a.userID != ""
}

// CookieString returns the cookie header value for authenticated requests
func (a *RPAuthClient) CookieString() string {
	return fmt.Sprintf("player_id=rptui; C_username=%s; C_passwd=%s; C_validated=%s; C_user_id=%s",
		a.username, a.cPasswd, a.cValidated, a.userID)
}

// Username returns the authenticated username
func (a *RPAuthClient) Username() string {
	return a.username
}

// UserID returns the authenticated user ID
func (a *RPAuthClient) UserID() string {
	return a.userID
}

// SetCredentials sets credentials for later Login call
func (a *RPAuthClient) SetCredentials(username, password string) {
	a.username = username
	a.password = password
}

// SaveState persists session tokens to disk
func (a *RPAuthClient) SaveState(path string) error {
	state := RPAuthState{
		CPasswd:      a.cPasswd,
		UserID:       a.userID,
		CValidated:   a.cValidated,
		Chan99Cutoff: a.chan99Cutoff,
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create auth state directory: %w", err)
	}

	data, err := toml.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal auth state: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write auth state: %w", err)
	}

	return nil
}

// LoadState restores session tokens from disk
func (a *RPAuthClient) LoadState(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read auth state: %w", err)
	}

	var state RPAuthState
	if err := toml.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse auth state: %w", err)
	}

	a.cPasswd = state.CPasswd
	a.userID = state.UserID
	a.cValidated = state.CValidated
	a.chan99Cutoff = state.Chan99Cutoff

	return nil
}

// Reauth attempts to re-authenticate using stored credentials
func (a *RPAuthClient) Reauth() error {
	if a.username == "" || a.password == "" {
		return fmt.Errorf("no stored credentials for re-authentication")
	}
	return a.Login(a.username, a.password)
}

// SetChan99Cutoff stores the RP favorites rating threshold
func (a *RPAuthClient) SetChan99Cutoff(cutoff int) {
	a.chan99Cutoff = cutoff
}

// Chan99Cutoff returns the RP favorites rating threshold (default 7 if not set)
func (a *RPAuthClient) Chan99Cutoff() int {
	if a.chan99Cutoff < 1 {
		return 7
	}
	return a.chan99Cutoff
}
