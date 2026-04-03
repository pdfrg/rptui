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
	groundRows := height - horizonRow
	if groundRows < 2 {
		groundRows = 2
	}

	bassEnergy := 0.0
	if len(v.bands) > 0 {
		bassEnergy = v.bands[0]
	}

	waveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))
	gridStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))

	// Horizontal scroll: 3 rows per second at 20 FPS
	scrollY := float64(v.frame) * 0.15

	for row := range height {
		var b strings.Builder

		if row < horizonRow {
			// Sky: twinkling stars
			for col := range width {
				seed := uint64(row*1000 + col*7 + 42)
				seed += uint64(v.frame) * 137
				seed = seed*6364136223846793005 + 1442695040888963407
				if (seed>>62)%4 == 0 {
					b.WriteString("·")
				} else {
					b.WriteString(" ")
				}
			}
		} else if row == horizonRow {
			// Horizon wave — audio-reactive
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
			// Ground: perspective grid
			// Use a "distance" coordinate that increases toward the viewer
			// This gives us the perspective effect
			dist := float64(row-horizonRow) / float64(groundRows) // 0 at horizon, 1 at bottom

			// Vertical lines: fixed convergence to center
			// Only draw a vertical line every N columns, where N shrinks toward bottom
			vPeriod := max(4, int(1.0/(dist*0.2+0.08)))

			// Horizontal lines: scroll downward
			// Space between lines shrinks toward bottom
			hPeriod := max(2, int(1.0/(dist*1.5+0.3)))

			// Scrolled position determines which rows get horizontal lines
			scrolledPos := float64(row-horizonRow) + scrollY
			isHLine := int(math.Mod(scrolledPos, float64(hPeriod))) == 0

			for col := range width {
				center := float64(width) / 2
				offset := float64(col) - center
				// Perspective: vertical lines converge toward center
				perspectiveX := offset / (dist*1.5 + 0.5)
				gridCol := int(math.Mod(perspectiveX+center, float64(vPeriod)))
				if gridCol < 0 {
					gridCol += vPeriod
				}
				isVLine := gridCol == 0

				if isHLine && isVLine {
					b.WriteString(gridStyle.Render("┼"))
				} else if isHLine {
					b.WriteString(gridStyle.Render("─"))
				} else if isVLine {
					if bassEnergy > 0.4 && (v.frame%6) < 3 {
						b.WriteString(waveStyle.Render("│"))
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
