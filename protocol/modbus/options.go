package modbus

import (
	"net"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// ---------------------------------------------------------------------------
// TCPConn options
// ---------------------------------------------------------------------------

// TCPConnOption is a functional option for configuring a TCPConn.
type TCPConnOption func(*TCPConn)

// WithPort sets the TCP port for the connection.
func WithPort(port int) TCPConnOption {
	return func(c *TCPConn) {
		c.port = port
	}
}

// WithTimeout sets the dial/connection timeout.
func WithTimeout(timeout time.Duration) TCPConnOption {
	return func(c *TCPConn) {
		c.timeout = timeout
	}
}

// WithConn injects a pre-existing net.Conn (e.g. from net.Pipe) for testing.
// When set, Connect will use this connection instead of dialing TCP.
func WithConn(conn net.Conn) TCPConnOption {
	return func(c *TCPConn) {
		c.injectedConn = conn
	}
}

// WithConnLogger sets the logger for the TCPConn.
func WithConnLogger(logger logging.Logger) TCPConnOption {
	return func(c *TCPConn) {
		c.logger = logger
	}
}

// ---------------------------------------------------------------------------
// Client options
// ---------------------------------------------------------------------------

// ClientOption is a functional option for configuring a Client.
type ClientOption func(*Client)

// WithRetries sets the maximum number of retries for transport errors.
func WithRetries(retries int) ClientOption {
	return func(c *Client) {
		c.retries = retries
	}
}

// WithRetryDelay sets the delay between retries.
func WithRetryDelay(delay time.Duration) ClientOption {
	return func(c *Client) {
		c.retryDelay = delay
	}
}

// WithLogger sets the logger for the client.
func WithLogger(logger logging.Logger) ClientOption {
	return func(c *Client) {
		c.logger = logger
	}
}

// WithUnitID sets the Modbus unit ID (slave address) for the client.
func WithUnitID(unitID UnitID) ClientOption {
	return func(c *Client) {
		c.unitID = unitID
	}
}
