package transport

import "context"

// Connector creates new connections of type C.
// Each protocol provides its own implementation:
//   - Modbus: dials TCP + starts MBAP framing
//   - EtherNet/IP: dials TCP + registers EIP session
type Connector[C any] interface {
	Connect(ctx context.Context) (C, error)
}

// Closer tears down a connection of type C.
type Closer[C any] interface {
	Close(conn C) error
}

// Transport manages the lifecycle of a protocol connection of type C.
// Two built-in implementations are provided: DirectTransport (connect once)
// and ReconnectingTransport (lazy connect, auto-reconnect).
type Transport[C any] interface {
	// Conn returns an active connection, creating one if necessary.
	Conn(ctx context.Context) (C, error)

	// Reset invalidates a stale connection. The stale parameter prevents
	// thundering-herd resets: only the goroutine holding the failed connection
	// triggers the invalidation.
	Reset(stale C) error

	// Close permanently shuts down the transport.
	//
	// TODO: Close does not accept a context.Context, so callers cannot bound
	// shutdown time. Adding a context parameter would be a breaking change to
	// this public interface and all implementations (DirectTransport,
	// ReconnectingTransport) and consumers (modbus.Client, ethernetip.Client).
	// Consider introducing a CloseWithContext method or a new interface in a
	// future major version.
	Close() error
}

// ConnectorFunc adapts a plain function into a Connector.
type ConnectorFunc[C any] func(ctx context.Context) (C, error)

func (f ConnectorFunc[C]) Connect(ctx context.Context) (C, error) { return f(ctx) }

// CloserFunc adapts a plain function into a Closer.
type CloserFunc[C any] func(conn C) error

func (f CloserFunc[C]) Close(conn C) error { return f(conn) }
