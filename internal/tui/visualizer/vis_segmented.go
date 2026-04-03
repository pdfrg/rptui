package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderSegmented draws bars with the same width as Bars, but each bar is
// broken into horizontal blocks with small gaps between them, like a
// segmented LED display.
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
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)

		var cellStyle lipgloss.Style
		switch {
		case rowBottom >= 0.6:
			cellStyle = highStyle
		case rowBottom >= 0.3:
			cellStyle = midStyle
		default:
			cellStyle = lowStyle
		}

		for range leftPad {
			b.WriteString(" ")
		}
		for i, level := range v.bands {
			block := fracBlock(level, rowBottom, rowTop)
			if block == " " {
				for range bandWidth {
					b.WriteString(" ")
				}
			} else {
				// Every other row gets a space gap, creating segmented look
				if row%2 == 0 {
					s := ""
					for range bandWidth {
						s += block
					}
					b.WriteString(cellStyle.Render(s))
				} else {
					b.WriteString(strings.Repeat(" ", bandWidth))
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
