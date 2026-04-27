package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// LastFMDoAuth runs the one-time Last.fm authorization flow.
// It opens the browser for the user to grant access, then fetches the session key.
// Returns the session key or an error.
func LastFMDoAuth() (string, error) {
	if LastFMAPIKey == "" || LastFMSharedSecret == "" {
		return "", fmt.Errorf("last.fm API key and secret not configured (build with -ldflags)")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Step 1: Get a request token
	token, err := lastFMGetToken(client)
	if err != nil {
		return "", fmt.Errorf("failed to get request token: %w", err)
	}
	fmt.Printf("Request token obtained: %s\n", token)

	// Step 2: Open browser for user authorization
	authURL := fmt.Sprintf("https://www.last.fm/api/auth/?api_key=%s&token=%s", LastFMAPIKey, token)
	fmt.Printf("Opening browser for authorization...\n")
	fmt.Printf("If the browser doesn't open, visit this URL:\n  %s\n\n", authURL)
	OpenBrowser(authURL)

	// Step 3: Poll for session key (user must authorize in browser first)
	fmt.Println("Waiting for authorization (press Ctrl+C to cancel)...")
	sessionKey, err := lastFMPollSession(client, token)
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	fmt.Println("Authorization successful!")
	return sessionKey, nil
}

func lastFMGetToken(client *http.Client) (string, error) {
	params := map[string]string{
		"method":  "auth.getToken",
		"api_key": LastFMAPIKey,
	}
	sig := signLastFMParams(params)
	params["api_sig"] = sig
	params["format"] = "json"

	u, _ := url.Parse("https://ws.audioscrobbler.com/2.0/")
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	resp, err := client.Get(u.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Token string `json:"token"`
		Error int    `json:"error"`
		Msg   string `json:"message"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}
	if result.Error != 0 {
		return "", fmt.Errorf("last.fm error %d: %s", result.Error, result.Msg)
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}
	return result.Token, nil
}

func lastFMPollSession(client *http.Client, token string) (string, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)

		params := map[string]string{
			"method":  "auth.getSession",
			"api_key": LastFMAPIKey,
			"token":   token,
		}
		sig := signLastFMParams(params)
		params["api_sig"] = sig
		params["format"] = "json"

		u, _ := url.Parse("https://ws.audioscrobbler.com/2.0/")
		q := u.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()

		resp, err := client.Get(u.String())
		if err != nil {
			fmt.Printf("  Retrying... (%v)\n", err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Session struct {
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"session"`
			Error   int    `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			continue
		}

		if result.Session.Key != "" {
			fmt.Printf("  Authorized as: %s\n", result.Session.Name)
			return result.Session.Key, nil
		}

		// Error 14 = token not authorized yet, keep polling
		if result.Error == 14 {
			fmt.Print(".")
			continue
		}

		// Other errors are fatal
		return "", fmt.Errorf("last.fm error %d: %s", result.Error, result.Message)
	}
	return "", fmt.Errorf("authorization timed out after 5 minutes")
}

// signLastFMParams creates a method signature for Last.fm API calls.
func signLastFMParams(params map[string]string) string {
	client := &LastFMClient{sharedSecret: LastFMSharedSecret}
	return client.sign(params)
}
