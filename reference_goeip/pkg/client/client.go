package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/session"
)

// cipError wraps CIP-level errors that should not be retried.
type cipError struct{ err error }

func (e *cipError) Error() string { return e.err.Error() }
func (e *cipError) Unwrap() error { return e.err }

// Client is a high-level EIP client that uses a Transport to manage sessions.
type Client struct {
	transport  Transport
	logger     internal.Logger
	retries    int
	retryDelay time.Duration
}

// NewClient creates a new client backed by the given transport.
func NewClient(t Transport, opts ...Option) *Client {
	c := &Client{
		transport:  t,
		logger:     internal.NopLogger(),
		retries:    0,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect is a convenience constructor that creates a direct (non-reconnecting)
// client. It connects immediately and returns an error on failure.
func Connect(address string, logger internal.Logger, opts ...Option) (*Client, error) {
	if logger == nil {
		logger = internal.NopLogger()
	}
	t, err := NewDirectTransport(address, logger)
	if err != nil {
		return nil, err
	}
	opts = append([]Option{WithLogger(logger)}, opts...)
	return NewClient(t, opts...), nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.transport.Close()
}

// do executes op with retry logic. On transport errors it resets the transport
// and retries. CIP-level errors (wrapped in cipError) are not retried.
func (c *Client) do(op func(*session.Session) error) error {
	for i := 0; c.retries < 0 || i <= c.retries; i++ {
		sess, err := c.transport.Session()
		if err != nil {
			if c.retries == 0 {
				return err
			}
			c.logger.Warnf("Session unavailable (attempt %d): %v", i+1, err)
			if c.retries < 0 || i < c.retries {
				time.Sleep(c.retryDelay)
			}
			continue
		}

		err = op(sess)
		if err == nil {
			return nil
		}

		// CIP errors are not retryable
		var ce *cipError
		if errors.As(err, &ce) {
			return ce.err
		}

		// Transport error: reset and retry
		c.transport.Reset(sess)
		c.logger.Warnf("Operation failed (attempt %d): %v", i+1, err)

		if c.retries < 0 || i < c.retries {
			time.Sleep(c.retryDelay)
		}

		// Prevent integer overflow for infinite retries
		if i == 2147483647 {
			i = 0
		}
	}

	return fmt.Errorf("operation failed after %d retries", c.retries+1)
}

// ReadTag reads a tag from the PLC.
func (c *Client) ReadTag(tagName string) ([]byte, error) {
	return c.ReadTagElements(tagName, 1)
}

// ReadTagElements reads multiple elements of a tag from the PLC.
// For arrays, count specifies how many elements to read starting from element 0.
// For single values, use count=1.
// Returns raw bytes including the type code (first 2 bytes).
func (c *Client) ReadTagElements(tagName string, count uint16) ([]byte, error) {
	if count == 0 {
		return nil, fmt.Errorf("element count must be at least 1")
	}

	p := cip.NewPath()
	p.AddSymbolicSegment(tagName)
	req := cip.NewReadTagRequest(p, count)

	var result []byte
	err := c.do(func(sess *session.Session) error {
		resp, err := sess.SendCIPRequest(req)
		if err != nil {
			return err
		}
		if err := resp.Error(); err != nil {
			return &cipError{err}
		}
		result = resp.ResponseData
		return nil
	})
	return result, err
}

// WriteTag writes a value to a tag on the PLC.
// The value must be a basic Go type (int, float, etc.) or implement cip.Marshaler.
func (c *Client) WriteTag(tagName string, value any) error {
	// CIP encoding done outside the retry loop to fail-fast on invalid input
	p := cip.NewPath()
	p.AddSymbolicSegment(tagName)

	dataType, err := cip.GoTypeToCIPType(value)
	if err != nil {
		return err
	}

	data, err := cip.Marshal(value)
	if err != nil {
		return err
	}

	req := cip.NewWriteTagRequest(p, dataType, 1, data)

	return c.do(func(sess *session.Session) error {
		resp, err := sess.SendCIPRequest(req)
		if err != nil {
			return err
		}
		if err := resp.Error(); err != nil {
			return &cipError{err}
		}
		return nil
	})
}

// ReadTagInto reads a tag from the PLC and unmarshals it into dst.
// dst must be a pointer to a type that can be unmarshaled (basic type, struct, or Unmarshaler).
func (c *Client) ReadTagInto(tagName string, dst any) error {
	return c.ReadTagElementsInto(tagName, 1, dst)
}

// ReadTagElementsInto reads multiple elements of a tag and unmarshals them into dst.
// dst must be a pointer to an array or slice that can hold count elements.
func (c *Client) ReadTagElementsInto(tagName string, count uint16, dst any) error {
	data, err := c.ReadTagElements(tagName, count)
	if err != nil {
		return err
	}

	if len(data) < 2 {
		return fmt.Errorf("response too short to contain type code")
	}

	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return fmt.Errorf("response too short for type header")
	}

	return cip.Unmarshal(data[hdrLen:], dst)
}

// ReadTimer reads a Timer tag from the PLC and decodes it.
func (c *Client) ReadTimer(tagName string) (*cip.Timer, error) {
	data, err := c.ReadTag(tagName)
	if err != nil {
		return nil, err
	}

	if len(data) < 2 {
		return nil, fmt.Errorf("response too short to contain type code")
	}

	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return nil, fmt.Errorf("response too short for type header")
	}

	return cip.DecodeTimer(data[hdrLen:])
}
