package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderBinary draws streaming columns of 0s and 1s that scroll at speeds
// proportional to each band's energy. Higher energy = more 1s and faster flow.
// Uses accent/cursor gradient: dim 0s, bright 1s.
func (v *Visualizer) renderBinary(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)

	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	// 0s use muted/dim, 1s use accent→cursor gradient based on energy
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorDim))
	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		col := 0

		for range leftPad {
			b.WriteString(" ")
		}
		for i := range bandCount {
			energy := v.bands[i]
			for range bandWidth {
				speed := max(1, 4-int(energy*3))
				scroll := int(v.frame) / speed

				h := scatterHash(i, row+scroll, col, 0)
				oneProb := energy*0.6 + 0.15

				var ch byte
				if h < oneProb {
					ch = '1'
				} else {
					ch = '0'
				}

				var style lipgloss.Style
				if ch == '1' {
					if energy > 0.5 {
						style = highStyle
					} else {
						style = lowStyle
					}
				} else {
					style = dimStyle
				}
				b.WriteString(style.Render(string(ch)))
				col++
			}
			if i < bandCount-1 {
				b.WriteString(" ")
				col++
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
