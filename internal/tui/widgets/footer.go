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
	accentStyle  lipgloss.Style
	mutedStyle   lipgloss.Style
	width        int
	keys         []KeyBinding
	jukeKeys     []KeyBinding
	offlineKeys  []KeyBinding
	stationKeys  []KeyBinding
	miniKeys     []KeyBinding
	jukeboxMode  bool
	offlineMode  bool
	offlineCache string
	miniMode     bool

	// Scrobble indicator
	scrobbleServices []string // e.g. ["fm", "lb"]
	flashState       int      // 0=off, 1=solid accent, 2=blink-on, 3=blink-off
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
			{Key: "c", Icon: "", Label: "Copy"},
			{Key: "o", Icon: "", Label: "Opt"},
			{Key: "m", Icon: "", Label: "Manage"},
			{Key: "$", Icon: "", Label: "Support"},
			{Key: "q", Icon: "", Label: "Quit"},
		},
		jukeKeys: []KeyBinding{
			{Key: "p", Icon: "󰒮", Label: ""},
			{Key: "r", Icon: "󰜉", Label: ""},
			{Key: "Space", Icon: "󰐎", Label: ""},
			{Key: "\u25c0 \u25b6", Icon: "", Label: "Seek"},
			{Key: "n", Icon: "󰒭", Label: ""},
			{Key: "v", Icon: "", Label: "View"},
			{Key: "c", Icon: "", Label: "Copy"},
			{Key: "o", Icon: "", Label: "Opt"},
			{Key: "$", Icon: "", Label: "Support"},
			{Key: "J", Icon: "", Label: "Normal"},
			{Key: "q", Icon: "", Label: "Quit"},
		},
		offlineKeys: []KeyBinding{
			{Key: "p", Icon: "󰒮", Label: ""},
			{Key: "r", Icon: "󰜉", Label: ""},
			{Key: "Space", Icon: "󰐎", Label: ""},
			{Key: "\u25c0 \u25b6", Icon: "", Label: "Seek"},
			{Key: "n", Icon: "󰒭", Label: ""},
			{Key: "v", Icon: "", Label: "View"},
			{Key: "f", Icon: "⭐", Label: ""},
			{Key: "b", Icon: "🚫", Label: ""},
			{Key: "c", Icon: "", Label: "Copy"},
			{Key: "o", Icon: "", Label: "Opt"},
			{Key: "m", Icon: "", Label: "Manage"},
			{Key: "q", Icon: "", Label: "Quit"},
		},
		stationKeys: []KeyBinding{
			{Key: "0", Icon: "", Label: "Main"},
			{Key: "1", Icon: "", Label: "Mellow"},
			{Key: "2", Icon: "", Label: "RockIt"},
			{Key: "3", Icon: "", Label: "Globe"},
			{Key: "4", Icon: "", Label: "Serenity"},
			{Key: "5", Icon: "", Label: "Beyond"},
			{Key: "6", Icon: "", Label: "KFAT"},
		},
		miniKeys: []KeyBinding{
			{Key: "Space", Icon: "󰐎", Label: ""},
			{Key: "p", Icon: "󰒮", Label: ""},
			{Key: "n", Icon: "󰒭", Label: ""},
			{Key: "q", Icon: "", Label: "Quit"},
		},
	}
}

// SetWidth sets the width of the footer
func (h *Footer) SetWidth(width int) {
	h.width = width
}

// UpdateStyles updates the footer styles with new theme colors
func (h *Footer) UpdateStyles(accentStyle, mutedStyle lipgloss.Style) {
	h.accentStyle = accentStyle
	h.mutedStyle = mutedStyle
}

// SetScrobbleServices sets which scrobble services are active (e.g. ["fm", "lb"])
func (h *Footer) SetScrobbleServices(services []string) {
	h.scrobbleServices = services
}

// SetFlashState sets the scrobble indicator flash state.
// 0=off, 1=solid accent (success), 2=blink-on (failure), 3=blink-off (failure).
func (h *Footer) SetFlashState(state int) {
	h.flashState = state
}

// SetJukeboxMode toggles between normal and jukebox key bindings
func (h *Footer) SetJukeboxMode(jukebox bool) {
	h.jukeboxMode = jukebox
}

// SetOfflineMode toggles between normal and offline key bindings
func (h *Footer) SetOfflineMode(offline bool, cacheName string) {
	h.offlineMode = offline
	h.offlineCache = cacheName
}

// SetMiniMode toggles compact single-line footer with essential keys only
func (h *Footer) SetMiniMode(mini bool) {
	h.miniMode = mini
}

// scrobbleIndicator returns the rendered scrobble indicator string, or empty if none.
func (h Footer) scrobbleIndicator() string {
	if len(h.scrobbleServices) == 0 {
		return ""
	}

	var style lipgloss.Style
	switch h.flashState {
	case 1: // solid accent (success)
		style = h.accentStyle
	case 2: // blink-on (failure)
		style = h.accentStyle
	default: // off or blink-off
		style = h.mutedStyle
	}

	return style.Render("[" + strings.Join(h.scrobbleServices, "+") + "]")
}

// View renders the footer (two lines: controls + stations)
func (h Footer) View() string {
	if h.miniMode {
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
			if h.width > 0 {
				contentWidth := lipgloss.Width(content)
				if h.width > contentWidth {
					padding := (h.width - contentWidth) / 2
					content = strings.Repeat(" ", padding) + content
				}
			}
			return content
		}
		return "\n" + renderLine(h.miniKeys)
	}

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

	stationLine := renderLine(h.stationKeys)
	controlsLine := renderLine(h.keys)

	if h.jukeboxMode {
		jukeLine := renderLine(h.jukeKeys)
		if ind := h.scrobbleIndicator(); ind != "" {
			jukeLine += "  " + ind
		}
		return "\n" + jukeLine
	}

	if h.offlineMode {
		offlineLine := renderLine(h.offlineKeys)
		return "\n" + offlineLine
	}

	// Append scrobble indicator right after station line in normal mode
	if ind := h.scrobbleIndicator(); ind != "" {
		stationLine += "  " + ind
	}

	return stationLine + "\n" + controlsLine
}
