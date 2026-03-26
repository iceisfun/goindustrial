package client

import (
	"fmt"
	"sync"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/session"
	"github.com/iceisfun/goeip/pkg/transport"
)

// directTransport connects once and does not reconnect on failure.
type directTransport struct {
	mu      sync.Mutex
	sess    *session.Session
	closed  bool
	cfg     transportConfig
	logger  internal.Logger
}

// NewDirectTransport creates a transport that connects immediately and returns
// an error if the connection or session registration fails.
func NewDirectTransport(address string, logger internal.Logger, opts ...TransportOption) (*directTransport, error) {
	if logger == nil {
		logger = internal.NopLogger()
	}

	var cfg transportConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	t, err := transport.NewTCPTransport(address)
	if err != nil {
		return nil, err
	}

	s := session.NewSession(t, logger)
	if err := s.Register(); err != nil {
		t.Close()
		return nil, err
	}

	dt := &directTransport{
		sess:   s,
		cfg:    cfg,
		logger: logger,
	}

	if cfg.onConnect != nil {
		cfg.onConnect()
	}

	return dt, nil
}

// Session returns the pre-created session or an error if the transport is closed.
func (d *directTransport) Session() (*session.Session, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, fmt.Errorf("transport is closed")
	}
	if d.sess == nil {
		return nil, fmt.Errorf("session is not available")
	}
	return d.sess, nil
}

// Reset closes the stale session and sets it to nil. A direct transport cannot
// create a new session after reset.
func (d *directTransport) Reset(stale *session.Session) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.sess == nil || d.sess != stale {
		return nil
	}

	d.sess.Unregister()
	err := d.sess.Close()
	d.sess = nil

	if d.cfg.onDisconnect != nil {
		d.cfg.onDisconnect(err)
	}

	return err
}

// Close permanently shuts down the transport.
func (d *directTransport) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil
	}
	d.closed = true

	if d.sess == nil {
		return nil
	}

	d.sess.Unregister()
	err := d.sess.Close()

	if d.cfg.onDisconnect != nil {
		d.cfg.onDisconnect(err)
	}

	d.sess = nil
	return err
}
