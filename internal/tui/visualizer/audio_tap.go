package visualizer

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
)

var audioLogger *log.Logger

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

// AudioTap captures audio from the PipeWire monitor sink via pw-record.
type AudioTap struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr io.ReadCloser
	buf    *ringBuffer
	done   chan struct{}
	closed bool
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

// NewAudioTap creates an AudioTap that captures mono float32 audio at 48kHz
// from the default PipeWire sink's monitor output.
// Returns nil if pw-record is not available or no sink is found.
func NewAudioTap() *AudioTap {
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

func (t *AudioTap) readLoop() {
	defer close(t.done)

	byteBuf := make([]byte, 480*4)
	floatBuf := make([]float32, 480)

	for {
		n, err := io.ReadFull(t.stdout, byteBuf)
		if err != nil {
			return
		}
		sampleCount := n / 4
		for i := 0; i < sampleCount; i++ {
			bits := binary.LittleEndian.Uint32(byteBuf[i*4 : (i+1)*4])
			floatBuf[i] = math.Float32frombits(bits)
		}
		t.buf.Write(floatBuf[:sampleCount])
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
