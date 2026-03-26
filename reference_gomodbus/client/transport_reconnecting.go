package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// reconnectingTransport creates connections lazily and re-creates them after
// failures. It uses an RWMutex double-check locking pattern for efficiency.
type reconnectingTransport struct {
	host    string
	tcpOpts []transport.TCPTransportOption
	logger  common.LoggerInterface
	cfg     transportConfig

	mu     sync.RWMutex
	conn   common.Transport
	closed bool
}

// NewReconnectingTransport creates a transport that connects lazily and
// reconnects on failure. The constructor never fails and never connects.
func NewReconnectingTransport(host string, logger common.LoggerInterface, transportOpts []TransportOption, tcpOpts []transport.TCPTransportOption) *reconnectingTransport {
	if logger == nil {
		logger = logging.NewLogger()
	}

	var cfg transportConfig
	for _, opt := range transportOpts {
		opt(&cfg)
	}

	return &reconnectingTransport{
		host:    host,
		tcpOpts: tcpOpts,
		logger:  logger,
		cfg:     cfg,
	}
}

// Conn returns the current transport or creates a new one if needed.
func (r *reconnectingTransport) Conn(ctx context.Context) (common.Transport, error) {
	// Fast path: read lock
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, fmt.Errorf("transport is closed")
	}
	conn := r.conn
	r.mu.RUnlock()

	if conn != nil {
		return conn, nil
	}

	// Slow path: write lock, double-check
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, fmt.Errorf("transport is closed")
	}
	if r.conn != nil {
		return r.conn, nil
	}

	conn, err := r.connect(ctx)
	if err != nil {
		return nil, err
	}

	r.conn = conn

	if r.cfg.onConnect != nil {
		r.cfg.onConnect()
	}

	return conn, nil
}

// Reset invalidates the stale transport if it is still the current one.
// The next Conn call will create a fresh connection.
func (r *reconnectingTransport) Reset(stale common.Transport) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.conn == nil || r.conn != stale {
		return nil
	}

	err := r.conn.Disconnect(context.Background())
	r.conn = nil

	if r.cfg.onDisconnect != nil {
		r.cfg.onDisconnect(err)
	}

	return nil
}

// Close permanently shuts down the transport.
func (r *reconnectingTransport) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	if r.conn == nil {
		return nil
	}

	err := r.conn.Disconnect(context.Background())

	if r.cfg.onDisconnect != nil {
		r.cfg.onDisconnect(err)
	}

	r.conn = nil
	return err
}

// connect creates a new TCPTransport and connects it.
func (r *reconnectingTransport) connect(ctx context.Context) (common.Transport, error) {
	t := transport.NewTCPTransport(r.host, r.tcpOpts...)
	t.WithLogger(r.logger)

	if err := t.Connect(ctx); err != nil {
		return nil, err
	}

	return t, nil
}
