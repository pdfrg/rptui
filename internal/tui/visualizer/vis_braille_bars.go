package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// brailleBit maps (row, col) in a 4x2 Braille cell to its bit value.
var brailleBit = [4][2]rune{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// renderBrailleBars renders bars where each cell is a single Braille character
// with dots filled bottom-up proportionally to band level. Unlike BarsDot which
// interpolates bands into many columns, this keeps one Braille char per band.
func (v *Visualizer) renderBrailleBars(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)

	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	dotRows := height * 4

	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowNorm := float64(height-1-row) / float64(height)

		var cellStyle lipgloss.Style
		switch {
		case rowNorm >= 0.6:
			cellStyle = highStyle
		case rowNorm >= 0.3:
			cellStyle = midStyle
		default:
			cellStyle = lowStyle
		}

		for range leftPad {
			b.WriteString(" ")
		}
		for i, level := range v.bands {
			for range bandWidth {
				braille := '\u2800'
				for dr := range 4 {
					for dc := range 2 {
						dotRow := row*4 + dr
						dotY := float64(dotRows-1-dotRow) / float64(dotRows)
						if dotY < level {
							braille |= brailleBit[dr][dc]
						}
					}
				}
				if braille == '\u2800' {
					b.WriteString(" ")
				} else {
					b.WriteString(cellStyle.Render(string(braille)))
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
