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
func NewNowPlaying(fgStyle, accentStyle, mutedStyle lipgloss.Style, accentColor string) *NowPlaying {
	p := progress.New(
		progress.WithWidth(40),
		progress.WithColors(lipgloss.Color(accentColor), lipgloss.Color("#333333")),
		progress.WithoutPercentage(),
	)
	
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
	n.progress.SetWidth(40) // Keep fixed width for now
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
) string {
	if song == nil {
		return " No song playing"
	}

	title := n.foregroundStyle.Bold(true).Render(song.Title)
	artist := n.mutedStyle.Render(song.Artist)
	album := n.mutedStyle.Render(fmt.Sprintf("%s (%s)", song.Album, song.Year))
	
	progView := n.progress.View()

	percentPos := timePos / song.GetDurationSeconds() * 100
	if song.GetDurationSeconds() <= 0 {
		percentPos = 0
	}
	
	timeStr := n.foregroundStyle.Render(fmt.Sprintf("%s / %s (%.0f%%)",
		formatDuration(timePos),
		formatDuration(song.GetDurationSeconds()),
		percentPos))

	rating := song.Rating
	if isFavorite {
		rating = "★ " + rating
	}
	ratingStr := n.accentStyle.Render(rating)

	status := "Playing"
	if isPaused {
		status = n.accentStyle.Render("Paused")
	} else {
		status = n.accentStyle.Render("Playing")
	}
	statusLine := fmt.Sprintf("%s • %s", status, n.mutedStyle.Render(configInfo))

	connectedLine := fmt.Sprintf("%s • %s", n.foregroundStyle.Render("Connected"), n.mutedStyle.Render(connectedTime.Format("15:04:05")))

	return fmt.Sprintf(" %s\n\n %s\n\n %s\n\n\n %s\n\n %s\n\n %s\n\n %s\n\n %s\n",
		title, artist, album, progView, timeStr, ratingStr, statusLine, connectedLine)
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
