package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderRain draws many thin rain-drop columns across the full terminal width.
// Each column has animated falling drops with head/body/tail coloring.
func (v *Visualizer) renderRain(width int) string {
	height := v.rows

	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowNorm := float64(height-1-row) / float64(height)

		for col := range width {
			// Average band level for this column position
			bandIdx := col * len(v.bands) / width
			if bandIdx >= len(v.bands) {
				bandIdx = len(v.bands) - 1
			}
			level := v.bands[bandIdx]

			if rowNorm >= level {
				b.WriteString(" ")
				continue
			}

			seed := uint64(col)*7919 + 104729
			if scatterHash(bandIdx, 0, col, v.frame/12) > level*1.6+0.1 {
				b.WriteString(" ")
				continue
			}

			speed := 1 + int(seed%3)
			dropLen := 2 + int((seed/7)%3)
			cycleLen := height + dropLen + 3
			offset := int((seed / 13) % uint64(cycleLen))
			pos := (int(v.frame)/speed + offset) % cycleLen
			dist := pos - row

			if dist >= 0 && dist < dropLen {
				var ch rune
				var style lipgloss.Style
				switch {
				case dist == 0:
					ch = '┃'
					style = highStyle
				case dist == 1:
					ch = '│'
					style = midStyle
				default:
					ch = ':'
					style = lowStyle
				}
				b.WriteString(style.Render(string(ch)))
			} else {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

// scatterHash returns a pseudo-random value in [0, 1).
func scatterHash(band, row, col int, frame uint64) float64 {
	f := (frame + uint64(row*3+col)) / 3
	h := uint64(band)*7919 + uint64(row)*6271 + uint64(col)*3037 + f*104729
	h ^= h >> 16
	h *= 0x45d9f3b37197344b
	h ^= h >> 16
	return float64(h%10000) / 10000.0
}
