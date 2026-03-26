package testutil

import (
	"fmt"
	"net"
)

// FindFreePort returns a free TCP port on localhost by binding to :0
// and immediately releasing the listener.
func FindFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// ListenFree creates a TCP listener on a random free port. The caller
// is responsible for closing the listener. This avoids the TOCTOU race
// inherent in FindFreePort.
func ListenFree() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}
