package transport

import (
	"context"
	"fmt"
	"sync"
)

// ReconnectingTransport creates connections lazily and re-creates them after
// failures. It uses an RWMutex double-check locking pattern so that concurrent
// readers share the fast path while only one goroutine performs reconnection.
type ReconnectingTransport[C comparable] struct {
	connector Connector[C]
	closer    Closer[C]
	cfg       Config

	mu     sync.RWMutex
	conn   C
	zero   C
	closed bool
}

// NewReconnectingTransport creates a transport that connects lazily and
// reconnects on failure. The constructor never fails and never connects.
func NewReconnectingTransport[C comparable](connector Connector[C], closer Closer[C], opts ...Option) *ReconnectingTransport[C] {
	cfg := applyOptions(opts)
	return &ReconnectingTransport[C]{
		connector: connector,
		closer:    closer,
		cfg:       cfg,
	}
}

// Conn returns the current connection, creating a new one via the Connector
// if none exists. Concurrent callers share the fast read-lock path when a
// connection is already established.
func (r *ReconnectingTransport[C]) Conn(ctx context.Context) (C, error) {
	// Fast path: read lock
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return r.zero, fmt.Errorf("transport is closed")
	}
	conn := r.conn
	r.mu.RUnlock()

	if conn != r.zero {
		return conn, nil
	}

	// Slow path: write lock, double-check
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return r.zero, fmt.Errorf("transport is closed")
	}
	if r.conn != r.zero {
		return r.conn, nil
	}

	conn, err := r.connector.Connect(ctx)
	if err != nil {
		return r.zero, err
	}

	r.conn = conn

	if r.cfg.OnConnect != nil {
		r.cfg.OnConnect()
	}

	return conn, nil
}

// Peek reports whether a connection is currently established without
// attempting to create one. It implements the [Peeker] interface.
func (r *ReconnectingTransport[C]) Peek() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return !r.closed && r.conn != r.zero
}

// Reset invalidates the connection if it matches stale, closing the
// underlying session. The next Conn call will transparently establish a
// new connection.
func (r *ReconnectingTransport[C]) Reset(stale C) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn == r.zero || r.conn != stale {
		return nil
	}

	err := r.closer.Close(r.conn)
	r.conn = r.zero

	if r.cfg.OnDisconnect != nil {
		r.cfg.OnDisconnect(err)
	}

	return nil
}

// Close permanently shuts down the transport and closes any active
// connection. It is safe to call multiple times.
func (r *ReconnectingTransport[C]) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	if r.conn == r.zero {
		return nil
	}

	err := r.closer.Close(r.conn)

	if r.cfg.OnDisconnect != nil {
		r.cfg.OnDisconnect(err)
	}

	r.conn = r.zero
	return err
}
