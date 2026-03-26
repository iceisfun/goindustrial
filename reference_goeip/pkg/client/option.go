package client

import (
	"time"

	"github.com/iceisfun/goeip/internal"
)

// Option configures a Client.
type Option func(*Client)

// WithRetries sets the number of retries for operations.
// 0 means no retries (fail immediately), -1 means infinite retries.
func WithRetries(n int) Option {
	return func(c *Client) {
		c.retries = n
	}
}

// WithRetryDelay sets the delay between retries. Default is 1 second.
func WithRetryDelay(d time.Duration) Option {
	return func(c *Client) {
		c.retryDelay = d
	}
}

// WithLogger overrides the logger on the client.
func WithLogger(l internal.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.logger = l
		}
	}
}
