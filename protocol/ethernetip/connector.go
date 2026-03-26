package ethernetip

import (
	"context"

	"github.com/iceisfun/goindustrial/logging"
)

// SessionConnector creates new EIP sessions by dialing TCP and registering.
// It implements transport.Connector[*Session].
type SessionConnector struct {
	address  string
	logger   logging.Logger
	connOpts []ConnOption
}

// NewSessionConnector creates a SessionConnector for the given address.
func NewSessionConnector(address string, logger logging.Logger, connOpts ...ConnOption) *SessionConnector {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &SessionConnector{
		address:  address,
		logger:   logger,
		connOpts: connOpts,
	}
}

// Connect dials TCP, creates a session, and registers it.
func (c *SessionConnector) Connect(ctx context.Context) (*Session, error) {
	tc, err := NewTCPConn(c.address, c.connOpts...)
	if err != nil {
		return nil, err
	}

	s := NewSession(tc, c.logger)
	if err := s.Register(ctx); err != nil {
		tc.Close()
		return nil, err
	}

	return s, nil
}

// SessionCloser tears down an EIP session by unregistering and closing the
// underlying connection. It implements transport.Closer[*Session].
type SessionCloser struct{}

// Close unregisters and closes the session.
func (c SessionCloser) Close(sess *Session) error {
	// Use a background context since Close has no ctx parameter.
	sess.Unregister(context.Background())
	return sess.Close()
}
