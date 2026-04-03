// Package visualizer provides audio spectrum visualization with multiple rendering modes.
package visualizer

import (
	"math"
)

const (
	DefaultBandCount = 10
	DefaultRows      = 5
)

// Unicode block elements for bar height (9 levels including space)
var barBlocks = []string{" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// Mode constants — must match the order in visModes registry
const (
	ModeBars VisMode = iota
	ModeBarsDot
	ModeClassicPeak
	ModeWave
	ModeRetro
	ModeCount
)

// VisMode selects the visualizer rendering style.
type VisMode int

// visEntry pairs a display name with a mode.
type visEntry struct {
	name string
}

// visModes is the single source of truth for all visualizer modes.
var visModes = [ModeCount]visEntry{
	ModeBars:        {"Bars"},
	ModeBarsDot:     {"BarsDot"},
	ModeClassicPeak: {"ClassicPeak"},
	ModeWave:        {"Wave"},
	ModeRetro:       {"Retro"},
}

// ModeNames returns display names for all visualizer modes.
func ModeNames() []string {
	names := make([]string, ModeCount)
	for i := range visModes {
		names[i] = visModes[i].name
	}
	return names
}

// Visualizer performs spectrum analysis and renders visualizer output.
type Visualizer struct {
	bands          []float64
	prevBands      []float64
	mode           VisMode
	rows           int
	frame          uint64
	refreshPending bool
	seed           uint64
	peakState      *peakState // persistent peak positions for ClassicPeak mode
}

// New creates a Visualizer with the given seed for spectrum generation.
func New(seed uint64) *Visualizer {
	v := &Visualizer{
		bands:     make([]float64, DefaultBandCount),
		prevBands: make([]float64, DefaultBandCount),
		rows:      DefaultRows,
		seed:      seed,
	}
	v.initSpectrum()
	return v
}

// Mode returns the current visualizer mode.
func (v *Visualizer) Mode() VisMode { return v.mode }

// ModeName returns the display name of the current mode.
func (v *Visualizer) ModeName() string {
	if v.mode >= 0 && int(v.mode) < len(visModes) {
		return visModes[v.mode].name
	}
	return "Unknown"
}

// CycleMode advances to the next visualizer mode.
func (v *Visualizer) CycleMode() {
	v.mode = (v.mode + 1) % ModeCount
	v.refreshPending = true
}

// CycleModeReverse goes to the previous visualizer mode.
func (v *Visualizer) CycleModeReverse() {
	v.mode = (v.mode - 1 + ModeCount) % ModeCount
	v.refreshPending = true
}

// RequestRefresh marks that the visualizer needs re-rendering.
func (v *Visualizer) RequestRefresh() {
	v.refreshPending = true
}

// ConsumeRefresh reports and clears the refresh flag.
func (v *Visualizer) ConsumeRefresh() bool {
	if v == nil || !v.refreshPending {
		return false
	}
	v.refreshPending = false
	return true
}

// Bands returns the current spectrum band values (0-1).
func (v *Visualizer) Bands() []float64 { return v.bands }

// Frame returns the current animation frame counter.
func (v *Visualizer) Frame() uint64 { return v.frame }

// Tick advances the visualizer state. Call at ~20 FPS when playing, ~5 FPS when paused.
func (v *Visualizer) Tick(playing bool, paused bool) {
	if v == nil {
		return
	}

	if paused {
		// Decay bands when paused, don't advance animation frame
		for i := range v.bands {
			v.bands[i] *= 0.85
			v.prevBands[i] = v.bands[i]
		}
		v.refreshPending = true
		return
	}

	v.frame++
	if playing {
		v.updateSpectrum()
	}
}

// Render returns the rendered visualizer output as a string.
// Dispatches to the appropriate mode renderer.
func (v *Visualizer) Render(width int) string {
	if v == nil {
		return ""
	}
	switch v.mode {
	case ModeBars:
		return v.renderBars(width)
	case ModeBarsDot:
		return v.renderBarsDot(width)
	case ModeClassicPeak:
		return v.renderClassicPeak(width)
	case ModeWave:
		return v.renderWave(width)
	case ModeRetro:
		return v.renderRetro(width)
	default:
		return v.renderBars(width)
	}
}

// SetSeed reinitializes the spectrum with a new seed (e.g. on song change).
func (v *Visualizer) SetSeed(seed uint64) {
	v.seed = seed
	v.peakState = nil // reset peak positions for new song
	v.initSpectrum()
	v.refreshPending = true
}

// SetRows sets the display height in terminal rows.
func (v *Visualizer) SetRows(rows int) {
	if rows > 0 {
		v.rows = rows
	}
}

// SetMode sets the visualizer mode directly.
func (v *Visualizer) SetMode(mode VisMode) {
	if mode >= 0 && mode < ModeCount {
		v.mode = mode
		v.refreshPending = true
	}
}

// --- Simulated spectrum generation ---

// initSpectrum generates initial band values from the seed.
func (v *Visualizer) initSpectrum() {
	seed := v.seed
	for i := range v.bands {
		seed = seed*6364136223846793005 + 1442695040888963407
		val := float64(seed>>33) / float64(1<<31)
		v.bands[i] = val*0.6 + 0.1
		v.prevBands[i] = v.bands[i]
	}
}

// updateSpectrum evolves the spectrum bands with temporal smoothing.
// Incorporates v.frame so the spectrum changes every tick.
func (v *Visualizer) updateSpectrum() {
	for i := range v.bands {
		// Each band gets a unique seed per frame — no iterative LCG across bands
		seed := v.seed + uint64(i)*104729 + uint64(v.frame)*3571
		seed = seed*6364136223846793005 + 1442695040888963407
		raw := float64(seed>>33) / float64(1<<31)

		// Shape the raw value: bias toward musical patterns
		target := math.Sin(raw*math.Pi*2+float64(i)*0.7)*0.3 + 0.5
		target = math.Max(0, math.Min(1, target))

		// Temporal smoothing: fast attack, slow decay (same as cliamp)
		if target > v.bands[i] {
			v.bands[i] = target*0.6 + v.prevBands[i]*0.4
		} else {
			v.bands[i] = target*0.25 + v.prevBands[i]*0.75
		}
		v.prevBands[i] = v.bands[i]
	}
	v.refreshPending = true
}

// ModeFromString converts a mode name to VisMode.
func ModeFromString(name string) VisMode {
	for i, entry := range visModes {
		if entry.name == name {
			return VisMode(i)
		}
	}
	return ModeBars // default
}
