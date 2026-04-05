// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/config"
)

// StationWarningMsg is sent when user dismisses the station warning
type StationWarningMsg struct {
	Dismissed bool
}

// StationIssue represents a single station discrepancy
type StationIssue = config.StationIssue

// StationWarning modal for station validation results
type StationWarning struct {
	styles *config.ThemeStyles
	issues []StationIssue
}

// NewStationWarning creates a new StationWarning modal
func NewStationWarning(styles *config.ThemeStyles, issues []StationIssue) *StationWarning {
	return &StationWarning{
		styles: styles,
		issues: issues,
	}
}

// Update handles messages
func (s *StationWarning) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case tea.KeyPressMsg:
		return func() tea.Msg { return StationWarningMsg{Dismissed: true} }
	}
	return nil
}

// View renders the modal
func (s StationWarning) View() string {
	modalWidth := 64
	contentWidth := modalWidth - 6

	accentStyle := s.styles.AccentStyle
	mutedStyle := s.styles.MutedStyle

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Bold(true)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12"))

	var b strings.Builder

	// Title
	b.WriteString(centerStyled(warningStyle.Render("STATION NOTICE"), contentWidth))
	b.WriteString("\n\n")

	for _, issue := range s.issues {
		var icon string
		var style lipgloss.Style
		switch issue.Kind {
		case "new":
			icon = "+"
			style = infoStyle
		case "missing":
			icon = "-"
			style = errorStyle
		case "renamed":
			icon = "~"
			style = warningStyle
		default:
			icon = "!"
			style = warningStyle
		}
		b.WriteString(centerStyled(style.Render(fmt.Sprintf("[%s] %s", icon, issue.Message)), contentWidth))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(centerStyled(mutedStyle.Render("If issue remains unresolved, please file an issue"), contentWidth))
	b.WriteString("\n")
	b.WriteString(centerStyled(mutedStyle.Render("on the project's GitHub page."), contentWidth))

	b.WriteString("\n\n")

	helpText := accentStyle.Render("any key") + mutedStyle.Render(" dismiss")
	b.WriteString(centerStyled(helpText, contentWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(s.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}
