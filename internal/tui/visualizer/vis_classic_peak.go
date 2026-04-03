package visualizer

import "strings"

// peakPositions stores the falling peak cap height per band.
// Lives on the Visualizer so it resets when SetSeed is called.
type peakState struct {
	positions []float64
}

// getPeakState returns the peak state, creating it if needed.
func (v *Visualizer) getPeakState() *peakState {
	if v.peakState == nil {
		v.peakState = &peakState{positions: make([]float64, len(v.bands))}
	}
	if len(v.peakState.positions) != len(v.bands) {
		v.peakState.positions = make([]float64, len(v.bands))
	}
	return v.peakState
}

// renderClassicPeak renders classic falling peak meter bars.
func (v *Visualizer) renderClassicPeak(width int) string {
	height := v.rows
	bandCount := len(v.bands)
	bandWidth := max(1, (width-(bandCount-1))/bandCount)
	if bandWidth < 1 {
		bandWidth = 1
	}

	ps := v.getPeakState()

	// Update peak positions: rise with level, fall slowly
	for i, level := range v.bands {
		peakTarget := level * float64(height)
		if peakTarget > ps.positions[i] {
			ps.positions[i] = peakTarget
		} else {
			ps.positions[i] -= 0.15
			if ps.positions[i] < 0 {
				ps.positions[i] = 0
			}
		}
	}

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowLevel := float64(height - 1 - row)

		for i, level := range v.bands {
			barH := level * float64(height)
			peakH := ps.positions[i]
			ch := " "
			if rowLevel < barH {
				ch = "│"
			}
			if rowLevel >= peakH-0.5 && rowLevel <= peakH+0.5 {
				ch = "━"
			}
			for range bandWidth {
				b.WriteString(ch)
			}
			if i < bandCount-1 {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
