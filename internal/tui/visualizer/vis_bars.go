package visualizer

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderBars renders smooth spectrum bars with fractional Unicode blocks
// and a vertical color gradient from accent (bottom) to cursor (top).
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

	// Pre-build styles for each row level (3-tier gradient like cliamp)
	lowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow))
	midStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5)))
	highStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh))

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)

		// Select row color based on vertical position (3-tier gradient)
		var blockStyle lipgloss.Style
		switch {
		case rowBottom >= 0.6:
			blockStyle = highStyle
		case rowBottom >= 0.3:
			blockStyle = midStyle
		default:
			blockStyle = lowStyle
		}

		for range leftPad {
			b.WriteString(" ")
		}
		for i, level := range v.bands {
			block := fracBlock(level, rowBottom, rowTop)
			if block == " " {
				for range bandWidth {
					b.WriteString(" ")
				}
			} else {
				s := ""
				for range bandWidth {
					s += block
				}
				b.WriteString(blockStyle.Render(s))
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

// interpolateColor blends between two hex colors and returns a hex string.
func (v *Visualizer) interpolateColor(c1, c2 string, t float64) string {
	r1, g1, b1 := parseHex(c1)
	r2, g2, b2 := parseHex(c2)
	r := int(float64(r1)*(1-t) + float64(r2)*t)
	g := int(float64(g1)*(1-t) + float64(g2)*t)
	b := int(float64(b1)*(1-t) + float64(b2)*t)
	return toHex(r, g, b)
}

func parseHex(hex string) (int, int, int) {
	if len(hex) == 7 && hex[0] == '#' {
		var r, g, b int
		_, _ = fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
		return r, g, b
	}
	return 128, 128, 128
}

func toHex(r, g, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}
