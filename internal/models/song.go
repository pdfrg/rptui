// Package models defines the core data models for the application
package models

import (
	"fmt"
)

// Song represents a song with all its metadata
type Song struct {
	Title          string `json:"title"`
	Artist         string `json:"artist"`
	Album          string `json:"album"`
	Year           string `json:"year"`
	Rating         string `json:"rating"`          // From get_block (average user rating)
	ListenerRating string `json:"listener_rating"` // From nowplaying_list
	UserRating     string `json:"user_rating"`     // Logged-in user's rating ("0"-"10")
	SongID         int64  `json:"song_id"`         // RP song ID (needed for comments, ratings)
	CoverLarge     string `json:"cover_large"`
	CoverMedium    string `json:"cover_medium"`
	CoverSmall     string `json:"cover_small"`
	Duration       int64  `json:"duration"`  // In milliseconds
	EventID        int64  `json:"event_id"`  // Unique identifier
	PlayTime       int64  `json:"play_time"` // When song is scheduled to play (sched_time_millis)
	GaplessURL     string `json:"gapless_url"`
	URL            string `json:"url"`
	IsCurrent      bool   `json:"is_current"`
	IsFromFavorite bool   `json:"-"` // true if queued from favorites (not from API)
	BlockID        int64  `json:"-"` // which block this song belongs to
}

// NewSong creates a new Song from API data
func NewSong(data map[string]any, imageBase string) *Song {
	song := &Song{
		Title:          getString(data, "title", "Unknown Title"),
		Artist:         getString(data, "artist", "Unknown Artist"),
		Album:          getString(data, "album", "—"),
		Year:           getString(data, "year", "—"),
		Rating:         getString(data, "rating", "—"),
		ListenerRating: getString(data, "listener_rating", "—"),
		UserRating:     getString(data, "user_rating", "0"),
		SongID:         getInt64(data, "song_id", 0),
		Duration:       getInt64(data, "duration", 0),
		EventID:        getInt64(data, "event", 0),
		PlayTime:       getInt64(data, "sched_time_millis", 0),
		GaplessURL:     getString(data, "gapless_url", ""),
		URL:            getString(data, "url", ""),
	}

	// Build cover URLs
	coverLarge := getString(data, "cover_large", getString(data, "cover", ""))
	coverMedium := getString(data, "cover_medium", getString(data, "cover_med", ""))
	coverSmall := getString(data, "cover_small", "")

	song.CoverLarge = buildCoverURL(coverLarge, imageBase)
	song.CoverMedium = buildCoverURL(coverMedium, imageBase)
	song.CoverSmall = buildCoverURL(coverSmall, imageBase)

	return song
}

// buildCoverURL builds full URL from cover path and image_base
func buildCoverURL(coverPath, imageBase string) string {
	if coverPath == "" {
		return ""
	}
	if len(coverPath) >= 7 && (coverPath[:7] == "http://" || coverPath[:8] == "https://") {
		return coverPath
	}
	if len(imageBase) >= 2 && imageBase[:2] == "//" {
		return "https:" + imageBase + coverPath
	}
	return imageBase + coverPath
}

// getString safely extracts a string from a map
func getString(data map[string]any, key, defaultVal string) string {
	if v, ok := data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// getInt64 safely extracts an int64 from a map (handles string or number)
func getInt64(data map[string]any, key string, defaultVal int64) int64 {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case int64:
			return val
		case int:
			return int64(val)
		case float64:
			return int64(val)
		case string:
			// Handle string numbers (e.g., "2850087")
			var result int64
			fmt.Sscanf(val, "%d", &result)
			return result
		}
	}
	return defaultVal
}

// GetDurationSeconds returns the duration in seconds
func (s *Song) GetDurationSeconds() float64 {
	return float64(s.Duration) / 1000.0
}

// GetDurationFormatted returns the duration formatted as MM:SS or HH:MM:SS
func (s *Song) GetDurationFormatted() string {
	totalSeconds := int64(s.Duration / 1000)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// FormatDisplayInfo returns a formatted string with artist, album, year, and title
// suitable for copying to clipboard
func (s *Song) FormatDisplayInfo() string {
	albumYear := s.Album
	if s.Year != "" && s.Year != "—" {
		albumYear = fmt.Sprintf("%s (%s)", s.Album, s.Year)
	}
	return fmt.Sprintf("%s - %s - %s", s.Artist, albumYear, s.Title)
}
