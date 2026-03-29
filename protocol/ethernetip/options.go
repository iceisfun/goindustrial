package ethernetip

import (
	"io"
	"net"
	"time"

	"github.com/iceisfun/goindustrial/hexdump"
	"github.com/iceisfun/goindustrial/logging"
)

// ---------- ConnOption ----------

// connConfig holds configuration for TCPConn.
type connConfig struct {
	conn        net.Conn
	dialTimeout time.Duration
	hexDumper   *hexdump.Dumper
}

// ConnOption configures a [TCPConn] created by [NewTCPConn].
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

// WithHexDump enables hex dump tracing of all EtherNet/IP wire traffic. Data
// read from and written to the TCP connection is formatted in traditional hex
// dump style and written to out. Pass [os.Stdout] for console debugging or an
// [os.File] for offline analysis.
func WithHexDump(out io.Writer) ConnOption {
	return func(cfg *connConfig) {
		cfg.hexDumper = hexdump.NewDumper(out)
	}
}

// ---------- ClientOption ----------

// ClientOption configures a [Client] created by [NewClient], [Connect], or
// [NewReconnectingClient].
type ClientOption func(*Client)

// WithRetries sets the maximum number of retry attempts for failed operations.
// A value of 0 means no retries (fail immediately), and -1 means infinite
// retries.
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

// WithLogger sets the logger used by the client for diagnostic output. If l
// is nil the call is a no-op.
func WithLogger(l logging.Logger) ClientOption {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}
