package ethernetip

import (
	"context"

	"github.com/iceisfun/goindustrial/logging"
)

// SessionConnector creates new EtherNet/IP sessions by dialing TCP and
// sending a RegisterSession command. It implements
// transport.Connector[*Session] and is used internally by [Connect] and
// [NewReconnectingClient].
type SessionConnector struct {
	address  string
	logger   logging.Logger
	connOpts []ConnOption
}

// NewSessionConnector creates a SessionConnector that will dial the given
// address. If logger is nil a no-op logger is used.
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

// Connect dials TCP, creates a [Session], and sends RegisterSession to obtain
// a session handle from the target device.
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

// SessionCloser tears down an EtherNet/IP session by sending
// UnregisterSession and closing the underlying TCP connection. It implements
// transport.Closer[*Session].
type SessionCloser struct{}

// Close sends UnregisterSession and closes the underlying TCP connection.
func (c SessionCloser) Close(sess *Session) error {
	// Use a background context since Close has no ctx parameter.
	sess.Unregister(context.Background())
	return sess.Close()
}
