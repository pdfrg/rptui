// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/cache"
	"rptui-bubbletea/internal/config"
)

// Network transition variants
const (
	NetworkGoingOffline = iota
	NetworkGoingOnline
)

// NetworkTransitionMsg is sent when user interacts with network transition modal
type NetworkTransitionMsg struct {
	Action   string // "select", "confirm", "dismiss"
	Cache    string // cache name if selected
	Response bool   // true/false for confirm
}

// NetworkTransition modal for network status changes
type NetworkTransition struct {
	styles   *config.ThemeStyles
	variant  int // NetworkGoingOffline or NetworkGoingOnline
	caches   []cache.CacheEntry
	errorMsg string
}

// NewNetworkTransition creates a new NetworkTransition modal
func NewNetworkTransition(styles *config.ThemeStyles, variant int, caches []cache.CacheEntry, errorMsg string) *NetworkTransition {
	return &NetworkTransition{
		styles:   styles,
		variant:  variant,
		caches:   caches,
		errorMsg: errorMsg,
	}
}

// Update handles messages
func (n *NetworkTransition) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()

		// Going offline: handle cache selection (1-9) or dismiss (n)
		if n.variant == NetworkGoingOffline {
			// User pressed a number 1-9 to select a cache
			if len(key) == 1 && key >= "1" && key <= "9" {
				idx := int(key[0] - '1')
				if idx < len(n.caches) {
					return func() tea.Msg {
						return NetworkTransitionMsg{Action: "select", Cache: n.caches[idx].Name}
					}
				}
			}
			// User pressed n to ignore/stay on retry
			if key == "n" {
				return func() tea.Msg {
					return NetworkTransitionMsg{Action: "dismiss"}
				}
			}
		}

		// Going online: handle confirm (y) or stay (n)
		if n.variant == NetworkGoingOnline {
			if key == "y" || key == "enter" {
				return func() tea.Msg {
					return NetworkTransitionMsg{Action: "confirm", Response: true}
				}
			}
			if key == "n" || key == "esc" || key == "q" {
				return func() tea.Msg {
					return NetworkTransitionMsg{Action: "confirm", Response: false}
				}
			}
		}

		// No caches: any key dismisses
		if n.variant == NetworkGoingOffline && len(n.caches) == 0 {
			return func() tea.Msg {
				return NetworkTransitionMsg{Action: "dismiss"}
			}
		}
	}
	return nil
}

// View renders the modal
func (n NetworkTransition) View() string {
	modalWidth := 64
	contentWidth := modalWidth - 6

	accentStyle := n.styles.AccentStyle
	mutedStyle := n.styles.MutedStyle

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("76")).
		Bold(true)

	var b strings.Builder

	if n.variant == NetworkGoingOffline {
		b.WriteString(centerStyled(warningStyle.Render("NETWORK ISSUE DETECTED"), contentWidth))
		b.WriteString("\n\n")
		b.WriteString(centerStyled(mutedStyle.Render(fmt.Sprintf("%s • Retrying...", n.errorMsg)), contentWidth))
		b.WriteString("\n\n")

		if len(n.caches) == 0 {
			b.WriteString(centerStyled(mutedStyle.Render("No offline caches available."), contentWidth))
			b.WriteString("\n")
			b.WriteString(centerStyled(mutedStyle.Render("Consider:"), contentWidth))
			b.WriteString("\n")
			b.WriteString(centerStyled(accentStyle.Render("  rptui --cache"), contentWidth))
			b.WriteString(mutedStyle.Render(" (save a cache)"))
			b.WriteString("\n")
			b.WriteString(centerStyled(accentStyle.Render("  rptui -j"), contentWidth))
			b.WriteString(mutedStyle.Render(" (jukebox mode)"))
			b.WriteString("\n")
			b.WriteString(centerStyled(mutedStyle.Render("    or press "), contentWidth))
			b.WriteString("\n")
			b.WriteString(centerStyled(accentStyle.Render("J")+mutedStyle.Render(" after dismiss"), contentWidth))
			b.WriteString("\n\n")
			dismissText := accentStyle.Render("any key") + mutedStyle.Render(" dismiss")
			b.WriteString(centerStyled(dismissText, contentWidth))
		} else {
			b.WriteString(centerStyled(mutedStyle.Render("Available offline caches:"), contentWidth))
			b.WriteString("\n")
			for i, c := range n.caches {
				if i < 9 {
					b.WriteString(centerStyled(
						accentStyle.Render(fmt.Sprintf(" %d. ", i+1))+
							mutedStyle.Render(c.Name),
						contentWidth,
					))
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
			helpText := accentStyle.Render("[1-9]") + mutedStyle.Render(" select  ") +
				accentStyle.Render("n") + mutedStyle.Render(" ignore")
			b.WriteString(centerStyled(helpText, contentWidth))
		}
	} else if n.variant == NetworkGoingOnline {
		b.WriteString(centerStyled(successStyle.Render("CONNECTION RESTORED"), contentWidth))
		b.WriteString("\n\n")
		b.WriteString(centerStyled(mutedStyle.Render("✓ Back online"), contentWidth))
		b.WriteString("\n\n")
		b.WriteString(centerStyled(mutedStyle.Render("Return to live stream?"), contentWidth))
		b.WriteString("\n\n")
		helpText := accentStyle.Render("y") + mutedStyle.Render(" yes  ") +
			accentStyle.Render("n") + mutedStyle.Render(" stay offline")
		b.WriteString(centerStyled(helpText, contentWidth))
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(n.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}
