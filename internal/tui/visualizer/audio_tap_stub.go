//go:build !windows && !darwin
// +build !windows,!darwin

package visualizer

// Stub for Windows compilation - these are defined in audio_tap.go for Linux
func newWASAPITap() *AudioTap {
	return nil
}

func WASAPIAvailable() bool {
	return false
}