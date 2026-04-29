// Package modals provides modal dialogs for the TUI
package modals

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
	"github.com/pdfrg/rptui/internal/config"
	"github.com/pdfrg/rptui/internal/tui/visualizer"
)

// OptionsMsg is sent when user applies options or closes the modal
type OptionsMsg struct {
	Station              *int
	Bitrate              *int
	ShowAlbumArt         *bool
	ShowSkipWarn         *bool
	SkipDJSegments       *bool
	CopyAlbumArt         *bool
	NotificationsEnabled *bool
	NotificationsShowArt *bool
	VisualizerMode       *string
	Theme                *string
	Closed               bool
}

// Option item identifiers
const (
	optStation = iota
	optBitrate
	optShowAlbumArt
	optShowSkipWarning
	optSkipDJSpeech
	optCopyAlbumArt
	optNotificationsEnabled
	optNotificationsShowArt
	optVisualizerMode
	optTheme
)

// Theme option list
func themeOptions() []string {
	return []string{
		"Custom",
		"Omarchy",
		"Default",
		"catppuccin-mocha",
		"gruvbox-dark",
		"dark-red",
		"osaka-jade",
		"synth",
		"basic",
	}
}

// themeFromConfig determines which theme option is currently active
func themeFromConfig(colorsFile, themeName string) int {
	opts := config.ThemeNames()

	// Check for custom colors_file
	if colorsFile != "" {
		return 0 // "Custom"
	}

	// Check for built-in theme
	for i, t := range opts {
		if t == themeName {
			// Found built-in theme, adjust for offset (first 3 are Custom, Omarchy, Default)
			return i + 3
		}
	}

	// Check if Omarchy theme exists
	omarchyPath := filepath.Join(xdg.ConfigHome, "omarchy", "current", "theme", "colors.toml")
	if _, err := os.Stat(omarchyPath); err == nil {
		return 1 // "Omarchy"
	}

	// Default fallback
	return 2 // "Default"
}

// Valid station IDs in order
var stationIDs = []int{0, 1, 2, 3, 42, 5, 945}

// Valid bitrate IDs in order
var bitrateIDs = []int{1, 2, 3, 4}

// Options modal for settings
type Options struct {
	styles *config.ThemeStyles
	cursor int // index into visible items

	// Current values (editable)
	stationIdx           int // index into stationIDs
	bitrateIdx           int // index into bitrateIDs
	showAlbumArt         bool
	showSkipWarn         bool
	skipDJSegments       bool
	copyAlbumArt         bool
	notificationsEnabled bool
	notificationsShowArt bool
	visModeIdx           int // index into visualizer mode names
	themeIdx             int // index into theme options

	// Original values for change detection
	origStationIdx   int
	origBitrateIdx   int
	origAlbumArt     bool
	origSkipWarn     bool
	origSkipDJSpeech bool
	origCopyArt      bool
	origNotifEnabled bool
	origNotifShowArt bool
	origVisModeIdx   int
	origThemeIdx     int

	// Feature availability flags
	djSkipAvailable bool // true if --setup-dj-skip has been run
}

// visibleItems returns the ordered list of option IDs to display.
func (o *Options) visibleItems() []int {
	items := []int{
		optStation,
		optBitrate,
		optShowAlbumArt,
		optShowSkipWarning,
	}
	if o.djSkipAvailable {
		items = append(items, optSkipDJSpeech)
	}
	items = append(items,
		optCopyAlbumArt,
		optNotificationsEnabled,
		optNotificationsShowArt,
		optVisualizerMode,
		optTheme,
	)
	return items
}

// currentOptID returns the option ID at the current cursor position.
func (o *Options) currentOptID() int {
	items := o.visibleItems()
	if o.cursor >= 0 && o.cursor < len(items) {
		return items[o.cursor]
	}
	return items[0]
}

// visualizerModeNames returns the display names of all visualizer modes.
func visualizerModeNames() []string {
	return visualizer.ModeNames()
}

// NewOptions creates a new Options modal
func NewOptions(styles *config.ThemeStyles, currentStation, currentBitrate int, showAlbumArt, showSkipWarn, skipDJSegments, copyAlbumArt, notificationsEnabled, notificationsShowArt bool, visMode, colorsFile, themeName string, djSkipAvailable bool) *Options {
	// Find index of current station in stationIDs
	stationIdx := 0
	for i, id := range stationIDs {
		if id == currentStation {
			stationIdx = i
			break
		}
	}

	// Find index of current bitrate in bitrateIDs
	bitrateIdx := 0
	for i, id := range bitrateIDs {
		if id == currentBitrate {
			bitrateIdx = i
			break
		}
	}

	// Find index of current visualizer mode
	visNames := visualizerModeNames()
	visModeIdx := 0
	for i, name := range visNames {
		if name == visMode {
			visModeIdx = i
			break
		}
	}

	// Find index of current theme
	themeIdx := themeFromConfig(colorsFile, themeName)

	return &Options{
		styles:               styles,
		stationIdx:           stationIdx,
		bitrateIdx:           bitrateIdx,
		showAlbumArt:         showAlbumArt,
		showSkipWarn:         showSkipWarn,
		skipDJSegments:       skipDJSegments,
		copyAlbumArt:         copyAlbumArt,
		notificationsEnabled: notificationsEnabled,
		notificationsShowArt: notificationsShowArt,
		visModeIdx:           visModeIdx,
		themeIdx:             themeIdx,
		origStationIdx:       stationIdx,
		origBitrateIdx:       bitrateIdx,
		origAlbumArt:         showAlbumArt,
		origSkipWarn:         showSkipWarn,
		origSkipDJSpeech:     skipDJSegments,
		origCopyArt:          copyAlbumArt,
		origNotifEnabled:     notificationsEnabled,
		origNotifShowArt:     notificationsShowArt,
		origVisModeIdx:       visModeIdx,
		origThemeIdx:         themeIdx,
		djSkipAvailable:      djSkipAvailable,
	}
}

// Update handles messages
func (o *Options) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return OptionsMsg{Closed: true} }

		case "up", "k":
			if o.cursor > 0 {
				o.cursor--
			}

		case "down", "j":
			if o.cursor < len(o.visibleItems())-1 {
				o.cursor++
			}

		case "left", "h":
			o.cycleLeft()

		case "right", "l":
			o.cycleRight()

		case "enter", " ":
			// For toggles, toggle them; for pickers, cycle right
			switch o.currentOptID() {
			case optStation:
				o.cycleRight()
			case optBitrate:
				o.cycleRight()
			case optShowAlbumArt:
				o.showAlbumArt = !o.showAlbumArt
			case optShowSkipWarning:
				o.showSkipWarn = !o.showSkipWarn
			case optSkipDJSpeech:
				o.skipDJSegments = !o.skipDJSegments
			case optCopyAlbumArt:
				o.copyAlbumArt = !o.copyAlbumArt
			case optNotificationsEnabled:
				o.notificationsEnabled = !o.notificationsEnabled
			case optNotificationsShowArt:
				o.notificationsShowArt = !o.notificationsShowArt
			case optVisualizerMode:
				o.cycleRight()
			case optTheme:
				o.cycleRight()
			}

		case "a":
			return o.applyChanges()
		}
	}
	return nil
}

func (o *Options) cycleLeft() {
	switch o.currentOptID() {
	case optStation:
		if o.stationIdx > 0 {
			o.stationIdx--
		} else {
			o.stationIdx = len(stationIDs) - 1
		}
	case optBitrate:
		if o.bitrateIdx > 0 {
			o.bitrateIdx--
		} else {
			o.bitrateIdx = len(bitrateIDs) - 1
		}
	case optShowAlbumArt:
		o.showAlbumArt = !o.showAlbumArt
	case optShowSkipWarning:
		o.showSkipWarn = !o.showSkipWarn
	case optSkipDJSpeech:
		o.skipDJSegments = !o.skipDJSegments
	case optCopyAlbumArt:
		o.copyAlbumArt = !o.copyAlbumArt
	case optNotificationsEnabled:
		o.notificationsEnabled = !o.notificationsEnabled
	case optNotificationsShowArt:
		o.notificationsShowArt = !o.notificationsShowArt
	case optVisualizerMode:
		visNames := visualizerModeNames()
		if o.visModeIdx > 0 {
			o.visModeIdx--
		} else {
			o.visModeIdx = len(visNames) - 1
		}
	case optTheme:
		themeOpts := themeOptions()
		if o.themeIdx > 0 {
			o.themeIdx--
		} else {
			o.themeIdx = len(themeOpts) - 1
		}
	}
}

func (o *Options) cycleRight() {
	switch o.currentOptID() {
	case optStation:
		if o.stationIdx < len(stationIDs)-1 {
			o.stationIdx++
		} else {
			o.stationIdx = 0
		}
	case optBitrate:
		if o.bitrateIdx < len(bitrateIDs)-1 {
			o.bitrateIdx++
		} else {
			o.bitrateIdx = 0
		}
	case optShowAlbumArt:
		o.showAlbumArt = !o.showAlbumArt
	case optShowSkipWarning:
		o.showSkipWarn = !o.showSkipWarn
	case optSkipDJSpeech:
		o.skipDJSegments = !o.skipDJSegments
	case optCopyAlbumArt:
		o.copyAlbumArt = !o.copyAlbumArt
	case optNotificationsEnabled:
		o.notificationsEnabled = !o.notificationsEnabled
	case optNotificationsShowArt:
		o.notificationsShowArt = !o.notificationsShowArt
	case optVisualizerMode:
		visNames := visualizerModeNames()
		if o.visModeIdx < len(visNames)-1 {
			o.visModeIdx++
		} else {
			o.visModeIdx = 0
		}
	case optTheme:
		themeOpts := themeOptions()
		if o.themeIdx < len(themeOpts)-1 {
			o.themeIdx++
		} else {
			o.themeIdx = 0
		}
	}
}

// applyChanges sends OptionsMsg with any changed values
func (o *Options) applyChanges() tea.Cmd {
	stationChanged := o.stationIdx != o.origStationIdx
	bitrateChanged := o.bitrateIdx != o.origBitrateIdx
	albumArtChanged := o.showAlbumArt != o.origAlbumArt
	skipWarnChanged := o.showSkipWarn != o.origSkipWarn
	skipDJSpeechChanged := o.skipDJSegments != o.origSkipDJSpeech
	copyArtChanged := o.copyAlbumArt != o.origCopyArt
	notifEnabledChanged := o.notificationsEnabled != o.origNotifEnabled
	notifShowArtChanged := o.notificationsShowArt != o.origNotifShowArt
	visModeChanged := o.visModeIdx != o.origVisModeIdx
	themeChanged := o.themeIdx != o.origThemeIdx

	if !stationChanged && !bitrateChanged && !albumArtChanged && !skipWarnChanged && !skipDJSpeechChanged && !copyArtChanged && !notifEnabledChanged && !notifShowArtChanged && !visModeChanged && !themeChanged {
		return func() tea.Msg { return OptionsMsg{Closed: true} }
	}

	var msg OptionsMsg
	if stationChanged {
		s := stationIDs[o.stationIdx]
		msg.Station = &s
	}
	if bitrateChanged {
		b := bitrateIDs[o.bitrateIdx]
		msg.Bitrate = &b
	}
	if albumArtChanged {
		v := o.showAlbumArt
		msg.ShowAlbumArt = &v
	}
	if skipWarnChanged {
		v := o.showSkipWarn
		msg.ShowSkipWarn = &v
	}
	if skipDJSpeechChanged {
		v := o.skipDJSegments
		msg.SkipDJSegments = &v
	}
	if copyArtChanged {
		v := o.copyAlbumArt
		msg.CopyAlbumArt = &v
	}
	if notifEnabledChanged {
		v := o.notificationsEnabled
		msg.NotificationsEnabled = &v
	}
	if notifShowArtChanged {
		v := o.notificationsShowArt
		msg.NotificationsShowArt = &v
	}
	if visModeChanged {
		visNames := visualizerModeNames()
		m := visNames[o.visModeIdx]
		msg.VisualizerMode = &m
	}
	if themeChanged {
		themeVal := themeOptionValue(o.themeIdx)
		msg.Theme = &themeVal
	}
	return func() tea.Msg { return msg }
}

// themeOptionValue converts theme option index to config values
// Returns string in format: "colorsFile|themeName" or just "themeName" for built-ins
func themeOptionValue(idx int) string {
	opts := config.ThemeNames()

	if idx == 0 {
		return "CUSTOM" // Signals use colors_file
	}
	if idx == 1 {
		return "OMARCHY" // Signals use omarchy theme
	}
	if idx == 2 {
		return "DEFAULT" // Signals use default fallback chain
	}

	// Built-in theme (adjust for offset of 3)
	builtInIdx := idx - 3
	if builtInIdx >= 0 && builtInIdx < len(opts) {
		return opts[builtInIdx]
	}

	return "DEFAULT"
}

// View renders the modal
func (o Options) View() string {
	modalWidth := 60
	contentWidth := modalWidth - 6

	accentStyle := o.styles.AccentStyle
	mutedStyle := o.styles.MutedStyle
	cursorStyle := o.styles.CursorStyle

	var b strings.Builder

	title := accentStyle.Render("OPTIONS")
	b.WriteString(centerStyled(title, contentWidth))
	b.WriteString("\n\n")

	visNames := visualizerModeNames()
	visModeName := "Bars"
	if o.visModeIdx >= 0 && o.visModeIdx < len(visNames) {
		visModeName = visNames[o.visModeIdx]
	}

	themeOpts := themeOptions()
	themeName := themeOpts[0]
	if o.themeIdx >= 0 && o.themeIdx < len(themeOpts) {
		themeName = themeOpts[o.themeIdx]
	}

	// Build visible items with their option IDs
	type optItem struct {
		id    int
		label string
		value string
	}

	visibleItems := o.visibleItems()
	items := make([]optItem, 0, len(visibleItems))
	for _, id := range visibleItems {
		switch id {
		case optStation:
			items = append(items, optItem{id, "Station", o.renderPicker(config.StationNames[stationIDs[o.stationIdx]], o.cursor == len(items))})
		case optBitrate:
			items = append(items, optItem{id, "Bitrate", o.renderPicker(config.BitrateNames[bitrateIDs[o.bitrateIdx]], o.cursor == len(items))})
		case optShowAlbumArt:
			items = append(items, optItem{id, "Show album art", o.renderToggle(o.showAlbumArt, o.cursor == len(items))})
		case optShowSkipWarning:
			items = append(items, optItem{id, "Show skip warning", o.renderToggle(o.showSkipWarn, o.cursor == len(items))})
		case optSkipDJSpeech:
			items = append(items, optItem{id, "Skip DJ speech", o.renderToggle(o.skipDJSegments, o.cursor == len(items))})
		case optCopyAlbumArt:
			items = append(items, optItem{id, "Copy album art", o.renderToggle(o.copyAlbumArt, o.cursor == len(items))})
		case optNotificationsEnabled:
			items = append(items, optItem{id, "Desktop notifications", o.renderToggle(o.notificationsEnabled, o.cursor == len(items))})
		case optNotificationsShowArt:
			items = append(items, optItem{id, " Show album art", o.renderToggle(o.notificationsShowArt, o.cursor == len(items))})
		case optVisualizerMode:
			items = append(items, optItem{id, "Visualizer mode", o.renderPicker(visModeName, o.cursor == len(items))})
		case optTheme:
			items = append(items, optItem{id, "Theme", o.renderPicker(themeName, o.cursor == len(items))})
		}
	}

	labelColWidth := 22

	for i, item := range items {
		prefix := " "
		label := mutedStyle.Render(item.label)
		if i == o.cursor {
			prefix = cursorStyle.Render("▸ ")
			label = lipgloss.NewStyle().
				Foreground(o.styles.ForegroundStyle.GetForeground()).
				Render(item.label)
		}

		labelVisualWidth := lipgloss.Width(label)
		padCount := labelColWidth - labelVisualWidth
		if padCount < 0 {
			padCount = 0
		}
		row := prefix + label + strings.Repeat(" ", padCount) + item.value
		b.WriteString(row)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)
	b.WriteString(centerStyled(warningStyle.Render("Station/bitrate changes restart playback"), contentWidth))

	b.WriteString("\n\n")
	helpText := accentStyle.Render("←/→") + mutedStyle.Render(" change ") +
		accentStyle.Render("↑/↓") + mutedStyle.Render(" navigate ") +
		accentStyle.Render("a") + mutedStyle.Render(" apply ") +
		accentStyle.Render("esc") + mutedStyle.Render(" close")
	b.WriteString(centerStyled(helpText, contentWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(o.styles.AccentStyle.GetForeground()).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}

// centerStyled centers a string that may contain ANSI styling using visual width
func centerStyled(text string, width int) string {
	visWidth := lipgloss.Width(text)
	if visWidth >= width {
		return text
	}
	pad := (width - visWidth) / 2
	return strings.Repeat(" ", pad) + text
}

// renderPicker renders a value with left/right arrows when selected
func (o *Options) renderPicker(value string, selected bool) string {
	if selected {
		arrow := o.styles.MutedStyle.Render("◂ ")
		arrowR := o.styles.MutedStyle.Render(" ▸")
		val := o.styles.CursorStyle.Render(value)
		return arrow + val + arrowR
	}
	return o.styles.MutedStyle.Render(value)
}

// renderToggle renders a toggle switch
func (o *Options) renderToggle(on bool, selected bool) string {
	var indicator string
	if on {
		indicator = o.styles.AccentStyle.Render("●")
	} else {
		indicator = o.styles.MutedStyle.Render("○")
	}

	if selected {
		return o.styles.CursorStyle.Render("[") + indicator + o.styles.CursorStyle.Render("]")
	}
	return o.styles.MutedStyle.Render("[") + indicator + o.styles.MutedStyle.Render("]")
}
