// Package widgets provides reusable TUI components
package widgets

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key   string
	Icon  string
	Label string
}

// Footer represents the bottom status/shortcuts bar
type Footer struct {
	accentStyle lipgloss.Style
	mutedStyle  lipgloss.Style
	width       int
	keys        []KeyBinding
}

// NewFooter creates a new Footer widget
func NewFooter(accentStyle, mutedStyle lipgloss.Style) *Footer {
	return &Footer{
		accentStyle: accentStyle,
		mutedStyle:  mutedStyle,
		keys: []KeyBinding{
			{Key: "p", Icon: "󰒮", Label: ""},
			{Key: "r", Icon: "󰜉", Label: ""},
			{Key: "Space", Icon: "󰐎", Label: ""},
			{Key: "s", Icon: "󰓛", Label: ""},
			{Key: "n", Icon: "󰒭", Label: ""},
			{Key: "v", Icon: "", Label: "View"},
			{Key: "f", Icon: "", Label: "Fave"},
			{Key: "b", Icon: "", Label: "Block"},
			{Key: "o", Icon: "", Label: "Opt"},
			{Key: "m", Icon: "", Label: "Manage"},
			{Key: "$", Icon: "", Label: "Support RP"},
			{Key: "q", Icon: "", Label: "Quit"},
		},
	}
}

// SetWidth sets the width of the footer
func (h *Footer) SetWidth(width int) {
	h.width = width
}

// View renders the footer
func (h Footer) View() string {
	var parts []string
	for _, kb := range h.keys {
		keyPart := h.accentStyle.Render(kb.Key)

		var descPart string
		if kb.Icon != "" {
			descPart = h.mutedStyle.Render(kb.Icon)
		} else if kb.Label != "" {
			descPart = h.mutedStyle.Render(kb.Label)
		}

		if descPart != "" {
			parts = append(parts, keyPart+" "+descPart)
		} else {
			parts = append(parts, keyPart)
		}
	}

	content := strings.Join(parts, "  ")

	// Center only if width has been set
	if h.width > 0 {
		contentWidth := lipgloss.Width(content)
		if h.width > contentWidth {
			padding := (h.width - contentWidth) / 2
			content = strings.Repeat(" ", padding) + content
		}
	}

	return content
}
