//go:build darwin

package visualizer

import (
	"encoding/binary"
	"io"
	"math"
	"os/exec"
)

const blackHoleDevice = "BlackHole 2ch"

func SoXAvailable() bool {
	_, err := exec.LookPath("sox")
	return err == nil
}

func BlackHoleAvailable() bool {
	if !SoXAvailable() {
		return false
	}
	cmd := exec.Command("sox", "-t", "coreaudio", blackHoleDevice, "-n", "stat")
	err := cmd.Run()
	return err == nil
}

func newDarwinAudioTap() *AudioTap {
	if !SoXAvailable() {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: SoX not found")
		}
		return nil
	}

	if !BlackHoleAvailable() {
		if audioLogger != nil {
			audioLogger.Printf("AudioTap: BlackHole device '%s' not found", blackHoleDevice)
		}
		return nil
	}

	cmd := exec.Command("sox",
		"-t", "coreaudio", blackHoleDevice,
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