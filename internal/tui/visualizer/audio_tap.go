package visualizer

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
)

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
	buf    *ringBuffer
	done   chan struct{}
	closed bool
}

// findDefaultSinkNode returns the node ID of the default Audio/Sink.
func findDefaultSinkNode() (int, error) {
	cmd := exec.Command("pw-cli", "list-objects")
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	// Parse output to find Audio/Sink nodes
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if strings.Contains(line, `media.class = "Audio/Sink"`) {
			// Look backwards for the node ID (can be 10-25 lines back)
			for j := i - 1; j >= 0 && j >= i-30; j-- {
				if strings.Contains(lines[j], "node.id") {
					parts := strings.Split(lines[j], "=")
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
	return 0, fmt.Errorf("no Audio/Sink found")
}

// NewAudioTap creates an AudioTap that captures mono float32 audio at 48kHz
// from the default PipeWire sink's monitor output.
// Returns nil if pw-record is not available or no sink is found.
func NewAudioTap() *AudioTap {
	if _, err := exec.LookPath("pw-record"); err != nil {
		return nil
	}

	// Find the default sink node to capture its monitor output
	sinkNode, err := findDefaultSinkNode()
	if err != nil {
		return nil
	}

	cmd := exec.Command("pw-record",
		"--format=f32",
		"--rate=48000",
		"--channels=1",
		"--channel-map=mono",
		"--latency=50ms",
		fmt.Sprintf("--target=%d", sinkNode),
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}

	if err := cmd.Start(); err != nil {
		return nil
	}

	tap := &AudioTap{
		cmd:    cmd,
		stdout: stdout,
		buf:    newRingBuffer(8192),
		done:   make(chan struct{}),
	}

	go tap.readLoop()
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
