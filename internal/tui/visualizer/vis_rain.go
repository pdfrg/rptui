package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderRain draws bar-shaped columns filled with animated falling rain streaks.
// Bar height follows band level; interior has animated drops with head/body/tail.
func (v *Visualizer) renderRain(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)

	contentWidth := bandCount*bandWidth + (bandCount - 1)
	leftPad := (width - contentWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}

	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		level := 0.0
		rowNorm := float64(height-1-row) / float64(height)

		for range leftPad {
			b.WriteString(" ")
		}
		for i := range bandCount {
			level = v.bands[i]
			for range bandWidth {
				if rowNorm >= level {
					b.WriteString(" ")
					continue
				}

				seed := uint64(i)*7919 + 104729
				if scatterHash(i, 0, i, v.frame/12) > level*1.6+0.1 {
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
			if i < bandCount-1 {
				b.WriteString(" ")
			}
		}
		_ = level
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
