package visualizer

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderWave renders a waveform oscilloscope using Braille patterns.
// Uses the raw audio samples when available for a true oscilloscope display,
// falling back to band-driven synthesis when samples aren't available.
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

	// Try to use raw audio samples for a true oscilloscope
	if v.audioTap != nil && len(v.sampleBuf) >= brailleW {
		v.renderWaveFromSamples(grid, brailleW, brailleH)
	} else {
		v.renderWaveFromBands(grid, brailleW, brailleH)
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

// renderWaveFromSamples draws the actual audio waveform from raw samples.
func (v *Visualizer) renderWaveFromSamples(grid [][]bool, brailleW, brailleH int) {
	mid := float64(brailleH) / 2

	for col := 0; col < brailleW; col++ {
		// Map column to sample index
		sampleIdx := col * len(v.sampleBuf) / brailleW
		if sampleIdx >= len(v.sampleBuf) {
			sampleIdx = len(v.sampleBuf) - 1
		}
		sample := float64(v.sampleBuf[sampleIdx])

		// Map sample (-1 to 1) to row position
		rowF := mid - sample*mid*0.9
		row := int(rowF)

		// Light the pixel and neighbors for thickness
		for dr := -1; dr <= 1; dr++ {
			r := row + dr
			if r >= 0 && r < brailleH {
				grid[r][col] = true
			}
		}
	}
}

// renderWaveFromBands synthesizes a wave from frequency band data.
func (v *Visualizer) renderWaveFromBands(grid [][]bool, brailleW, brailleH int) {
	// Calculate overall energy to scale amplitude
	energy := 0.0
	for _, b := range v.bands {
		energy += b
	}
	energy /= float64(len(v.bands))
	amplitude := 0.3 + energy*0.7 // scale 0.3-1.0 based on energy

	for col := 0; col < brailleW; col++ {
		x := float64(col) / float64(brailleW)
		y := 0.0

		// Subtle base wave for visual interest
		y += math.Sin(x*math.Pi*4+float64(v.frame)*0.12) * 0.05 * amplitude

		// Modulate by bands — much stronger contribution
		for i, band := range v.bands {
			freq := float64(i+1) * 2.0
			phase := float64(i) * 0.6
			y += (band - 0.5) * math.Sin(x*math.Pi*freq+float64(v.frame)*0.08+phase) * 0.4 * amplitude
		}

		mid := float64(brailleH) / 2
		rowF := mid + y*float64(brailleH)*0.45
		row := int(rowF)

		for dr := -1; dr <= 1; dr++ {
			r := row + dr
			if r >= 0 && r < brailleH {
				grid[r][col] = true
			}
		}
	}
}
