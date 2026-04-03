package visualizer

import "strings"

// renderBars renders smooth spectrum bars with fractional Unicode blocks.
func (v *Visualizer) renderBars(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)
	if bandWidth < 1 {
		bandWidth = 1
	}

	// Calculate total content width and center it
	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)

		for range leftPad {
			b.WriteString(" ")
		}
		for i, level := range v.bands {
			block := fracBlock(level, rowBottom, rowTop)
			for range bandWidth {
				b.WriteString(block)
			}
			if i < bandCount-1 {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// fracBlock returns a Unicode block character representing the fractional
// coverage between rowBottom and rowTop for the given level.
func fracBlock(level, rowBottom, rowTop float64) string {
	if level <= rowBottom {
		return " "
	}
	if level >= rowTop {
		return "█"
	}
	frac := (level - rowBottom) / (rowTop - rowBottom)
	idx := int(frac*7) + 1
	if idx > 7 {
		idx = 7
	}
	if idx < 1 {
		idx = 1
	}
	return barBlocks[idx]
}
