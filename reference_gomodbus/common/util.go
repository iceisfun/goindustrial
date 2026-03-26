package common

import (
	"fmt"
	"net"
)

// FindFreePortTCP finds an available TCP port by listening on port 0.
// It returns the free port and closes the listener immediately.
func FindFreePortTCP() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to bind to port 0: %w", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected address type: %T", listener.Addr())
	}
	return addr.Port, nil
}