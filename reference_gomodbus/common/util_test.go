package common

import (
	"net"
	"strconv"
	"testing"
)

func TestFindFreePortTCP(t *testing.T) {
	// Get a free port
	port, err := FindFreePortTCP()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	// Verify the port is valid
	if port <= 0 || port > 65535 {
		t.Errorf("Invalid port number returned: %d", port)
	}

	// Verify we can actually listen on this port
	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Errorf("Could not listen on port %d: %v", port, err)
	}
	defer listener.Close()

	// Verify multiple calls return different ports
	port2, err := FindFreePortTCP()
	if err != nil {
		t.Fatalf("Failed to find second free port: %v", err)
	}

	if port == port2 {
		t.Errorf("Got same port (%d) for two consecutive calls", port)
	}
}