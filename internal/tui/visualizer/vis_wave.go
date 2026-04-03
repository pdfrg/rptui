package visualizer

import (
	"math"
	"strings"
)

// renderWave renders a waveform oscilloscope using Braille patterns.
func (v *Visualizer) renderWave(width int) string {
	height := v.rows
	if height < 3 {
		height = 3
	}

	// Braille gives us 4x vertical and 2x horizontal resolution
	brailleH := height * 4
	brailleW := width * 2

	// Build a grid of which dots should be lit
	grid := make([][]bool, brailleH)
	for row := range grid {
		grid[row] = make([]bool, brailleW)
	}

	// Draw waveform by sampling sine waves modulated by band levels
	for col := range brailleW {
		x := float64(col) / float64(brailleW)
		y := 0.0

		// Base wave
		y += math.Sin(x*math.Pi*6+float64(v.frame)*0.15) * 0.2

		// Modulate by bands
		for i, band := range v.bands {
			freq := float64(i+1) * 1.5
			phase := float64(i) * 0.8
			y += (band - 0.5) * math.Sin(x*math.Pi*freq+float64(v.frame)*0.1+phase) * 0.15
		}

		// Map y (-0.5 to 0.5) to braille row
		rowF := float64(brailleH)/2 + y*float64(brailleH)*0.8
		row := int(rowF)
		if row >= 0 && row < brailleH {
			grid[row][col] = true
			// Also light adjacent dots for a thicker line
			if row > 0 {
				grid[row-1][col] = true
			}
			if row < brailleH-1 {
				grid[row+1][col] = true
			}
		}
	}

	// Convert grid to Braille characters
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
						bit |= brailleBits[br][bc]
					}
				}
			}
			if bit == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(rune(0x2800 + bit))
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
