// Package api provides clients for external API
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Comment represents a single comment from RP
type Comment struct {
	Username   string
	PostedTime string
	Message    string
	QuotedText string
	Upvotes    int
	Downvotes  int
	Location   string
}

// CommentsResponse represents the comments::list API response
type CommentsResponse struct {
	Comments      []Comment
	TotalComments int
	MoreComments  bool
	MoreOffset    int
}

// RPCommentsClient handles RP comments API interactions
type RPCommentsClient struct {
	authClient *RPAuthClient
	httpClient *http.Client
}

// NewRPCommentsClient creates a new comments API client
func NewRPCommentsClient(authClient *RPAuthClient) *RPCommentsClient {
	return &RPCommentsClient{
		authClient: authClient,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetComments fetches comments for a song
func (c *RPCommentsClient) GetComments(songID int64, numComments int, order string) (*CommentsResponse, error) {
	url := fmt.Sprintf("https://api.radioparadise.com/siteapi.php?file=comments%%3A%%3Alist&song_id=%d&comments_num=%d&order=%s",
		songID, numComments, order)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.authClient != nil && c.authClient.HasAuth() {
		req.Header.Set("Cookie", c.authClient.CookieString())
	}
	req.Header.Set("User-Agent", "rptui/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var raw struct {
		Comments []struct {
			Username    string `json:"username"`
			PostedTime  string `json:"posted_time"`
			Message     string `json:"message"`
			Upvotes     int    `json:"upvotes"`
			Downvotes   int    `json:"downvotes"`
			Location    string `json:"location"`
			UserUpvotes int    `json:"userupvotes"`
		} `json:"comments"`
		TotalComments string `json:"total_comments"`
		MoreComments  bool   `json:"more_comments"`
		MoreOffset    int    `json:"more_offset"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode comments: %w", err)
	}

	var totalComments int
	fmt.Sscanf(raw.TotalComments, "%d", &totalComments)

	comments := make([]Comment, 0, len(raw.Comments))
	for _, rc := range raw.Comments {
		quotedText, message := parseCommentHTML(rc.Message)
		comments = append(comments, Comment{
			Username:   rc.Username,
			PostedTime: rc.PostedTime,
			Message:    message,
			QuotedText: quotedText,
			Upvotes:    rc.Upvotes,
			Downvotes:  rc.Downvotes,
			Location:   rc.Location,
		})
	}

	return &CommentsResponse{
		Comments:      comments,
		TotalComments: totalComments,
		MoreComments:  raw.MoreComments,
		MoreOffset:    raw.MoreOffset,
	}, nil
}

// GetCommentsWithOffset fetches comments starting from a given offset (for pagination)
func (c *RPCommentsClient) GetCommentsWithOffset(songID int64, numComments int, order string, offset int) (*CommentsResponse, error) {
	url := fmt.Sprintf("https://api.radioparadise.com/siteapi.php?file=comments%%3A%%3Alist&song_id=%d&comments_num=%d&order=%s&comments_offset=%d",
		songID, numComments, order, offset)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.authClient != nil && c.authClient.HasAuth() {
		req.Header.Set("Cookie", c.authClient.CookieString())
	}
	req.Header.Set("User-Agent", "rptui/0.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var raw struct {
		Comments []struct {
			Username    string `json:"username"`
			PostedTime  string `json:"posted_time"`
			Message     string `json:"message"`
			Upvotes     int    `json:"upvotes"`
			Downvotes   int    `json:"downvotes"`
			Location    string `json:"location"`
			UserUpvotes int    `json:"userupvotes"`
		} `json:"comments"`
		TotalComments string `json:"total_comments"`
		MoreComments  bool   `json:"more_comments"`
		MoreOffset    int    `json:"more_offset"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode comments: %w", err)
	}

	var totalComments int
	fmt.Sscanf(raw.TotalComments, "%d", &totalComments)

	comments := make([]Comment, 0, len(raw.Comments))
	for _, rc := range raw.Comments {
		quotedText, message := parseCommentHTML(rc.Message)
		comments = append(comments, Comment{
			Username:   rc.Username,
			PostedTime: rc.PostedTime,
			Message:    message,
			QuotedText: quotedText,
			Upvotes:    rc.Upvotes,
			Downvotes:  rc.Downvotes,
			Location:   rc.Location,
		})
	}

	return &CommentsResponse{
		Comments:      comments,
		TotalComments: totalComments,
		MoreComments:  raw.MoreComments,
		MoreOffset:    raw.MoreOffset,
	}, nil
}

// quotePattern matches: <strong...>username wrote:</strong>...<div class="quote">...</div>
// (?s) makes . match newlines to handle HTML with line breaks between elements
var quotePattern = regexp.MustCompile(`(?s)<strong[^>]*>([^<]*)wrote:</strong>.*?<div class="quote">(.*?)</div>`)

// htmlTagPattern matches HTML tags
var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

// htmlEntityMap maps common HTML entities
var htmlEntityMap = map[string]string{
	"&nbsp;": " ",
	"&amp;":  "&",
	"&lt;":   "<",
	"&gt;":   ">",
	"&quot;": `"`,
	"&#39;":  "'",
	"&apos;": "'",
}

// htmlEntityPattern matches numeric HTML entities like &#160;
var htmlEntityPattern = regexp.MustCompile(`&#(\d+);`)

// emojiPattern matches emoji img tags: <img title="{#Sleep}" ...>
var emojiPattern = regexp.MustCompile(`<img[^>]*title="(\{#[^}]*\})"[^>]*/>`)

// parseCommentHTML converts RP comment HTML to plain text, preserving quote structure
func parseCommentHTML(html string) (quotedText, message string) {
	// Extract emoji placeholders before stripping tags
	text := emojiPattern.ReplaceAllString(html, " $1 ")

	// Try to extract quoted block
	match := quotePattern.FindStringSubmatch(text)
	if match != nil {
		quotedAuthor := strings.TrimSpace(match[1])
		quotedContent := stripHTML(match[2])
		quotedText = fmt.Sprintf("%s wrote:\n  %s", quotedAuthor, quotedContent)
		// Remove the quote from the text for message parsing
		text = strings.Replace(text, match[0], "", 1)
	}

	// Strip remaining HTML and clean up
	message = stripHTML(text)
	message = normalizeWhitespace(message)

	return quotedText, message
}

// stripHTML removes all HTML tags and decodes entities
func stripHTML(html string) string {
	// Remove all HTML tags
	text := htmlTagPattern.ReplaceAllString(html, "")

	// Decode named entities
	for entity, replacement := range htmlEntityMap {
		text = strings.ReplaceAll(text, entity, replacement)
	}

	// Decode numeric entities
	text = htmlEntityPattern.ReplaceAllStringFunc(text, func(match string) string {
		var code int
		fmt.Sscanf(match, "&#%d;", &code)
		if code > 0 && code < 0x10FFFF {
			return string(rune(code))
		}
		return ""
	})

	return text
}

// normalizeWhitespace cleans up excessive whitespace
func normalizeWhitespace(s string) string {
	// Replace multiple spaces/tabs with single space
	spacePattern := regexp.MustCompile(`[ \t]+`)
	s = spacePattern.ReplaceAllString(s, " ")

	// Replace 3+ newlines with 2 newlines
	newlinePattern := regexp.MustCompile(`\n{3,}`)
	s = newlinePattern.ReplaceAllString(s, "\n\n")

	return strings.TrimSpace(s)
}
