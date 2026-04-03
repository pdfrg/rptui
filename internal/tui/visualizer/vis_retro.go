package visualizer

import (
	"math"
	"strings"
)

// renderRetro renders an 80s synthwave perspective grid with a scrolling ground
// and audio-modulated horizon wave.
func (v *Visualizer) renderRetro(width int) string {
	height := v.rows
	lines := make([]string, height)

	horizonRow := height / 3
	if horizonRow < 1 {
		horizonRow = 1
	}
	groundRows := height - horizonRow

	// Smooth vertical scroll offset for the ground grid
	scrollOffset := float64(v.frame) * 0.4

	for row := range height {
		var b strings.Builder

		if row <= horizonRow {
			// Sky area — draw an audio-modulated wave at the horizon
			if row == horizonRow {
				for col := range width {
					wave := math.Sin(float64(col)*0.15+float64(v.frame)*0.12) * 0.3
					bandIdx := col * len(v.bands) / width
					if bandIdx < len(v.bands) {
						wave += (v.bands[bandIdx] - 0.5) * 0.6
					}
					if wave > 0.1 {
						b.WriteString("═")
					} else if wave > -0.1 {
						b.WriteString("─")
					} else {
						b.WriteString(" ")
					}
				}
			} else {
				// Stars — use frame to twinkle
				for col := range width {
					seed := uint64(row*1000+col*7+42) + uint64(v.frame)/10
					seed = seed*6364136223846793005 + 1442695040888963407
					if (seed>>60)%4 == 0 {
						b.WriteString("·")
					} else {
						b.WriteString(" ")
					}
				}
			}
		} else {
			// Ground grid with perspective
			depthF := float64(row-horizonRow) / float64(groundRows)

			// Vertical line spacing based on perspective
			vSpacing := max(1, int(1.0/(depthF*0.5+0.1)))

			for col := range width {
				center := float64(width) / 2
				offset := float64(col) - center
				perspectiveX := offset / (depthF*2 + 0.5)
				gridCol := int(perspectiveX+center) % vSpacing
				if gridCol < 0 {
					gridCol += vSpacing
				}

				// Horizontal lines with smooth scroll
				hSpacing := max(1, int(1.0/(depthF*3+0.2))+1)
				// Add scroll offset to row position for smooth vertical movement
				scrolledRow := float64(row-horizonRow) + scrollOffset*float64(hSpacing)*0.05
				isHLine := int(scrolledRow)%hSpacing == 0

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
