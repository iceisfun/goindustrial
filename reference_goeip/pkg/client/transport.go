package client

import "github.com/iceisfun/goeip/pkg/session"

// Transport abstracts how a Client obtains a usable session.
// Two built-in implementations are provided: directTransport (connect once)
// and reconnectingTransport (lazy connect, auto-reconnect).
type Transport interface {
	// Session returns an active session, creating one if necessary.
	Session() (*session.Session, error)

	// Reset invalidates a stale session. The stale parameter prevents
	// thundering-herd resets: only the goroutine holding the failed session
	// triggers the invalidation.
	Reset(stale *session.Session) error

	// Close permanently shuts down the transport.
	Close() error
}

// TransportOption configures transport lifecycle hooks.
type TransportOption func(*transportConfig)

type transportConfig struct {
	onConnect    func()
	onDisconnect func(error)
}

// WithOnConnect registers a callback that fires after a session is established.
func WithOnConnect(fn func()) TransportOption {
	return func(cfg *transportConfig) {
		cfg.onConnect = fn
	}
}

// WithOnDisconnect registers a callback that fires when a session is lost.
func WithOnDisconnect(fn func(error)) TransportOption {
	return func(cfg *transportConfig) {
		cfg.onDisconnect = fn
	}
}
