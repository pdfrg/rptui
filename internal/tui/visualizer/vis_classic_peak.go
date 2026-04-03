package visualizer

import "strings"

// peakPositions tracks falling peak cap positions per band, keyed by band index.
// Stored on the Visualizer for persistence across frames.
type peakState struct {
	positions []float64
}

var peakCache map[uint64]*peakState

func (v *Visualizer) getPeakState() *peakState {
	if peakCache == nil {
		peakCache = make(map[uint64]*peakState)
	}
	if ps, ok := peakCache[v.seed]; ok {
		if len(ps.positions) != len(v.bands) {
			ps.positions = make([]float64, len(v.bands))
		}
		return ps
	}
	ps := &peakState{positions: make([]float64, len(v.bands))}
	peakCache[v.seed] = ps
	return ps
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

	// Update peak positions
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
