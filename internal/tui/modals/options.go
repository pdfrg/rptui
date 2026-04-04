// Package modals provides modal dialogs for the TUI
package modals

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/tui/visualizer"
)

// OptionsMsg is sent when user applies options or closes the modal
type OptionsMsg struct {
	Station              *int
	Bitrate              *int
	ShowAlbumArt         *bool
	ShowSkipWarn         *bool
	CopyAlbumArt         *bool
	NotificationsEnabled *bool
	NotificationsShowArt *bool
	VisualizerMode       *string
	Closed               bool
}

// Option item types
const (
	optStation = iota
	optBitrate
	optShowAlbumArt
	optShowSkipWarning
	optCopyAlbumArt
	optNotificationsEnabled
	optNotificationsShowArt
	optVisualizerMode
	optCount // number of items
)

// Valid station IDs in order (skipping 4)
var stationIDs = []int{0, 1, 2, 3, 5}

// Valid bitrate IDs in order
var bitrateIDs = []int{1, 2, 3, 4}

// Options modal for settings
type Options struct {
	styles *config.ThemeStyles
	cursor int // which row is selected

	// Current values (editable)
	stationIdx           int // index into stationIDs
	bitrateIdx           int // index into bitrateIDs
	showAlbumArt         bool
	showSkipWarn         bool
	copyAlbumArt         bool
	notificationsEnabled bool
	notificationsShowArt bool
	visModeIdx           int // index into visualizer mode names

	// Original values for change detection
	origStationIdx   int
	origBitrateIdx   int
	origAlbumArt     bool
	origSkipWarn     bool
	origCopyArt      bool
	origNotifEnabled bool
	origNotifShowArt bool
	origVisModeIdx   int
}

// visualizerModeNames returns the display names of all visualizer modes.
func visualizerModeNames() []string {
	return visualizer.ModeNames()
}

// NewOptions creates a new Options modal
func NewOptions(styles *config.ThemeStyles, currentStation, currentBitrate int, showAlbumArt, showSkipWarn, copyAlbumArt, notificationsEnabled, notificationsShowArt bool, visMode string) *Options {
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

	return &Options{
		styles:               styles,
		stationIdx:           stationIdx,
		bitrateIdx:           bitrateIdx,
		showAlbumArt:         showAlbumArt,
		showSkipWarn:         showSkipWarn,
		copyAlbumArt:         copyAlbumArt,
		notificationsEnabled: notificationsEnabled,
		notificationsShowArt: notificationsShowArt,
		visModeIdx:           visModeIdx,
		origStationIdx:       stationIdx,
		origBitrateIdx:       bitrateIdx,
		origAlbumArt:         showAlbumArt,
		origSkipWarn:         showSkipWarn,
		origCopyArt:          copyAlbumArt,
		origNotifEnabled:     notificationsEnabled,
		origNotifShowArt:     notificationsShowArt,
		origVisModeIdx:       visModeIdx,
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
			if o.cursor < optCount-1 {
				o.cursor++
			}

		case "left", "h":
			o.cycleLeft()

		case "right", "l":
			o.cycleRight()

		case "enter", " ":
			// For toggles, toggle them; for pickers, cycle right
			switch o.cursor {
			case optStation:
				o.cycleRight()
			case optBitrate:
				o.cycleRight()
			case optShowAlbumArt:
				o.showAlbumArt = !o.showAlbumArt
			case optShowSkipWarning:
				o.showSkipWarn = !o.showSkipWarn
			case optCopyAlbumArt:
				o.copyAlbumArt = !o.copyAlbumArt
			case optNotificationsEnabled:
				o.notificationsEnabled = !o.notificationsEnabled
			case optNotificationsShowArt:
				o.notificationsShowArt = !o.notificationsShowArt
			case optVisualizerMode:
				o.cycleRight()
			}

		case "a":
			return o.applyChanges()
		}
	}
	return nil
}

func (o *Options) cycleLeft() {
	switch o.cursor {
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
	}
}

func (o *Options) cycleRight() {
	switch o.cursor {
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
	}
}

// applyChanges sends OptionsMsg with any changed values
func (o *Options) applyChanges() tea.Cmd {
	stationChanged := o.stationIdx != o.origStationIdx
	bitrateChanged := o.bitrateIdx != o.origBitrateIdx
	albumArtChanged := o.showAlbumArt != o.origAlbumArt
	skipWarnChanged := o.showSkipWarn != o.origSkipWarn
	copyArtChanged := o.copyAlbumArt != o.origCopyArt
	notifEnabledChanged := o.notificationsEnabled != o.origNotifEnabled
	notifShowArtChanged := o.notificationsShowArt != o.origNotifShowArt
	visModeChanged := o.visModeIdx != o.origVisModeIdx

	if !stationChanged && !bitrateChanged && !albumArtChanged && !skipWarnChanged && !copyArtChanged && !notifEnabledChanged && !notifShowArtChanged && !visModeChanged {
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
	return func() tea.Msg { return msg }
}

// View renders the modal
func (o Options) View() string {
	modalWidth := 60
	// Inner content width (minus border 2 + padding 2*2 = 6)
	contentWidth := modalWidth - 6

	accentStyle := o.styles.AccentStyle
	mutedStyle := o.styles.MutedStyle
	cursorStyle := o.styles.CursorStyle

	var b strings.Builder

	// Title
	title := accentStyle.Render("OPTIONS")
	b.WriteString(centerStyled(title, contentWidth))
	b.WriteString("\n\n")

	// Render each option row
	visNames := visualizerModeNames()
	visModeName := "Bars"
	if o.visModeIdx >= 0 && o.visModeIdx < len(visNames) {
		visModeName = visNames[o.visModeIdx]
	}

	items := []struct {
		label string
		value string
	}{
		{"Station", o.renderPicker(config.StationNames[stationIDs[o.stationIdx]], o.cursor == optStation)},
		{"Bitrate", o.renderPicker(config.BitrateNames[bitrateIDs[o.bitrateIdx]], o.cursor == optBitrate)},
		{"Show album art", o.renderToggle(o.showAlbumArt, o.cursor == optShowAlbumArt)},
		{"Show skip warning", o.renderToggle(o.showSkipWarn, o.cursor == optShowSkipWarning)},
		{"Copy album art", o.renderToggle(o.copyAlbumArt, o.cursor == optCopyAlbumArt)},
		{"Desktop notifications", o.renderToggle(o.notificationsEnabled, o.cursor == optNotificationsEnabled)},
		{"  Show album art", o.renderToggle(o.notificationsShowArt, o.cursor == optNotificationsShowArt)},
		{"Visualizer mode", o.renderPicker(visModeName, o.cursor == optVisualizerMode)},
	}

	labelColWidth := 22

	for i, item := range items {
		prefix := "  "
		label := mutedStyle.Render(item.label)
		if i == o.cursor {
			prefix = cursorStyle.Render("▸ ")
			label = lipgloss.NewStyle().
				Foreground(lipgloss.Color(o.styles.Foreground)).
				Render(item.label)
		}

		// Pad label to fixed visual width using lipgloss-aware width
		labelVisualWidth := lipgloss.Width(label)
		padCount := labelColWidth - labelVisualWidth
		if padCount < 0 {
			padCount = 0
		}
		row := prefix + label + strings.Repeat(" ", padCount) + item.value
		b.WriteString(row)
		b.WriteString("\n")
	}

	// Warning text
	b.WriteString("\n")
	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")).
		Bold(true)
	b.WriteString(centerStyled(warningStyle.Render("Station/bitrate changes restart playback"), contentWidth))

	// Help text
	b.WriteString("\n\n")
	helpText := accentStyle.Render("←/→") + mutedStyle.Render(" change  ") +
		accentStyle.Render("↑/↓") + mutedStyle.Render(" navigate  ") +
		accentStyle.Render("a") + mutedStyle.Render(" apply  ") +
		accentStyle.Render("esc") + mutedStyle.Render(" close")
	b.WriteString(centerStyled(helpText, contentWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(o.styles.Accent)).
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
