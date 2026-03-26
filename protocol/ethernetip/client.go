package ethernetip

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
	"github.com/iceisfun/goindustrial/transport"
)

// cipError wraps CIP-level errors that should not be retried.
type cipError struct{ err error }

func (e *cipError) Error() string { return e.err.Error() }
func (e *cipError) Unwrap() error { return e.err }

// Client is a high-level EtherNet/IP client that uses a generic transport to
// manage sessions. It implements the plc.PLC interface.
type Client struct {
	transport  transport.Transport[*Session]
	logger     logging.Logger
	retries    int
	retryDelay time.Duration
}

// Compile-time check that Client implements plc.PLC.
var _ plc.PLC = (*Client)(nil)

// NewClient creates a new client backed by the given transport.
func NewClient(t transport.Transport[*Session], opts ...ClientOption) *Client {
	c := &Client{
		transport:  t,
		logger:     logging.NewNopLogger(),
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
func Connect(ctx context.Context, address string, opts ...ClientOption) (*Client, error) {
	// Extract logger from options (peek).
	c := &Client{
		logger:     logging.NewNopLogger(),
		retries:    0,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}

	connector := NewSessionConnector(address, c.logger)
	closer := SessionCloser{}

	dt, err := transport.NewDirectTransport[*Session](ctx, connector, closer)
	if err != nil {
		return nil, err
	}

	c.transport = dt
	return c, nil
}

// NewReconnectingClient creates a client that connects lazily and reconnects on
// failure. The constructor never fails and never connects immediately.
func NewReconnectingClient(address string, opts ...ClientOption) *Client {
	c := &Client{
		logger:     logging.NewNopLogger(),
		retries:    0,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}

	connector := NewSessionConnector(address, c.logger)
	closer := SessionCloser{}

	c.transport = transport.NewReconnectingTransport[*Session](connector, closer)
	return c
}

// ---------- plc.PLC interface ----------

// Connect establishes the connection. For a direct transport this is a no-op
// (already connected). For a reconnecting transport the first Conn call will
// trigger the dial.
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.transport.Conn(ctx)
	return err
}

// Disconnect closes the transport.
func (c *Client) Disconnect(_ context.Context) error {
	return c.transport.Close()
}

// IsConnected returns true if a session is currently available.
func (c *Client) IsConnected() bool {
	_, err := c.transport.Conn(context.Background())
	return err == nil
}

// Read reads one or more data points. Each DataPoint must be a Tag.
func (c *Client) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	values := make([]plc.Value, 0, len(points))
	for _, dp := range points {
		tag, ok := dp.(Tag)
		if !ok {
			return nil, fmt.Errorf("ethernetip: expected Tag DataPoint, got %T", dp)
		}
		elements := tag.Elements
		if elements == 0 {
			elements = 1
		}
		raw, err := c.ReadTagElements(ctx, tag.Name, elements)
		if err != nil {
			return nil, err
		}
		values = append(values, plc.Value{
			DataPoint: dp,
			Raw:       raw,
		})
	}
	return values, nil
}

// Write writes data to a tag on the controller.
func (c *Client) Write(ctx context.Context, point plc.DataPoint, data []byte) error {
	tag, ok := point.(Tag)
	if !ok {
		return fmt.Errorf("ethernetip: expected Tag DataPoint, got %T", point)
	}

	// We need at least 2 bytes for the type code in the raw data.
	if len(data) < 2 {
		return fmt.Errorf("ethernetip: write data must include 2-byte type code prefix")
	}

	dataType := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	elements := tag.Elements
	if elements == 0 {
		elements = 1
	}

	p := cip.NewPath()
	p.AddSymbolicSegment(tag.Name)
	req := cip.NewWriteTagRequest(p, dataType, elements, data[2:])

	return c.do(ctx, func(sess *Session) error {
		resp, err := sess.SendCIPRequest(ctx, req)
		if err != nil {
			return err
		}
		if err := resp.Error(); err != nil {
			return &cipError{err}
		}
		return nil
	})
}

// Close closes the client transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

// ---------- Tag-level API ----------

// ReadTag reads a single element of a tag from the PLC.
// Returns raw bytes including the type code (first 2 bytes).
func (c *Client) ReadTag(ctx context.Context, tagName string) ([]byte, error) {
	return c.ReadTagElements(ctx, tagName, 1)
}

// ReadTagElements reads multiple elements of a tag from the PLC.
// Returns raw bytes including the type code (first 2 bytes).
func (c *Client) ReadTagElements(ctx context.Context, tagName string, count uint16) ([]byte, error) {
	if count == 0 {
		return nil, fmt.Errorf("element count must be at least 1")
	}

	p := cip.NewPath()
	p.AddSymbolicSegment(tagName)
	req := cip.NewReadTagRequest(p, count)

	var result []byte
	err := c.do(ctx, func(sess *Session) error {
		resp, err := sess.SendCIPRequest(ctx, req)
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

// WriteTag writes a Go value to a tag on the PLC.
// The value must be a basic Go type (int, float, etc.) or implement cip.Marshaler.
func (c *Client) WriteTag(ctx context.Context, tagName string, value any) error {
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

	return c.do(ctx, func(sess *Session) error {
		resp, err := sess.SendCIPRequest(ctx, req)
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
// dst must be a pointer to a type that can be unmarshaled.
func (c *Client) ReadTagInto(ctx context.Context, tagName string, dst any) error {
	return c.ReadTagElementsInto(ctx, tagName, 1, dst)
}

// ReadTagElementsInto reads multiple elements of a tag and unmarshals them into dst.
func (c *Client) ReadTagElementsInto(ctx context.Context, tagName string, count uint16, dst any) error {
	data, err := c.ReadTagElements(ctx, tagName, count)
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
func (c *Client) ReadTimer(ctx context.Context, tagName string) (*cip.Timer, error) {
	data, err := c.ReadTag(ctx, tagName)
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

// ---------- Discovery / Enumeration ----------

// ListIdentity sends the ListIdentity command via the current session.
func (c *Client) ListIdentity(ctx context.Context) ([]eip.ListIdentityItem, error) {
	var result []eip.ListIdentityItem
	err := c.do(ctx, func(sess *Session) error {
		items, err := sess.ListIdentity(ctx)
		if err != nil {
			return err
		}
		result = items
		return nil
	})
	return result, err
}

// ListServices sends the ListServices command via the current session.
func (c *Client) ListServices(ctx context.Context) ([]eip.ListServicesItem, error) {
	var result []eip.ListServicesItem
	err := c.do(ctx, func(sess *Session) error {
		items, err := sess.ListServices(ctx)
		if err != nil {
			return err
		}
		result = items
		return nil
	})
	return result, err
}

// ListTags enumerates all tags on the PLC by iterating the Symbol Object.
// The entire enumeration runs inside a single do() call so a mid-enumeration
// connection drop retries from scratch.
func (c *Client) ListTags(ctx context.Context) ([]cip.SymbolInstance, error) {
	var result []cip.SymbolInstance
	err := c.do(ctx, func(sess *Session) error {
		reqClass := cip.NewGetSymbolClassAttributesRequest()
		respClass, err := sess.SendCIPRequest(ctx, reqClass)
		if err != nil {
			return err
		}
		if !respClass.IsSuccess() {
			return &cipError{respClass.Error()}
		}

		_, maxInstance, err := cip.DecodeSymbolClassAttributesResponse(respClass.ResponseData)
		if err != nil {
			return fmt.Errorf("failed to decode symbol class attributes: %w", err)
		}

		c.logger.Info(ctx, "Max Symbol Instance: %d", maxInstance)

		var allSymbols []cip.SymbolInstance

		for id := uint32(1); id <= uint32(maxInstance); id++ {
			req := cip.NewGetSymbolAttributesRequest(id)
			resp, err := sess.SendCIPRequest(ctx, req)
			if err != nil {
				c.logger.Warn(ctx, "Failed to fetch attributes for instance %d: %v", id, err)
				continue
			}

			if !resp.IsSuccess() {
				if resp.GeneralStatus == cip.StatusObjectDoesNotExist || resp.GeneralStatus == cip.StatusPathDestinationUnknown {
					continue
				}
				continue
			}

			name, typeCode, err := cip.DecodeSymbolAttributesResponse(resp.ResponseData)
			if err != nil {
				c.logger.Warn(ctx, "Failed to decode attributes for instance %d: %v", id, err)
				continue
			}

			if name != "" {
				allSymbols = append(allSymbols, cip.SymbolInstance{
					InstanceID: id,
					Name:       name,
					Type:       typeCode,
				})
			}
		}

		result = allSymbols
		return nil
	})
	return result, err
}

// ---------- Generic helpers ----------

// Read reads a single tag value from the PLC and returns it as type T.
// T must be a fixed-size type compatible with binary.Read or implement
// cip.Unmarshaler.
func Read[T any](c *Client, ctx context.Context, tagName string) (T, error) {
	var zero T
	data, err := c.ReadTag(ctx, tagName)
	if err != nil {
		return zero, err
	}
	if len(data) < 2 {
		return zero, fmt.Errorf("response too short to contain type code")
	}
	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return zero, fmt.Errorf("response too short for type header")
	}
	var result T
	if err := cip.Unmarshal(data[hdrLen:], &result); err != nil {
		return zero, err
	}
	return result, nil
}

// ReadSlice reads count elements of a tag and returns them as []T.
// T must be a fixed-size type compatible with binary.Read.
func ReadSlice[T any](c *Client, ctx context.Context, tagName string, count uint16) ([]T, error) {
	data, err := c.ReadTagElements(ctx, tagName, count)
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
	result := make([]T, count)
	if err := cip.Unmarshal(data[hdrLen:], &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ---------- Internal retry loop ----------

// do executes op with retry logic. On transport errors it resets the transport
// and retries. CIP-level errors (wrapped in cipError) are not retried.
func (c *Client) do(ctx context.Context, op func(*Session) error) error {
	for i := 0; c.retries < 0 || i <= c.retries; i++ {
		sess, err := c.transport.Conn(ctx)
		if err != nil {
			if c.retries == 0 {
				return err
			}
			c.logger.Warn(ctx, "Session unavailable (attempt %d): %v", i+1, err)
			if c.retries < 0 || i < c.retries {
				time.Sleep(c.retryDelay)
			}
			continue
		}

		err = op(sess)
		if err == nil {
			return nil
		}

		// CIP errors are not retryable.
		var ce *cipError
		if errors.As(err, &ce) {
			return ce.err
		}

		// Transport error: reset and retry.
		c.transport.Reset(sess)
		c.logger.Warn(ctx, "Operation failed (attempt %d): %v", i+1, err)

		if c.retries < 0 || i < c.retries {
			time.Sleep(c.retryDelay)
		}

		// Prevent integer overflow for infinite retries.
		if i == 2147483647 {
			i = 0
		}
	}

	return fmt.Errorf("operation failed after %d retries", c.retries+1)
}
