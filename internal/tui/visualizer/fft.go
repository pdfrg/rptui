package visualizer

import (
	"log"
	"math"

	"gonum.org/v1/gonum/dsp/fourier"
)

const (
	fftSize    = 2048
	bandCount  = 10
	sampleRate = 48000.0
)

var fftLogger *log.Logger

func SetFFTLogger(l *log.Logger) {
	fftLogger = l
}

func isInf(f float64) bool {
	return f == math.Inf(1) || f == math.Inf(-1)
}

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

	// Check for NaN or invalid samples
	if fftLogger != nil {
		hasNaN := false
		for i := 0; i < min(10, len(samples)); i++ {
			v := float64(samples[i])
			if math.IsNaN(v) || isInf(v) {
				hasNaN = true
				break
			}
		}
		if hasNaN {
			fftLogger.Printf("FFT: Input samples contain NaN/Inf, first 10: %v", samples[:10])
		}
	}

	// Convert float32 to float64 and apply window
	for i := range fftSize {
		val := float64(samples[i])
		if math.IsNaN(val) || isInf(val) {
			a.samples[i] = 0
		} else {
			a.samples[i] = val * a.window[i]
		}
	}

	// FFT
	a.fft.Coefficients(a.complexBuf, a.samples)

	// Compute magnitudes
	for i := range a.magBuf {
		c := a.complexBuf[i]
		realPart := real(c)
		imagPart := imag(c)
		if math.IsNaN(realPart) || math.IsNaN(imagPart) || isInf(realPart) || isInf(imagPart) {
			a.magBuf[i] = 0
		} else {
			a.magBuf[i] = math.Sqrt(realPart*realPart + imagPart*imagPart)
		}
	}

	if fftLogger != nil {
		fftLogger.Printf("FFT: magBuf first 10: %v", a.magBuf[:10])
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
		// Normalize: apply log scaling to compress dynamic range
		// Typical music: avg 10-200 per bin. Log10(1+avg)/2.5 maps this to ~0.4-0.92
		avg := sum / float64(end-start)
		a.bands[b] = math.Log10(1+avg) / 2.5
		if a.bands[b] > 1 {
			a.bands[b] = 1
		}
	}

	// Temporal smoothing: fast attack, slow decay
	for i := range bandCount {
		if a.bands[i] > a.prevBands[i] {
			a.bands[i] = a.bands[i]*0.8 + a.prevBands[i]*0.2
		} else {
			a.bands[i] = a.bands[i]*0.25 + a.prevBands[i]*0.75
		}
		a.prevBands[i] = a.bands[i]
	}

	return a.bands
}
