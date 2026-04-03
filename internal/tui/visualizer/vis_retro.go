package visualizer

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderRetro renders an 80s synthwave perspective grid with a scrolling ground
// and audio-modulated horizon wave, using theme colors.
func (v *Visualizer) renderRetro(width int) string {
	height := v.rows
	lines := make([]string, height)

	horizonRow := height / 3
	if horizonRow < 1 {
		horizonRow = 1
	}

	scrollY := float64(v.frame) * 0.73

	waveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))
	gridStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))

	for row := range height {
		var b strings.Builder

		if row <= horizonRow {
			if row == horizonRow {
				for col := range width {
					wave := math.Sin(float64(col)*0.15+float64(v.frame)*0.12) * 0.3
					bandIdx := col * len(v.bands) / width
					if bandIdx < len(v.bands) {
						wave += (v.bands[bandIdx] - 0.5) * 0.6
					}
					if wave > 0.1 {
						b.WriteString(waveStyle.Render("═"))
					} else if wave > -0.1 {
						b.WriteString(waveStyle.Render("─"))
					} else {
						b.WriteString(" ")
					}
				}
			} else {
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
			depth := float64(row-horizonRow) / float64(height-horizonRow)
			vSpacing := int(1.0 / (depth*0.5 + 0.1))
			if vSpacing < 1 {
				vSpacing = 1
			}

			for col := range width {
				center := float64(width) / 2
				offset := float64(col) - center
				perspectiveX := offset / (depth*2 + 0.5)
				gridCol := int(perspectiveX+center) % vSpacing

				hSpacing := int(1.0/(depth*3+0.2)) + 1
				scrolledRow := float64(row-horizonRow) + scrollY
				isHLine := int(scrolledRow)%max(1, hSpacing) == 0

				isBottom := row >= height-2
				if isBottom {
					b.WriteString(" ")
					continue
				}

				if gridCol == 0 || isHLine {
					if isHLine && gridCol == 0 {
						b.WriteString(gridStyle.Render("┼"))
					} else if isHLine {
						b.WriteString(gridStyle.Render("─"))
					} else {
						b.WriteString(gridStyle.Render("│"))
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
