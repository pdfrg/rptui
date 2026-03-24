// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/config"
)

// SkipWarningMsg is sent when user confirms or cancels skip
type SkipWarningMsg struct {
	Confirmed bool
}

// SkipWarning modal for confirming song skip
type SkipWarning struct {
	styles       *config.ThemeStyles
	minFavorites int
}

// NewSkipWarning creates a new SkipWarning modal
func NewSkipWarning(styles *config.ThemeStyles, minFavorites int) *SkipWarning {
	return &SkipWarning{
		styles:       styles,
		minFavorites: minFavorites,
	}
}

// Update handles messages
func (s *SkipWarning) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "enter":
			return func() tea.Msg { return SkipWarningMsg{Confirmed: true} }
		case "n", "esc", "q":
			return func() tea.Msg { return SkipWarningMsg{Confirmed: false} }
		}
	}
	return nil
}

// View renders the modal
func (s SkipWarning) View() string {
	modalWidth := 60
	contentWidth := modalWidth - 6

	accentStyle := s.styles.AccentStyle
	mutedStyle := s.styles.MutedStyle

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	var lines []string

	// Title
	lines = append(lines, centerStyled(warningStyle.Render("SKIP SONG?"), contentWidth))
	lines = append(lines, "")

	// Body text
	lines = append(lines, centerStyled(mutedStyle.Render("Skipping ahead of the live stream can result"), contentWidth))
	lines = append(lines, centerStyled(mutedStyle.Render("in an interruption of playback when the"), contentWidth))
	lines = append(lines, centerStyled(mutedStyle.Render("playlist end is reached."), contentWidth))
	lines = append(lines, "")
	lines = append(lines, centerStyled(mutedStyle.Render(fmt.Sprintf("Save at least %d favorites (press ", s.minFavorites))+
		accentStyle.Render("f")+mutedStyle.Render(") to enable"), contentWidth))
	lines = append(lines, centerStyled(mutedStyle.Render("favorite mode for continuous playback."), contentWidth))
	lines = append(lines, "")

	// Confirm/cancel
	helpText := accentStyle.Render("y") + mutedStyle.Render(" skip  ") +
		accentStyle.Render("n") + mutedStyle.Render(" cancel")
	lines = append(lines, centerStyled(helpText, contentWidth))

	content := ""
	for i, line := range lines {
		content += line
		if i < len(lines)-1 {
			content += "\n"
		}
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(content)
}
