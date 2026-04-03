package visualizer

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderStars renders a starfield where dots flicker on/off across the entire view.
// Star positions are fixed; each star has its own flicker cycle so only some
// change per frame. Overall density varies with audio energy.
func (v *Visualizer) renderStars(width int) string {
	height := v.rows

	// Overall energy determines star density (0-1 range)
	energy := 0.0
	for _, b := range v.bands {
		energy += b
	}
	energy /= float64(len(v.bands))

	// Pre-build styles for variety
	styles := []lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorHigh)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(v.colorLow)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(v.interpolateColor(v.colorLow, v.colorHigh, 0.5))),
	}

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		for col := range width {
			// Fixed star locations: ~40% of cells can ever have a star
			locSeed := uint64(row)*73856093 ^ uint64(col)*19349663
			locSeed = locSeed*6364136223846793005 + 1442695040888963407
			locHash := locSeed >> 56

			if locHash > 102 { // ~40% of cells are potential star positions
				b.WriteString(" ")
				continue
			}

			// Flicker: each star has its own phase, changes ~every 400ms
			timeSeed := locHash ^ uint64(v.frame/8)*83492791
			timeSeed = timeSeed*6364136223846793005 + 1442695040888963407
			timeHash := timeSeed >> 56

			// Visibility threshold scales with energy
			// Low energy: 1-2% of cells visible
			// High energy: ~25% of cells visible
			visibleThreshold := uint64(3 + energy*61)
			if timeHash >= visibleThreshold {
				b.WriteString(" ")
				continue
			}

			// Render the star
			styleIdx := int(locHash>>4) % 3
			b.WriteString(styles[styleIdx].Render("·"))
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
