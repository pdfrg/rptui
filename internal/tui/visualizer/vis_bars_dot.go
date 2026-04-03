package visualizer

import "strings"

// brailleBits maps (row, col) in a 4x2 Braille cell to its bit value.
var brailleBits = [4][2]int{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// renderBarsDot renders spectrum bars with Braille dot stipple for sub-cell resolution.
// Each Braille character covers 4 terminal rows x 2 columns, giving fine vertical resolution.
func (v *Visualizer) renderBarsDot(width int) string {
	height := v.rows
	bandCount := len(v.bands)

	// Interpolate bands to pixel columns (2x horizontal from Braille)
	totalCols := width * 2
	if totalCols < bandCount {
		totalCols = bandCount
	}
	cols := interpolateBands(v.bands, totalCols)

	// Each Braille character = 4 rows, so we need height/4 characters vertically
	// But we render at terminal row granularity: height rows
	brailleH := height * 4 // sub-row resolution
	lines := make([]string, height)

	for row := range height {
		var b strings.Builder
		for col := range width {
			bit := 0
			for br := 0; br < 4; br++ {
				// Map Braille sub-row to actual level
				subRow := row*4 + br
				rowLevel := float64(brailleH-1-subRow) / float64(brailleH)

				// Two horizontal sub-columns per Braille char
				for bc := 0; bc < 2; bc++ {
					pixelCol := col*2 + bc
					if pixelCol >= len(cols) {
						continue
					}
					level := cols[pixelCol]
					if level > rowLevel {
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
