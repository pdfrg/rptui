package visualizer

import (
	"math"
	"strings"
)

// renderRetro renders an 80s synthwave perspective grid with a waveform horizon.
func (v *Visualizer) renderRetro(width int) string {
	height := v.rows
	lines := make([]string, height)

	horizonRow := height / 3
	if horizonRow < 1 {
		horizonRow = 1
	}

	for row := range height {
		var b strings.Builder

		if row <= horizonRow {
			// Sky area — draw a subtle wave at the horizon
			if row == horizonRow {
				for col := range width {
					wave := math.Sin(float64(col)*0.2+float64(v.frame)*0.1) * 0.3
					for _, band := range v.bands {
						wave += (band - 0.5) * 0.2
					}
					if wave > 0.1 {
						b.WriteString("═")
					} else {
						b.WriteString(" ")
					}
				}
			} else {
				for range width {
					b.WriteString(" ")
				}
			}
		} else {
			// Ground grid with perspective
			depth := float64(row-horizonRow) / float64(height-horizonRow)
			vSpacing := int(1.0 / (depth*0.5 + 0.1))
			if vSpacing < 1 {
				vSpacing = 1
			}

			for col := range width {
				// Vertical lines with perspective convergence
				center := float64(width) / 2
				offset := float64(col) - center
				perspectiveX := offset / (depth*2 + 0.5)
				gridCol := int(perspectiveX+center) % vSpacing

				// Horizontal lines
				hSpacing := int(1.0/(depth*3+0.2)) + 1
				isHLine := (row-horizonRow)%max(1, hSpacing) == 0

				if gridCol == 0 || isHLine {
					if isHLine && gridCol == 0 {
						b.WriteString("┼")
					} else if isHLine {
						b.WriteString("─")
					} else {
						b.WriteString("│")
					}
				} else {
					b.WriteString(" ")
				}
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
