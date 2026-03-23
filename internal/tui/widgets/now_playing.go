// Package widgets provides reusable TUI components
package widgets

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/models"
)

// darkenColor reduces the brightness of a hex color by the given factor (0.0-1.0)
func darkenColor(hex string, factor float64) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	var r, g, b int
	fmt.Sscanf(hex[1:3], "%x", &r)
	fmt.Sscanf(hex[3:5], "%x", &g)
	fmt.Sscanf(hex[5:7], "%x", &b)
	r = int(float64(r) * (1 - factor))
	g = int(float64(g) * (1 - factor))
	b = int(float64(b) * (1 - factor))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// NowPlaying represents the current song info widget
type NowPlaying struct {
	foregroundStyle lipgloss.Style
	accentStyle     lipgloss.Style
	mutedStyle      lipgloss.Style
	width           int

	// Bubbles progress bar
	progress progress.Model
}

// NewNowPlaying creates a new NowPlaying widget
func NewNowPlaying(fgStyle, accentStyle, mutedStyle lipgloss.Style, accentColor, cursorColor, bgColor string) *NowPlaying {
	p := progress.New(
		progress.WithWidth(40),
		progress.WithColors(lipgloss.Color(cursorColor), lipgloss.Color(accentColor)), // gradient fill
		progress.WithoutPercentage(),
		progress.WithFillCharacters('', ''),
	)
	p.EmptyColor = lipgloss.Color(darkenColor(bgColor, 0.3)) // unfilled = background darkened 30%

	return &NowPlaying{
		foregroundStyle: fgStyle,
		accentStyle:     accentStyle,
		mutedStyle:      mutedStyle,
		progress:        p,
	}
}

// SetWidth sets the width of the widget
func (n *NowPlaying) SetWidth(width int) {
	n.width = width
	// Cap progress bar at 40 chars
	progWidth := min(40, width-2)
	progWidth = max(20, progWidth)
	n.progress.SetWidth(progWidth)
}

// UpdateProgress updates the progress bar percent (0.0 to 1.0)
func (n *NowPlaying) UpdateProgress(percent float64) tea.Cmd {
	return n.progress.SetPercent(percent)
}

// Update handles messages (like FrameMsg for animations)
func (n *NowPlaying) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	n.progress, cmd = n.progress.Update(msg)
	return cmd
}

// View renders the now playing info as a string
func (n NowPlaying) View(
	song *models.Song,
	isPaused bool,
	timePos float64,
	connectedTime time.Time,
	configInfo string,
	favoriteCount int,
	isFavorite bool,
	skipsAvailable int,
	prevAvailable int,
) string {
	if song == nil {
		return " No song playing"
	}

	title := n.accentStyle.Bold(true).Render(song.Title)
	artist := n.foregroundStyle.Render(song.Artist)
	album := n.mutedStyle.Render(fmt.Sprintf("%s (%s)", song.Album, song.Year))

	progView := n.progress.View()

	percentPos := timePos / song.GetDurationSeconds() * 100
	if song.GetDurationSeconds() <= 0 {
		percentPos = 0
	}

	timeStr := n.mutedStyle.Render(fmt.Sprintf("%s / %s (%.0f%%)",
		formatDuration(timePos),
		formatDuration(song.GetDurationSeconds()),
		percentPos))

	rating := song.Rating
	if isFavorite {
		rating = "★ " + rating
	}
	ratingStr := n.accentStyle.Render(rating)

	// Navigation line
	nextStr := "--"
	if skipsAvailable > 0 {
		nextStr = fmt.Sprintf("%d", skipsAvailable)
	}
	prevStr := "--"
	if prevAvailable > 0 {
		prevStr = fmt.Sprintf("%d", prevAvailable)
	}
	navLine := n.foregroundStyle.Render(fmt.Sprintf("Next: %s | Prev: %s | Favorites: %d",
		nextStr, prevStr, favoriteCount))

	status := "Playing"
	if isPaused {
		status = n.mutedStyle.Render("Paused")
	} else {
		status = n.mutedStyle.Render("Playing")
	}
	statusLine := fmt.Sprintf("%s • %s", status, n.mutedStyle.Render(configInfo))

	connectedLine := n.mutedStyle.Render(fmt.Sprintf("Connected • %s", connectedTime.Format("15:04:05")))

	return fmt.Sprintf(" %s\n %s\n %s\n\n %s\n %s\n\n %s\n\n %s\n\n %s\n\n %s\n\n",
		title, artist, album, progView, timeStr, ratingStr, navLine, statusLine, connectedLine)
}

// formatDuration formats seconds as MM:SS or HH:MM:SS
func formatDuration(seconds float64) string {
	totalSeconds := int(seconds)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	secs := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}
