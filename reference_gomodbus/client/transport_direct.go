package client

import (
	"context"
	"fmt"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// directTransport connects once and does not reconnect on failure.
type directTransport struct {
	mu     sync.Mutex
	conn   common.Transport
	closed bool
	cfg    transportConfig
	logger common.LoggerInterface
}

// NewDirectTransport creates a transport that connects immediately and returns
// an error if the connection fails. The onConnect callback fires on success.
func NewDirectTransport(ctx context.Context, host string, logger common.LoggerInterface, transportOpts []TransportOption, tcpOpts []transport.TCPTransportOption) (*directTransport, error) {
	if logger == nil {
		logger = logging.NewLogger()
	}

	var cfg transportConfig
	for _, opt := range transportOpts {
		opt(&cfg)
	}

	tcpTransport := transport.NewTCPTransport(host, tcpOpts...)
	tcpTransport.WithLogger(logger)

	if err := tcpTransport.Connect(ctx); err != nil {
		return nil, err
	}

	dt := &directTransport{
		conn:   tcpTransport,
		cfg:    cfg,
		logger: logger,
	}

	if cfg.onConnect != nil {
		cfg.onConnect()
	}

	return dt, nil
}

// Conn returns the pre-created transport or an error if the transport is closed
// or has been reset.
func (d *directTransport) Conn(ctx context.Context) (common.Transport, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return nil, fmt.Errorf("transport is closed")
	}
	if d.conn == nil {
		return nil, fmt.Errorf("connection is not available")
	}
	return d.conn, nil
}

// Reset closes the stale transport and sets it to nil. A direct transport
// cannot create a new connection after reset — subsequent Conn calls return error.
func (d *directTransport) Reset(stale common.Transport) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn == nil || d.conn != stale {
		return nil
	}

	err := d.conn.Disconnect(context.Background())
	d.conn = nil

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

	if d.conn == nil {
		return nil
	}

	err := d.conn.Disconnect(context.Background())

	if d.cfg.onDisconnect != nil {
		d.cfg.onDisconnect(err)
	}

	d.conn = nil
	return err
}
