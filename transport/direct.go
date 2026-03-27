package transport

import (
	"context"
	"fmt"
	"sync"
)

// DirectTransport connects once via Connector and does not reconnect on failure.
// After Close or Reset, subsequent Conn calls return an error.
type DirectTransport[C comparable] struct {
	connector Connector[C]
	closer    Closer[C]
	cfg       Config

	mu     sync.Mutex
	conn   C
	zero   C
	active bool
	closed bool
}

// NewDirectTransport creates a transport that connects immediately.
// It returns an error if the initial connection fails.
func NewDirectTransport[C comparable](ctx context.Context, connector Connector[C], closer Closer[C], opts ...Option) (*DirectTransport[C], error) {
	cfg := applyOptions(opts)

	conn, err := connector.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("direct transport connect: %w", err)
	}

	if cfg.OnConnect != nil {
		cfg.OnConnect()
	}

	return &DirectTransport[C]{
		connector: connector,
		closer:    closer,
		cfg:       cfg,
		conn:      conn,
		active:    true,
	}, nil
}

// Conn returns the established connection. It returns an error if the
// transport has been closed or the connection was previously reset.
func (d *DirectTransport[C]) Conn(ctx context.Context) (C, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return d.zero, fmt.Errorf("transport is closed")
	}
	if !d.active {
		return d.zero, fmt.Errorf("transport connection was reset")
	}
	return d.conn, nil
}

// Peek reports whether a connection is currently active without side effects.
// It implements the [Peeker] interface.
func (d *DirectTransport[C]) Peek() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active && !d.closed
}

// Reset invalidates the connection if it matches stale, closing the
// underlying session. Subsequent Conn calls will return an error because
// DirectTransport does not reconnect.
func (d *DirectTransport[C]) Reset(stale C) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.active || d.conn != stale {
		return nil
	}

	err := d.closer.Close(d.conn)
	d.conn = d.zero
	d.active = false

	if d.cfg.OnDisconnect != nil {
		d.cfg.OnDisconnect(err)
	}

	return nil
}

// Close permanently shuts down the transport and closes the underlying
// connection. It is safe to call multiple times.
func (d *DirectTransport[C]) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	if !d.active {
		return nil
	}

	err := d.closer.Close(d.conn)
	d.active = false

	if d.cfg.OnDisconnect != nil {
		d.cfg.OnDisconnect(err)
	}

	d.conn = d.zero
	return err
}
