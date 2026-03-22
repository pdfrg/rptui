// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

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
	width        int
	height       int
	minFavorites int
}

// NewSkipWarning creates a new SkipWarning modal
func NewSkipWarning(styles *config.ThemeStyles, minFavorites int) *SkipWarning {
	return &SkipWarning{
		styles:       styles,
		minFavorites: minFavorites,
	}
}

// SetSize sets the dimensions
func (s *SkipWarning) SetSize(width, height int) {
	s.width = width
	s.height = height
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
	modalWidth := 50
	modalHeight := 10

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(centerText("SKIP SONG?", modalWidth))
	b.WriteString("\n\n")
	b.WriteString(centerText("Skipping ahead of the live stream can result", modalWidth))
	b.WriteString("\n")
	b.WriteString(centerText("in an interruption of playback when the", modalWidth))
	b.WriteString("\n")
	b.WriteString(centerText("playlist end is reached.", modalWidth))
	b.WriteString("\n\n")
	b.WriteString(centerText(fmt.Sprintf("Save at least %d favorites to enable", s.minFavorites), modalWidth))
	b.WriteString("\n")
	b.WriteString(centerText("favorite mode for continuous playback.", modalWidth))
	b.WriteString("\n\n")
	b.WriteString(centerText("[y] Yes   [n] No", modalWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth).
		Height(modalHeight)

	return modalStyle.Render(b.String())
}

func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}
