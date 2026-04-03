package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderBinary draws streaming columns of 0s and 1s that scroll at speeds
// proportional to each band's energy. Higher energy = more 1s and faster flow.
func (v *Visualizer) renderBinary(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)

	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorDim))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
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
				if ch == '1' && energy > 0.4 {
					style = highStyle
				} else if ch == '1' || energy > 0.3 {
					style = midStyle
				} else {
					style = lowStyle
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
