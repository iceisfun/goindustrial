package modbus

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// countingConn wraps a net.Conn and records how many times Close is called, so
// a test can assert the underlying socket is actually released.
type countingConn struct {
	net.Conn
	closes atomic.Int32
}

func (c *countingConn) Close() error {
	c.closes.Add(1)
	return c.Conn.Close()
}

// TestDisconnectClosesSocketAfterPeerClose is a regression test for a file
// descriptor leak. When the peer closes the connection (a clean FIN surfaces
// as io.EOF in readLoop), setDisconnected clears c.connected. A Disconnect
// guarded by `if !c.connected { return nil }` then early-returns and never
// calls conn.Close(), so the socket is leaked (it lingers in CLOSE_WAIT and is
// orphaned once the reconnecting transport dials a replacement). This bit an
// ABB Modbus PLC that closes its connections routinely, accumulating thousands
// of dead sockets over a few days.
//
// The socket must be closed exactly once regardless of who observed the drop
// first.
func TestDisconnectClosesSocketAfterPeerClose(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Simulate a peer-initiated close: the far end sends FIN. readLoop's
	// io.ReadFull then returns io.EOF and calls setDisconnected, which clears
	// c.connected before the transport gets a chance to tear the socket down.
	_ = serverConn.Close()

	// Wait for readLoop to observe the peer close.
	deadline := time.Now().Add(2 * time.Second)
	for c.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if c.IsConnected() {
		t.Fatal("readLoop did not observe the peer close (connected still true)")
	}

	// The transport's Closer tears the connection down. Before the fix this
	// early-returned on !connected and left the socket open.
	if err := (&TCPCloser{}).Close(c); err != nil {
		// Close may surface the already-closed pipe error; not a failure.
		t.Logf("close returned: %v", err)
	}

	if got := wrapped.closes.Load(); got != 1 {
		t.Fatalf("underlying socket Close called %d time(s), want 1 "+
			"(fd leak: Disconnect skipped conn.Close after peer close)", got)
	}
}

// TestDisconnectIdempotent guards the other half of the fix: gating teardown on
// closeOnce (instead of the connected flag) must not let close(c.done) panic on
// a second Disconnect.
func TestDisconnectIdempotent(t *testing.T) {
	_, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	closer := &TCPCloser{}
	_ = closer.Close(c)
	_ = closer.Close(c) // must not panic (double close of c.done)

	if got := wrapped.closes.Load(); got != 1 {
		t.Fatalf("underlying socket Close called %d time(s), want 1", got)
	}
}
