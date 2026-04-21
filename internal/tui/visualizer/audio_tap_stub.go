//go:build !darwin && !windows
// +build !darwin,!windows

package visualizer

func newDarwinAudioTap() *AudioTap {
	return nil
}

func newWASAPITap() *AudioTap {
	return nil
}

func WASAPIAvailable() bool {
	return false
}