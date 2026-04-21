package visualizer

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

var audioLogger *log.Logger
var audioServer      string
var activeBackend   string

func SetAudioLogger(l *log.Logger) {
	audioLogger = l
}

// ringBuffer is a lock-free single-writer/single-reader ring buffer for float32 samples.
type ringBuffer struct {
	data []float32
	head uint64 // write position (atomic)
	tail uint64 // read position (atomic)
	mask uint64 // size-1, must be power of 2
}

func newRingBuffer(size uint64) *ringBuffer {
	power := uint64(1)
	for power < size {
		power <<= 1
	}
	return &ringBuffer{
		data: make([]float32, power),
		mask: power - 1,
	}
}

func (r *ringBuffer) Write(samples []float32) {
	for _, s := range samples {
		head := atomic.LoadUint64(&r.head)
		r.data[head&r.mask] = s
		atomic.StoreUint64(&r.head, head+1)
	}
}

func (r *ringBuffer) Read(dst []float32) int {
	tail := atomic.LoadUint64(&r.tail)
	head := atomic.LoadUint64(&r.head)
	available := head - tail

	n := uint64(len(dst))
	if n > available {
		n = available
	}
	if n == 0 {
		return 0
	}

	for i := uint64(0); i < n; i++ {
		dst[i] = r.data[(tail+i)&r.mask]
	}
	atomic.StoreUint64(&r.tail, tail+n)
	return int(n)
}

func (r *ringBuffer) Available() uint64 {
	head := atomic.LoadUint64(&r.head)
	tail := atomic.LoadUint64(&r.tail)
	return head - tail
}

func DetectAudioServer() string {
	if audioServer != "" {
		return audioServer
	}

	cmd := exec.Command("pactl", "info")
	out, err := cmd.Output()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: failed to detect audio server: %v", err)
		}
		audioServer = "Unknown"
		return audioServer
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Server Name:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "Server Name:"))
			audioServer = name
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: detected audio server: %s", name)
			}
			return name
		}
	}

	audioServer = "Unknown"
	return audioServer
}

func IsPulseAudio() bool {
	server := DetectAudioServer()
	return strings.HasPrefix(server, "pulseaudio")
}

func DetectPulseAudioFormat() (string, int, error) {
	cmd := exec.Command("pactl", "info")
	out, err := cmd.Output()
	if err != nil {
		return "", 0, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Default Sample Specification:") {
			spec := strings.TrimSpace(strings.TrimPrefix(line, "Default Sample Specification:"))
			parts := strings.Fields(spec)
			if len(parts) >= 2 {
				format := parts[0] // "s16le", "float32le", etc.
				rate := parts[1]   // "44100Hz", "48000Hz", etc.

				// Parse sample rate
				rate = strings.TrimSuffix(rate, "Hz")
				rateNum, err := strconv.Atoi(rate)
				if err != nil {
					rateNum = 44100
				}

				return format, rateNum, nil
			}
		}
	}
	return "", 0, fmt.Errorf("Default Sample Specification not found")
}

// AudioTap captures audio from the PipeWire monitor sink via pw-record.
type AudioTap struct {
	cmd        *exec.Cmd
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	buf        *ringBuffer
	done       chan struct{}
	closed     bool
	sampleSize int  // bytes per sample: 4 for float32, 2 for s16le
	useStderr  bool // for parecord -v: audio goes to stderr instead of stdout
}

// findMonitorSourceNode returns the node ID of the default sink's monitor source.
// Uses pactl which lists monitor sources even when suspended (unlike pw-cli).
func findMonitorSourceNode() (int, error) {
	// Try pactl first — it always lists sources even when suspended
	cmd := exec.Command("pactl", "list", "sources", "short")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, ".monitor") {
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					num, err := strconv.Atoi(fields[0])
					if err == nil {
						return num, nil
					}
				}
			}
		}
	}

	// Fallback: try pw-cli list-objects for active monitor nodes
	cmd = exec.Command("pw-cli", "list-objects")
	out, err = cmd.Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if strings.Contains(line, `media.class = "Stream/Input/Audio"`) {
			for j := i - 1; j >= 0 && j >= i-30; j-- {
				if strings.Contains(lines[j], `node.name`) && strings.Contains(lines[j], ".monitor") {
					for k := j; k >= 0 && k >= j-10; k-- {
						if strings.Contains(lines[k], "node.id") {
							parts := strings.Split(lines[k], "=")
							if len(parts) >= 2 {
								numStr := strings.TrimSpace(parts[1])
								numStr = strings.Trim(numStr, `"`)
								num, err := strconv.Atoi(numStr)
								if err == nil {
									return num, nil
								}
							}
						}
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("no monitor source found")
}

// newPipeWireTap creates an AudioTap using pw-record (native PipeWire).
// Returns nil if pw-record is not available or no sink is found.
func newPipeWireTap() *AudioTap {
	if _, err := exec.LookPath("pw-record"); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: pw-record not found: %v", err)
		}
		return nil
	}

	// Find the default sink's monitor source node to capture system audio output
	monitorNode, err := findMonitorSourceNode()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: findMonitorSourceNode failed: %v", err)
		}
		return nil
	}
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: found monitor node %d", monitorNode)
	}

	cmd := exec.Command("pw-record",
		"--format=f32",
		"--rate=48000",
		"--channels=1",
		"--channel-map=mono",
		"--latency=50ms",
		fmt.Sprintf("--target=%d", monitorNode),
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: StdoutPipe failed: %v", err)
		}
		return nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: StderrPipe failed: %v", err)
		}
		return nil
	}

	if err := cmd.Start(); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: cmd.Start failed: %v", err)
		}
		return nil
	}
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: pw-record started successfully (PID: %d)", cmd.Process.Pid)
	}

	tap := &AudioTap{
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		buf:    newRingBuffer(8192),
		done:   make(chan struct{}),
	}

	go tap.readLoop()
	// Drain stderr so pw-record doesn't block
	go io.Copy(io.Discard, tap.stderr)

	return tap
}

// newPulseAudioTap creates an AudioTap using parecord (PulseAudio).
// Returns nil if parecord is not available.
func newPulseAudioTap() *AudioTap {
	if _, err := exec.LookPath("parecord"); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord not found: %v", err)
		}
		return nil
	}

	// Detect format for determining bytes per sample (used by readLoop)
	// and for command line flags
	formatName, sampleRate, _ := DetectPulseAudioFormat()
	sampleSize := 4 // default to float32
	var formatFlag string
	if formatName == "s16le" || formatName == "s16be" || formatName == "s16" {
		sampleSize = 2
		formatFlag = "s16le"
	} else {
		formatFlag = "float32le"
	}
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: using PulseAudio format: %s, rate: %d, sampleSize: %d",
			formatName, sampleRate, sampleSize)
	}

	// Use stdbuf to disable stdout buffering - without it, parecord buffers
	// output and Go's pipe sees EOF immediately
	cmd := exec.Command("stdbuf", "-o0", "parecord",
		"--raw",
		"--device=@DEFAULT_MONITOR@",
		"--format="+formatFlag,
		"--rate="+fmt.Sprint(sampleRate),
		"--channels=1",
		"--channel-map=mono",
	) // no "-" needed - stdout is default with stdbuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord StdoutPipe failed: %v", err)
		}
		return nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord StderrPipe failed: %v", err)
		}
		return nil
	}

	if err := cmd.Start(); err != nil {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord cmd.Start failed: %v", err)
		}
		return nil
	}
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: parecord started successfully (PID: %d)", cmd.Process.Pid)
	}

	// Log when process exits
	go func() {
		err := cmd.Wait()
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord process exited: %v", err)
		}
	}()

	tap := &AudioTap{
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
		buf:        newRingBuffer(8192),
		done:       make(chan struct{}),
		sampleSize:  sampleSize,
		useStderr:  false, // parecord sends audio to stdout
	}

	go tap.readLoop()
	// Drain stderr so parecord doesn't block
	go io.Copy(io.Discard, tap.stderr)

	return tap
}

// NewAudioTap creates an AudioTap that captures mono float32 audio at 48kHz
// from the default audio sink's monitor output.
// Auto-detects platform (Windows/PipeWire/PulseAudio/macOS/SoX) and uses appropriate backend.
// Returns nil if no audio backend is available.
func NewAudioTap() *AudioTap {
	// Windows: use WASAPI loopback capture
	if runtime.GOOS == "windows" {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: Windows detected, using WASAPI loopback")
		}
		tap := newWASAPITap()
		if tap != nil {
			activeBackend = "WASAPI"
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: using WASAPI backend")
			}
			return tap
		}
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: WASAPI not available, using simulated mode")
		}
		return nil
	}

	// macOS: use SoX with BlackHole for system audio capture
	if runtime.GOOS == "darwin" {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: macOS detected, using SoX + BlackHole")
		}
		tap := newDarwinAudioTap()
		if tap != nil {
			activeBackend = "SoX"
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: using SoX backend")
			}
			return tap
		}
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: SoX/BlackHole not available, using simulated mode")
		}
		return nil
	}

	// Linux: detect PipeWire vs PulseAudio
	server := DetectAudioServer()
	isPulse := IsPulseAudio()

	// Primary backend: use the detected audio server's native tool
	if isPulse {
		// PulseAudio detected - use parecord first
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: primary backend: PulseAudio (server: %s)", server)
		}
		tap := newPulseAudioTap()
		if tap != nil {
			activeBackend = "PulseAudio"
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: using PulseAudio backend")
			}
			return tap
		}
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: parecord failed, trying PipeWire fallback")
		}

		// Fallback: try pw-record if PulseAudio didn't work
		if PwRecordAvailable() {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: trying PipeWire fallback")
			}
			tap = newPipeWireTap()
			if tap != nil {
				activeBackend = "PipeWire"
				if audioLogger != nil {
					audioLogger.Printf("AudioTap: using PipeWire fallback backend")
				}
				return tap
			}
		}
	} else {
		// PipeWire detected (or unknown) - use pw-record first
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: primary backend: PipeWire (server: %s)", server)
		}
		tap := newPipeWireTap()
		if tap != nil {
			activeBackend = "PipeWire"
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: using PipeWire backend")
			}
			return tap
		}
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: pw-record failed, trying PulseAudio fallback")
		}

		// Fallback: try parecord if PipeWire didn't work
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: trying PulseAudio fallback")
		}
		tap = newPulseAudioTap()
		if tap != nil {
			activeBackend = "PulseAudio"
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: using PulseAudio fallback backend")
			}
			return tap
		}
	}

	activeBackend = ""
	if audioLogger != nil {
		audioLogger.Printf("AudioTap: no audio backend available, using simulated mode")
	}
	return nil
}

// ActiveBackend returns the currently active audio backend: "PipeWire", "PulseAudio", or "".
func ActiveBackend() string {
	return activeBackend
}

func (t *AudioTap) readLoop() {
	defer close(t.done)

	if audioLogger != nil {
		audioLogger.Printf("AudioTap readLoop: starting, sampleSize=%d, useStderr=%v",
			t.sampleSize, t.useStderr)
	}

	accumSamples := 2048  // accumulate this many before writing to ring buffer
	// sampleSize is 0 for pw-record (float32), 2 for parecord (s16le)
	sampleSizeBuf := t.sampleSize
	if sampleSizeBuf == 0 {
		sampleSizeBuf = 4 // default to float32 for pw-record
	}
	byteBuf := make([]byte, accumSamples*sampleSizeBuf)
	floatBuf := make([]float32, accumSamples)

	// Select reader: stderr for parecord -v, stdout otherwise
	reader := t.stdout
	if t.useStderr {
		reader = t.stderr
	}

	sampleSize := t.sampleSize
	if sampleSize == 0 {
		sampleSize = 4 // default to float32
	}

	for {
		// Read audio into accumulation buffer
		for collected := 0; collected < accumSamples; {
			var n int
			var err error

			// pw-record (stdout): use io.ReadFull for exact buffer
			// parecord: accumulate via Read() for variable chunks
			if t.useStderr {
				n, err = reader.Read(byteBuf[collected*sampleSize:])
			} else {
				// Read from stdout (parecord, pw-record)
				localBuf := byteBuf[collected*sampleSize:]
				n, err = io.ReadFull(reader, localBuf)
			}

			if err != nil {
				if audioLogger != nil {
					audioLogger.Printf("AudioTap readLoop: read error: %v", err)
					// Log process state
					state, sterr := t.cmd.Process.Wait()
					if sterr == nil {
						audioLogger.Printf("AudioTap readLoop: process exited: %v", state)
					}
				}
				return
			}
			if audioLogger != nil && collected == 0 {
				audioLogger.Printf("AudioTap readLoop: first read n=%d, sampleSize=%d", n, sampleSize)
			}

			sampleCount := n / sampleSize
			if sampleCount == 0 {
				continue
			}
			collected += sampleCount
		}

		// Convert accumulated samples to float32
		for i := 0; i < accumSamples; i++ {
			if sampleSize == 2 {
				// s16le to float32: divide by 32768.0
				bits := int16(binary.LittleEndian.Uint16(byteBuf[i*2 : (i+1)*2]))
				floatBuf[i] = float32(bits) / 32768.0
			} else {
				// float32 (4 bytes)
				bits := binary.LittleEndian.Uint32(byteBuf[i*4 : (i+1)*4])
				floatBuf[i] = math.Float32frombits(bits)
			}
		}

		t.buf.Write(floatBuf[:accumSamples])
		if audioLogger != nil {
			audioLogger.Printf("AudioTap readLoop: wrote %d samples to buffer, available=%d",
				accumSamples, t.buf.Available())
		}
	}
}

func (t *AudioTap) Close() {
	if t == nil || t.closed {
		return
	}
	t.closed = true
	t.cmd.Process.Kill()
	t.cmd.Wait()
	<-t.done
}

func (t *AudioTap) ReadSamples(dst []float32) int {
	if t == nil {
		return 0
	}
	return t.buf.Read(dst)
}

func (t *AudioTap) AvailableSamples() uint64 {
	if t == nil {
		return 0
	}
	return t.buf.Available()
}

func PwRecordAvailable() bool {
	_, err := exec.LookPath("pw-record")
	return err == nil
}

func ParecordAvailable() bool {
	_, err := exec.LookPath("parecord")
	return err == nil
}

func newDarwinAudioTap() *AudioTap {
	return nil
}






