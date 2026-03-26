package common

import "context"

// Transport is an abstraction for the underlying transport mechanism.
// It provides a common interface for TCP, RTU, etc.
type Transport interface {
	// Connect establishes a connection.
	Connect(ctx context.Context) error
	// Disconnect closes the connection.
	Disconnect(ctx context.Context) error
	// IsConnected returns true if connected.
	IsConnected() bool
	// Send sends a request and returns the response.
	Send(ctx context.Context, request Request) (Response, error)
	// WithLogger sets the logger for the transport.
	WithLogger(logger LoggerInterface) Transport
}
