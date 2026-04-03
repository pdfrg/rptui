package visualizer

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderWave renders a waveform oscilloscope using Braille patterns
// with theme-based coloring.
func (v *Visualizer) renderWave(width int) string {
	height := v.rows
	if height < 3 {
		height = 3
	}

	brailleH := height * 4
	brailleW := width * 2

	grid := make([][]bool, brailleH)
	for row := range grid {
		grid[row] = make([]bool, brailleW)
	}

	for col := range brailleW {
		x := float64(col) / float64(brailleW)
		y := 0.0

		y += math.Sin(x*math.Pi*6+float64(v.frame)*0.15) * 0.2

		for i, band := range v.bands {
			freq := float64(i+1) * 1.5
			phase := float64(i) * 0.8
			y += (band - 0.5) * math.Sin(x*math.Pi*freq+float64(v.frame)*0.1+phase) * 0.15
		}

		rowF := float64(brailleH)/2 + y*float64(brailleH)*0.8
		row := int(rowF)
		if row >= 0 && row < brailleH {
			grid[row][col] = true
			if row > 0 {
				grid[row-1][col] = true
			}
			if row < brailleH-1 {
				grid[row+1][col] = true
			}
		}
	}

	waveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		for col := range width {
			bit := 0
			for br := 0; br < 4; br++ {
				for bc := 0; bc < 2; bc++ {
					gridRow := row*4 + br
					gridCol := col*2 + bc
					if gridRow < brailleH && gridCol < brailleW && grid[gridRow][gridCol] {
						bit |= int(brailleBit[br][bc])
					}
				}
			}
			if bit == 0 {
				b.WriteString(" ")
			} else {
				b.WriteString(waveStyle.Render(string(rune(0x2800 + bit))))
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
