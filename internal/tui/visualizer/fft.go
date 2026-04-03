package visualizer

import (
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	fftSize    = 2048
	bandCount  = 10
	sampleRate = 48000.0
)

// Analyzer performs FFT analysis on audio samples and extracts frequency bands.
type Analyzer struct {
	fft        *fourier.FFT
	window     []float64
	samples    []float64
	complexBuf []complex128
	magBuf     []float64

	// Band extraction
	bandEdges []int // index boundaries for each frequency band

	// Temporal smoothing state
	bands     []float64
	prevBands []float64
}

// NewAnalyzer creates an FFT analyzer ready for audio processing.
func NewAnalyzer() *Analyzer {
	a := &Analyzer{
		fft:        fourier.NewFFT(fftSize),
		window:     make([]float64, fftSize),
		samples:    make([]float64, fftSize),
		complexBuf: make([]complex128, fftSize/2+1),
		magBuf:     make([]float64, fftSize/2+1),
		bands:      make([]float64, bandCount),
		prevBands:  make([]float64, bandCount),
	}

	// Hann window
	for i := range a.window {
		a.window[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(fftSize-1)))
	}

	// Logarithmic band edges: 20Hz to 20kHz split into 10 bands
	a.bandEdges = computeBandEdges()

	return a
}

// computeBandEdges creates logarithmically spaced FFT bin boundaries.
func computeBandEdges() []int {
	edges := make([]int, bandCount+1)
	logMin := math.Log10(20.0)
	logMax := math.Log10(20000.0)

	for i := range bandCount {
		logFreq := logMin + float64(i)/float64(bandCount)*(logMax-logMin)
		freq := math.Pow(10, logFreq)
		bin := int(freq * fftSize / sampleRate)
		if bin < 1 {
			bin = 1
		}
		if bin > fftSize/2 {
			bin = fftSize / 2
		}
		edges[i] = bin
	}
	edges[bandCount] = fftSize / 2

	// Ensure monotonically increasing
	for i := 1; i <= bandCount; i++ {
		if edges[i] <= edges[i-1] {
			edges[i] = edges[i-1] + 1
		}
	}

	return edges
}

// Analyze processes audio samples and returns smoothed frequency bands (0-1).
// Returns nil if not enough samples are available.
func (a *Analyzer) Analyze(samples []float32) []float64 {
	if len(samples) < fftSize {
		return nil
	}

	// Convert float32 to float64 and apply window
	for i := range fftSize {
		a.samples[i] = float64(samples[i]) * a.window[i]
	}

	// FFT
	a.fft.Coefficients(a.complexBuf, a.samples)

	// Compute magnitudes
	for i := range a.magBuf {
		c := a.complexBuf[i]
		a.magBuf[i] = math.Sqrt(real(c)*real(c) + imag(c)*imag(c))
	}

	// Extract bands
	for b := range bandCount {
		start := a.bandEdges[b]
		end := a.bandEdges[b+1]
		if start >= end {
			a.bands[b] = 0
			continue
		}

		// Sum magnitudes in band
		sum := 0.0
		for i := start; i < end; i++ {
			sum += a.magBuf[i]
		}
		// Normalize: divide by bin count and apply log scaling
		avg := sum / float64(end-start)
		a.bands[b] = math.Log10(1+avg*100) / 3.0 // log scale, clamp to ~0-1
		if a.bands[b] > 1 {
			a.bands[b] = 1
		}
	}

	// Temporal smoothing: fast attack, slow decay
	for i := range bandCount {
		if a.bands[i] > a.prevBands[i] {
			a.bands[i] = a.bands[i]*0.6 + a.prevBands[i]*0.4
		} else {
			a.bands[i] = a.bands[i]*0.25 + a.prevBands[i]*0.75
		}
		a.prevBands[i] = a.bands[i]
	}

	return a.bands
}
