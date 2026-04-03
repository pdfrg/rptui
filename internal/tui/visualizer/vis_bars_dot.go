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
func (v *Visualizer) renderBarsDot(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	// Braille gives us 2x horizontal resolution
	totalCols := width / 2
	if totalCols < bandCount {
		totalCols = bandCount
	}

	// Interpolate bands to column count
	cols := interpolateBands(v.bands, totalCols)

	brailleRows := height * 2
	lines := make([]string, brailleRows)

	for brow := range brailleRows {
		var b strings.Builder
		for col := range totalCols {
			level := cols[col]
			// Each Braille cell covers 2 rows of our display
			cellRow := brow % 4
			cellCol := 0
			rowLevel := float64(brailleRows-1-brow) / float64(brailleRows)

			bit := 0
			if level > rowLevel {
				bit = brailleBits[cellRow][cellCol]
			}
			if bit == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(rune(0x2800 + bit))
			}
		}
		lines[brow] = b.String()
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
