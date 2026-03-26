package client

import (
	"context"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
)

// Transport abstracts how a client obtains a usable connection.
// Two built-in implementations are provided: directTransport (connect once)
// and reconnectingTransport (lazy connect, auto-reconnect).
type Transport interface {
	// Conn returns an active transport, creating one if necessary.
	Conn(ctx context.Context) (common.Transport, error)

	// Reset invalidates a stale transport. The stale parameter prevents
	// thundering-herd resets: only the goroutine holding the failed transport
	// triggers the invalidation.
	Reset(stale common.Transport) error

	// Close permanently shuts down the transport.
	Close() error
}

// TransportOption configures transport lifecycle hooks.
type TransportOption func(*transportConfig)

type transportConfig struct {
	onConnect    func()
	onDisconnect func(error)
}

// WithOnConnect registers a callback that fires after a connection is established.
func WithOnConnect(fn func()) TransportOption {
	return func(cfg *transportConfig) {
		cfg.onConnect = fn
	}
}

// WithOnDisconnect registers a callback that fires when a connection is lost.
func WithOnDisconnect(fn func(error)) TransportOption {
	return func(cfg *transportConfig) {
		cfg.onDisconnect = fn
	}
}

// transportBridge adapts a Transport into a common.Transport so it can be
// passed to NewBaseClient without modifying BaseClient.
type transportBridge struct {
	ct     Transport
	logger common.LoggerInterface
	mu     sync.Mutex
}

// Connect establishes a connection by calling Conn on the underlying Transport.
func (b *transportBridge) Connect(ctx context.Context) error {
	_, err := b.ct.Conn(ctx)
	return err
}

// Disconnect permanently closes the underlying Transport.
func (b *transportBridge) Disconnect(ctx context.Context) error {
	return b.ct.Close()
}

// IsConnected returns true. The Transport abstraction manages connection state
// internally; returning true ensures BaseClient.Send proceeds to bridge.Send
// where Conn handles lazy connection and reconnection.
func (b *transportBridge) IsConnected() bool {
	return true
}

// Send obtains the current transport via Conn, sends the request through it,
// and resets on transport-level errors (non-ModbusError). No retry is performed
// — that is the caller's concern.
func (b *transportBridge) Send(ctx context.Context, request common.Request) (common.Response, error) {
	conn, err := b.ct.Conn(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := conn.Send(ctx, request)
	if err != nil && !common.IsModbusError(err) {
		b.mu.Lock()
		resetErr := b.ct.Reset(conn)
		b.mu.Unlock()
		if resetErr != nil {
			b.logger.Error(ctx, "Failed to reset transport: %v", resetErr)
		}
	}
	return resp, err
}

// WithLogger returns a new transportBridge with the given logger.
func (b *transportBridge) WithLogger(logger common.LoggerInterface) common.Transport {
	return &transportBridge{
		ct:     b.ct,
		logger: logger,
	}
}

// newTransportBridge creates a transportBridge wrapping the given Transport.
func newTransportBridge(ct Transport) *transportBridge {
	return &transportBridge{
		ct:     ct,
		logger: logging.NewLogger(),
	}
}
