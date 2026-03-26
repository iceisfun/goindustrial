package client

import (
	"fmt"
	"sync"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/session"
	pkgtransport "github.com/iceisfun/goeip/pkg/transport"
)

// reconnectingTransport creates sessions lazily and re-creates them after
// failures. It uses an RWMutex double-check locking pattern for efficiency.
type reconnectingTransport struct {
	address string
	logger  internal.Logger
	cfg     transportConfig

	mu     sync.RWMutex
	sess   *session.Session
	closed bool
}

// NewReconnectingTransport creates a transport that connects lazily and
// reconnects on failure. The constructor never fails.
func NewReconnectingTransport(address string, logger internal.Logger, opts ...TransportOption) *reconnectingTransport {
	if logger == nil {
		logger = internal.NopLogger()
	}

	var cfg transportConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	return &reconnectingTransport{
		address: address,
		logger:  logger,
		cfg:     cfg,
	}
}

// Session returns the current session or creates a new one if needed.
func (r *reconnectingTransport) Session() (*session.Session, error) {
	// Fast path: read lock
	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil, fmt.Errorf("transport is closed")
	}
	s := r.sess
	r.mu.RUnlock()

	if s != nil {
		return s, nil
	}

	// Slow path: write lock, double-check
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, fmt.Errorf("transport is closed")
	}
	if r.sess != nil {
		return r.sess, nil
	}

	s, err := r.connect()
	if err != nil {
		return nil, err
	}

	r.sess = s

	if r.cfg.onConnect != nil {
		r.cfg.onConnect()
	}

	return s, nil
}

// Reset invalidates the stale session if it is still the current one.
func (r *reconnectingTransport) Reset(stale *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.sess == nil || r.sess != stale {
		return nil
	}

	r.sess.Unregister()
	err := r.sess.Close()
	r.sess = nil

	if r.cfg.onDisconnect != nil {
		r.cfg.onDisconnect(err)
	}

	return err
}

// Close permanently shuts down the transport.
func (r *reconnectingTransport) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}
	r.closed = true

	if r.sess == nil {
		return nil
	}

	r.sess.Unregister()
	err := r.sess.Close()

	if r.cfg.onDisconnect != nil {
		r.cfg.onDisconnect(err)
	}

	r.sess = nil
	return err
}

func (r *reconnectingTransport) connect() (*session.Session, error) {
	t, err := pkgtransport.NewTCPTransport(r.address)
	if err != nil {
		return nil, err
	}

	s := session.NewSession(t, r.logger)
	if err := s.Register(); err != nil {
		t.Close()
		return nil, err
	}

	return s, nil
}
