package ethernetip

import (
	"net"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// ---------- ConnOption ----------

// connConfig holds configuration for TCPConn.
type connConfig struct {
	conn        net.Conn
	dialTimeout time.Duration
}

// ConnOption configures a TCPConn.
type ConnOption func(*connConfig)

// WithConn injects a pre-existing net.Conn, bypassing the TCP dial.
// This is useful for testing with net.Pipe.
func WithConn(conn net.Conn) ConnOption {
	return func(cfg *connConfig) {
		cfg.conn = conn
	}
}

// WithDialTimeout sets the timeout for the initial TCP dial.
// Default is 5 seconds.
func WithDialTimeout(d time.Duration) ConnOption {
	return func(cfg *connConfig) {
		cfg.dialTimeout = d
	}
}

// ---------- ClientOption ----------

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithRetries sets the number of retries for operations.
// 0 means no retries (fail immediately), -1 means infinite retries.
func WithRetries(n int) ClientOption {
	return func(c *Client) {
		c.retries = n
	}
}

// WithRetryDelay sets the delay between retries. Default is 1 second.
func WithRetryDelay(d time.Duration) ClientOption {
	return func(c *Client) {
		c.retryDelay = d
	}
}

// WithLogger overrides the logger on the client.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}
