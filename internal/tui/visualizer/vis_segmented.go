package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderSegmented draws solid block bars with visible gaps between rows.
// Uses half-height blocks (▄) so each "brick" is half a terminal row,
// with blank gaps between them — like cliamp's Bricks visualizer.
func (v *Visualizer) renderSegmented(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)

	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowThreshold := float64(height-1-row) / float64(height)

		var cellStyle lipgloss.Style
		switch {
		case rowThreshold >= 0.6:
			cellStyle = highStyle
		case rowThreshold >= 0.3:
			cellStyle = midStyle
		default:
			cellStyle = lowStyle
		}

		for range leftPad {
			b.WriteString(" ")
		}
		for i, level := range v.bands {
			if level > rowThreshold {
				s := ""
				for range bandWidth {
					s += "▄"
				}
				b.WriteString(cellStyle.Render(s))
			} else {
				for range bandWidth {
					b.WriteString(" ")
				}
			}
			if i < bandCount-1 {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
