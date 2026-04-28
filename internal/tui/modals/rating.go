// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pdfrg/rptui/internal/config"
)

// RatingMsg is sent when user submits or cancels a rating
type RatingMsg struct {
	Rating    int // 1-10 if submitted, 0 if cancelled
	Submitted bool
}

// Rating modal for submitting song ratings
type Rating struct {
	styles     *config.ThemeStyles
	songTitle  string
	songArtist string
	songAlbum  string
	songYear   string
	rating     int
}

// NewRating creates a new Rating modal
func NewRating(styles *config.ThemeStyles, title, artist, album, year string, currentRating int) *Rating {
	if currentRating < 1 || currentRating > 10 {
		currentRating = 5
	}
	return &Rating{
		styles:     styles,
		songTitle:  title,
		songArtist: artist,
		songAlbum:  album,
		songYear:   year,
		rating:     currentRating,
	}
}

// Update handles messages
func (r *Rating) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "down":
			if r.rating > 1 {
				r.rating--
			}
		case "right", "up":
			if r.rating < 10 {
				r.rating++
			}
		case "enter":
			return func() tea.Msg { return RatingMsg{Rating: r.rating, Submitted: true} }
		case "esc", "q":
			return func() tea.Msg { return RatingMsg{Rating: 0, Submitted: false} }
		}
	}
	return nil
}

// View renders the modal
func (r Rating) View() string {
	modalWidth := 60
	contentWidth := modalWidth - 6

	accentStyle := r.styles.AccentStyle
	mutedStyle := r.styles.MutedStyle
	cursorStyle := r.styles.CursorStyle

	var lines []string

	// Title
	lines = append(lines, centerStyled(accentStyle.Bold(true).Render("RATE THIS SONG"), contentWidth))
	lines = append(lines, "")

	// Song info
	lines = append(lines, centerStyled(accentStyle.Render(r.songTitle), contentWidth))
	lines = append(lines, centerStyled(r.styles.ForegroundStyle.Render(r.songArtist), contentWidth))
	albumYear := r.songAlbum
	if r.songYear != "" && r.songYear != "—" {
		albumYear = fmt.Sprintf("%s (%s)", r.songAlbum, r.songYear)
	}
	lines = append(lines, centerStyled(mutedStyle.Render(albumYear), contentWidth))
	lines = append(lines, "")

	// Rating selector
	ratingBar := buildRatingBar(r.rating, accentStyle, cursorStyle, mutedStyle)
	lines = append(lines, centerStyled(ratingBar, contentWidth))
	lines = append(lines, "")
	lines = append(lines, centerStyled(mutedStyle.Render(fmt.Sprintf("Your rating: %d", r.rating)), contentWidth))
	lines = append(lines, "")

	// Help text
	helpText := accentStyle.Render("← →") + mutedStyle.Render(" adjust  ") +
		accentStyle.Render("enter") + mutedStyle.Render(" submit  ") +
		accentStyle.Render("esc") + mutedStyle.Render(" cancel")
	lines = append(lines, centerStyled(helpText, contentWidth))

	content := strings.Join(lines, "\n")

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(r.styles.AccentStyle.GetForeground()).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(content)
}

// buildRatingBar renders the 1-10 rating selector with arrows
func buildRatingBar(selected int, accentStyle, cursorStyle, mutedStyle lipgloss.Style) string {
	var parts []string
	parts = append(parts, mutedStyle.Render("◀ "))
	for i := 1; i <= 10; i++ {
		if i == selected {
			parts = append(parts, cursorStyle.Bold(true).Render(fmt.Sprintf("%d", i)))
		} else {
			parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d", i)))
		}
		if i < 10 {
			parts = append(parts, " ")
		}
	}
	parts = append(parts, mutedStyle.Render(" ▶"))
	return strings.Join(parts, "")
}
