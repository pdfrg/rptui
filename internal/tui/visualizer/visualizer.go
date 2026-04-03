// Package visualizer provides audio spectrum visualization with multiple rendering modes.
package visualizer

import (
	"math"
	"strings"
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

// Visualizer performs spectrum analysis and renders visualizer output.
type Visualizer struct {
	bands          []float64
	prevBands      []float64
	mode           VisMode
	rows           int
	frame          uint64
	refreshPending bool
	seed           uint64
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
	v.frame++

	if paused {
		// Decay bands when paused
		for i := range v.bands {
			v.bands[i] *= 0.85
			v.prevBands[i] = v.bands[i]
		}
		v.refreshPending = true
		return
	}

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
	// Use the seed to create a unique energy profile per song
	seed := v.seed
	for i := range v.bands {
		// Simple LCG-based PRNG for deterministic per-song patterns
		seed = seed*6364136223846793005 + 1442695040888963407
		val := float64(seed>>33) / float64(1<<31) // normalize to 0-1
		v.bands[i] = val*0.6 + 0.1                // bias toward mid range
		v.prevBands[i] = v.bands[i]
	}
}

// updateSpectrum evolves the spectrum bands with temporal smoothing.
func (v *Visualizer) updateSpectrum() {
	seed := v.seed
	for i := range v.bands {
		// Evolve each band using a different phase of the PRNG
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

// --- Render stubs (Phase 3: full implementations) ---

func (v *Visualizer) renderBars(width int) string {
	return v.renderBarsGeneric(width, false)
}

func (v *Visualizer) renderBarsDot(width int) string {
	return v.renderBarsGeneric(width, true)
}

func (v *Visualizer) renderBarsGeneric(width int, dot bool) string {
	height := v.rows
	bandCount := len(v.bands)
	barWidth := max(1, width/bandCount)
	gap := 1
	totalW := bandCount*barWidth + (bandCount-1)*gap
	if totalW > width {
		barWidth = max(1, (width-(bandCount-1))/bandCount)
	}

	lines := make([]string, height)
	for row := range height {
		var b strings.Builder
		rowBottom := float64(height-1-row) / float64(height)
		rowTop := float64(height-row) / float64(height)
		for i, level := range v.bands {
			block := blockChar(level, rowBottom, rowTop, dot)
			for range barWidth {
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

func blockChar(level, rowBottom, rowTop float64, dot bool) string {
	if level <= rowBottom {
		return " "
	}
	if level >= rowTop {
		if dot {
			return "⣿"
		}
		return "█"
	}
	frac := (level - rowBottom) / (rowTop - rowBottom)
	idx := int(frac*8) + 1
	if idx > 8 {
		idx = 8
	}
	if dot {
		dots := []string{"⢀", "⡀", "⠄", "⠂", "⠁", "⠈", "⠐", "⠠"}
		return dots[max(0, min(7, idx-1))]
	}
	return barBlocks[max(0, min(8, idx))]
}

func (v *Visualizer) renderClassicPeak(width int) string {
	// Simple peak-meter style: bars with falling peak caps
	height := v.rows
	bandCount := len(v.bands)
	barWidth := max(1, width/bandCount)
	lines := make([]string, height)

	// Track peak positions (decay over frames)
	peaks := make([]float64, bandCount)
	for i, level := range v.bands {
		peakPos := level * float64(height)
		decay := float64(v.frame%20) / 20.0 * 0.3
		peaks[i] = math.Max(peakPos-float64(v.frame%20)*0.15, 0)
		_ = decay
	}

	for row := range height {
		var b strings.Builder
		rowLevel := float64(height - 1 - row)
		for i, level := range v.bands {
			barH := level * float64(height)
			peakH := peaks[i]
			ch := " "
			if rowLevel < barH {
				ch = "│"
			}
			if math.Abs(rowLevel-peakH) < 0.5 {
				ch = "━"
			}
			for range barWidth {
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

func (v *Visualizer) renderWave(width int) string {
	// Waveform oscilloscope style using Braille patterns
	height := v.rows
	if height < 3 {
		height = 3
	}
	midRow := height / 2
	lines := make([]string, height)

	// Generate wave pattern from bands
	brailleCols := width * 2
	for row := range height {
		var b strings.Builder
		for col := range brailleCols {
			wave := math.Sin(float64(col)*0.3+float64(v.frame)*0.2) * 0.5
			for _, band := range v.bands {
				wave += (band - 0.5) * math.Sin(float64(col)*0.15+float64(v.frame)*0.15) * 0.3
			}
			rowPos := float64(midRow) + wave*float64(midRow)
			if math.Abs(float64(row)-rowPos) < 0.5 {
				b.WriteString("⠁")
			} else {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}

func (v *Visualizer) renderRetro(width int) string {
	// 80s synthwave perspective grid
	height := v.rows
	lines := make([]string, height)

	for row := range height {
		var b strings.Builder
		perspective := float64(row+1) / float64(height)
		spacing := int(2.0 / perspective)
		if spacing < 1 {
			spacing = 1
		}
		for col := range width {
			if col%spacing == 0 {
				b.WriteString("│")
			} else {
				b.WriteString(" ")
			}
		}
		lines[row] = b.String()
	}
	return strings.Join(lines, "\n")
}
