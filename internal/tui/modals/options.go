// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/config"
)

// OptionsMsg is sent when user selects an option or closes the modal
type OptionsMsg struct {
	Station *int
	Bitrate *int
	Closed  bool
}

// Options modal for settings
type Options struct {
	styles         *config.ThemeStyles
	width          int
	height         int
	cursor         int // 0 to 1 (Station, Bitrate)
	stationCursor  int
	bitrateCursor  int
	isSubSelection bool
}

// NewOptions creates a new Options modal
func NewOptions(styles *config.ThemeStyles, currentStation, currentBitrate int) *Options {
	return &Options{
		styles:        styles,
		stationCursor: currentStation,
		bitrateCursor: currentBitrate,
	}
}

// Update handles messages
func (o *Options) Update(msg tea.Msg) (tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return OptionsMsg{Closed: true} }
		case "up", "k":
			if o.isSubSelection {
				if o.cursor == 0 { // Station
					if o.stationCursor > 0 {
						o.stationCursor--
					}
				} else { // Bitrate
					if o.bitrateCursor > 1 {
						o.bitrateCursor--
					}
				}
			} else {
				if o.cursor > 0 {
					o.cursor--
				}
			}
		case "down", "j":
			if o.isSubSelection {
				if o.cursor == 0 { // Station
					if o.stationCursor < 5 { // 0, 1, 2, 3, 5 are valid
						o.stationCursor++
						if o.stationCursor == 4 { // Skip 4
							o.stationCursor = 5
						}
					}
				} else { // Bitrate
					if o.bitrateCursor < 6 {
						o.bitrateCursor++
					}
				}
			} else {
				if o.cursor < 1 {
					o.cursor++
				}
			}
		case "enter", "right", "l":
			if !o.isSubSelection {
				o.isSubSelection = true
			} else {
				// Apply and close
				if o.cursor == 0 {
					s := o.stationCursor
					return func() tea.Msg { return OptionsMsg{Station: &s} }
				} else {
					b := o.bitrateCursor
					return func() tea.Msg { return OptionsMsg{Bitrate: &b} }
				}
			}
		case "left", "h":
			if o.isSubSelection {
				o.isSubSelection = false
			}
		}
	}
	return nil
}

// View renders the modal
func (o Options) View() string {
	modalWidth := 40
	modalHeight := 12

	var b strings.Builder
	b.WriteString(centerText("OPTIONS", modalWidth))
	b.WriteString("\n\n")

	// Station selection
	stationLabel := "Station"
	if o.cursor == 0 {
		stationLabel = o.styles.AccentStyle.Render("> Station")
	}
	
	stationValue := config.StationNames[o.stationCursor]
	if stationValue == "" {
		stationValue = fmt.Sprintf("Station %d", o.stationCursor)
	}
	
	if o.cursor == 0 && o.isSubSelection {
		stationValue = o.styles.CursorStyle.Render("[ " + stationValue + " ]")
	}

	b.WriteString(fmt.Sprintf(" %-15s %s\n", stationLabel, stationValue))

	// Bitrate selection
	bitrateLabel := "Bitrate"
	if o.cursor == 1 {
		bitrateLabel = o.styles.AccentStyle.Render("> Bitrate")
	}
	
	bitrateValue := config.BitrateNames[o.bitrateCursor]
	if bitrateValue == "" {
		bitrateValue = fmt.Sprintf("Bitrate %d", o.bitrateCursor)
	}

	if o.cursor == 1 && o.isSubSelection {
		bitrateValue = o.styles.CursorStyle.Render("[ " + bitrateValue + " ]")
	}

	b.WriteString(fmt.Sprintf(" %-15s %s\n", bitrateLabel, bitrateValue))
	b.WriteString("\n\n")
	b.WriteString(centerText("Use arrows/h/j/k/l to navigate", modalWidth-4))
	b.WriteString("\n")
	b.WriteString(centerText("Enter to select, Esc to close", modalWidth-4))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(o.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth).
		Height(modalHeight)

	return modalStyle.Render(b.String())
}
