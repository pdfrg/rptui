// Package widgets provides reusable TUI components
package widgets

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const connStateDisconnected = "disconnected"

const (
	LidarrStateNotInLidarr = 0
	LidarrStateInLidarr    = 1
	LidarrStateMonitored   = 2
	LidarrStateError       = 3
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
	connState    string // "connected", "disconnected", or ""

	// Scrobble indicator
	scrobbleServices []string // e.g. ["fm", "lb"]
	flashStatesByService map[string]int // per-service state: "fm" -> 0/1/2/3

	// Lidarr indicator
	lidarrConfigured bool
	lidarrState int // LidarrStateNotInLidarr, LidarrStateInLidarr, LidarrStateMonitored, LidarrStateError
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
			{Key: "R", Icon: "", Label: "Rate"},
			{Key: "c", Icon: "", Label: "Copy"},
			{Key: "z", Icon: "", Label: "Sleep"},
			{Key: "o", Icon: "", Label: "Opt"},
		{Key: "m", Icon: "", Label: "Manage"},
		{Key: "L", Icon: "", Label: "Lidarr"},
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
		{Key: "L", Icon: "", Label: "Lidarr"},
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

// GetWidth returns the current width of the footer
func (h *Footer) GetWidth() int {
	return h.width
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

// SetFlashStateByService sets the scrobble flash state per service.
// Map: service name (e.g. "fm", "lb") -> state (0=off, 1=solid, 2=blink-on, 3=blink-off).
func (h *Footer) SetFlashStateByService(states map[string]int) {
	h.flashStatesByService = states
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

// SetConnectionState sets the connection state for scrobble indicator logic
func (h *Footer) SetConnectionState(state string) {
	h.connState = state
}

// AddChannel99 adds "My Paradise" channel to station shortcuts (when RP auth is active)
func (h *Footer) AddChannel99() {
	h.stationKeys = append(h.stationKeys, KeyBinding{Key: "9", Icon: "", Label: "MyParadise"})
}

// SetLidarrConfigured sets whether Lidarr integration is configured
func (h *Footer) SetLidarrConfigured(configured bool) {
	h.lidarrConfigured = configured
}

// SetLidarrState sets the current Lidarr artist status for the [L] indicator
func (h *Footer) SetLidarrState(state int) {
	h.lidarrState = state
}

const (
	flashOff     = 0
	flashSolid   = 1
	flashBlinkOn = 2
)

// scrobbleIndicator returns rendered scrobble indicators, one per service: [fm][lb].
func (h Footer) scrobbleIndicator() string {
	if len(h.scrobbleServices) == 0 {
		return ""
	}

	if h.jukeboxMode && h.connState == connStateDisconnected {
		return ""
	}

	var parts []string
	for _, svc := range h.scrobbleServices {
		state := flashOff
		if h.flashStatesByService != nil {
			state = h.flashStatesByService[svc]
		}
		var style lipgloss.Style
		switch state {
		case flashSolid:
			style = h.accentStyle
		case flashBlinkOn:
			style = h.accentStyle
		default:
			style = h.mutedStyle
		}
		parts = append(parts, style.Render("["+svc+"]"))
	}
	return strings.Join(parts, "")
}

func (h Footer) lidarrIndicator() string {
	if !h.lidarrConfigured {
		return ""
	}

	var style lipgloss.Style
	switch h.lidarrState {
	case LidarrStateMonitored:
		style = h.accentStyle
	case LidarrStateInLidarr:
		style = h.foregroundStyle()
	case LidarrStateError:
		style = h.mutedStyle
	default:
		style = h.mutedStyle
	}
	return style.Render("[L]")
}

func (h Footer) foregroundStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(h.accentStyle.GetForeground())
}

// View renders the footer (two lines: controls + stations)
func (h Footer) View() string {
	renderLine := func(bindings []KeyBinding) string {
		var parts []string
		for _, kb := range bindings {
			if kb.Key == "L" && !h.lidarrConfigured {
				continue
			}
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

		// Only center if width is set and content is narrower than width
		if h.width > 0 {
			contentWidth := lipgloss.Width(content)
			if h.width > contentWidth {
				padding := (h.width - contentWidth) / 2
				content = strings.Repeat(" ", padding) + content
			}
		}

		return content
	}

	if h.miniMode {
		line := renderLine(h.miniKeys)
		return "\n" + line
	}

	stationLine := renderLine(h.stationKeys)
	controlsLine := renderLine(h.keys)

	if h.jukeboxMode {
		jukeLine := renderLine(h.jukeKeys)
		if ind := h.scrobbleIndicator(); ind != "" {
			jukeLine += " " + ind
		}
		if ind := h.lidarrIndicator(); ind != "" {
			jukeLine += " " + ind
		}
		return "\n" + jukeLine
	}

	if h.offlineMode {
		offlineLine := renderLine(h.offlineKeys)
		return "\n" + offlineLine
	}

	// Append scrobble indicator right after station line in normal mode
	if ind := h.scrobbleIndicator(); ind != "" {
		stationLine += " " + ind
	}
	if ind := h.lidarrIndicator(); ind != "" {
		stationLine += " " + ind
	}

	return stationLine + "\n" + controlsLine
}
