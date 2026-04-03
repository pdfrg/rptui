package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderBarsDot renders spectrum bars with Braille dot stipple for sub-cell resolution.
// Each Braille character covers 4 terminal rows x 2 columns.
func (v *Visualizer) renderBarsDot(width int) string {
	height := v.rows
	bandCount := len(v.bands)

	// Interpolate bands to pixel columns (2x horizontal from Braille)
	totalCols := width * 2
	if totalCols < bandCount {
		totalCols = bandCount
	}
	cols := interpolateBands(v.bands, totalCols)

	brailleH := height * 4

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

		for col := range width {
			bit := 0
			for br := 0; br < 4; br++ {
				subRow := row*4 + br
				rowLevel := float64(brailleH-1-subRow) / float64(brailleH)

				for bc := 0; bc < 2; bc++ {
					pixelCol := col*2 + bc
					if pixelCol >= len(cols) {
						continue
					}
					level := cols[pixelCol]
					if level > rowLevel {
						bit |= int(brailleBit[br][bc])
					}
				}
			}
			if bit == 0 {
				b.WriteString(" ")
			} else {
				b.WriteString(cellStyle.Render(string(rune(0x2800 + bit))))
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// interpolateBands linearly interpolates band values to the given column count.
func interpolateBands(bands []float64, cols int) []float64 {
	if cols <= 0 || len(bands) == 0 {
		return nil
	}
	if len(bands) == cols {
		out := make([]float64, len(bands))
		copy(out, bands)
		return out
	}
	out := make([]float64, cols)
	if cols == 1 {
		sum := 0.0
		for _, v := range bands {
			sum += v
		}
		out[0] = sum / float64(len(bands))
		return out
	}
	last := float64(len(bands) - 1)
	for col := range cols {
		pos := float64(col) / float64(cols-1) * last
		idx := int(pos)
		frac := pos - float64(idx)
		if idx >= len(bands)-1 {
			out[col] = bands[len(bands)-1]
		} else {
			out[col] = bands[idx]*(1-frac) + bands[idx+1]*frac
		}
	}
	return out
}
