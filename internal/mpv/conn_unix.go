//go:build !windows

package mpv

import "net"

func dialMPV(socketPath string) (net.Conn, error) {
	return net.Dial("unix", socketPath)
}
