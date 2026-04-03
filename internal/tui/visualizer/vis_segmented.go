package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderSegmented draws many thin interpolated columns for a dense organic look.
// Bands are interpolated across the full width so adjacent columns vary slightly.
func (v *Visualizer) renderSegmented(width int) string {
	height := v.rows

	// Interpolate bands across all available columns
	totalCols := width
	cols := interpolateBands(v.bands, totalCols)

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

		for col := range totalCols {
			level := cols[col]
			block := fracBlock(level, rowBottom, rowTop)
			if block == " " {
				b.WriteString(" ")
			} else {
				b.WriteString(cellStyle.Render(block))
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
