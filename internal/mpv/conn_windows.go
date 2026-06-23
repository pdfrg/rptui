//go:build windows

package mpv

import (
	"net"

	winio "github.com/Microsoft/go-winio"
)

func dialMPV(socketPath string) (net.Conn, error) {
	return winio.DialPipe(socketPath, nil)
}
