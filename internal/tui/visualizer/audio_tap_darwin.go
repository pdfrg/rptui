//go:build darwin

package visualizer

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
)

func newWASAPITap() *AudioTap {
	return nil
}

func SoXAvailable() bool {
	_, err := exec.LookPath("sox")
	return err == nil
}

func BlackHoleAvailable() (bool, string) {
	if !SoXAvailable() {
		return false, ""
	}

	// Try multiple possible BlackHole device names in priority order
	candidates := []string{
		"BlackHole 2ch",  // Most common
		"BlackHole 16ch", // Multi-channel
		"BlackHole 64ch", // High channel count
		"BlackHole",      // Base name fallback
	}

	for _, device := range candidates {
		cmd := exec.Command("sox", "-t", "coreaudio", device, "-n", "stat")
		if err := cmd.Run(); err == nil {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap: Found BlackHole device: %s", device)
			}
			return true, device
		}
	}

	return false, ""
}

func newDarwinAudioTap() *AudioTap {
	if !SoXAvailable() {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: SoX not found")
		}
		return nil
	}

	available, device := BlackHoleAvailable()
	if !available {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: No BlackHole device found")
		}
		return nil
	}

	cmd := exec.Command("sox",
		"-t", "coreaudio", device,
		"-t", "raw", "-",
		"rate", "48000",
		"channels", "1",
		"encoding", "float",
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

	tap := &AudioTap{
		cmd:        cmd,
		stdout:     stdout,
		stderr:     stderr,
		buf:        newRingBuffer(8192),
		done:       make(chan struct{}),
		sampleSize: 4, // float32 = 4 bytes
	}

	go tap.readLoopDarwin()
	go io.Copy(io.Discard, tap.stderr)

	return tap
}

func (t *AudioTap) readLoopDarwin() {
	defer close(t.done)

	if audioLogger != nil {
		audioLogger.Printf("AudioTap readLoopDarwin: starting")
	}

	accumSamples := 2048
	sampleSize := 4
	floatBuf := make([]float32, accumSamples)
	byteBuf := make([]byte, accumSamples*sampleSize)

	for {
		n, err := io.ReadFull(t.stdout, byteBuf)
		if err != nil {
			if audioLogger != nil {
				audioLogger.Printf("AudioTap readLoopDarwin: read error: %v", err)
				state, sterr := t.cmd.Process.Wait()
				if sterr == nil {
					audioLogger.Printf("AudioTap readLoopDarwin: process exited: %v", state)
				}
			}
			return
		}

		sampleCount := n / sampleSize
		if sampleCount == 0 {
			continue
		}

		for i := 0; i < sampleCount; i++ {
			bits := binary.LittleEndian.Uint32(byteBuf[i*4 : (i+1)*4])
			floatBuf[i] = math.Float32frombits(bits)
		}

		t.buf.Write(floatBuf[:sampleCount])
		if audioLogger != nil {
			audioLogger.Printf("AudioTap readLoopDarwin: wrote %d samples", sampleCount)
		}
	}
}
