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
	stationKeys []KeyBinding
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
			{Key: "\u25c0 \u25b6", Icon: "", Label: "Seek"},
			{Key: "n", Icon: "󰒭", Label: ""},
			{Key: "v", Icon: "", Label: "View"},
			{Key: "f", Icon: "⭐", Label: ""},
			{Key: "b", Icon: "🚫", Label: ""},
			{Key: "o", Icon: "", Label: "Opt"},
			{Key: "m", Icon: "", Label: "Manage"},
			{Key: "$", Icon: "", Label: "Support RP"},
			{Key: "q", Icon: "", Label: "Quit"},
		},
		stationKeys: []KeyBinding{
			{Key: "0", Icon: "", Label: "Main"},
			{Key: "1", Icon: "", Label: "Mellow"},
			{Key: "2", Icon: "", Label: "Rock"},
			{Key: "3", Icon: "", Label: "Global"},
			{Key: "5", Icon: "", Label: "Beyond"},
		},
	}
}

// SetWidth sets the width of the footer
func (h *Footer) SetWidth(width int) {
	h.width = width
}

// View renders the footer (two lines: controls + stations)
func (h Footer) View() string {
	renderLine := func(bindings []KeyBinding) string {
		var parts []string
		for _, kb := range bindings {
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

	return renderLine(h.stationKeys) + "\n" + renderLine(h.keys)
}
