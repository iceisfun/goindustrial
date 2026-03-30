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

// Client is a high-level EtherNet/IP client that reads and writes PLC tags
// over TCP using explicit CIP messaging. It manages sessions through a
// pluggable transport layer and implements the plc.PLC interface.
// Use [Connect] for a direct connection, [NewReconnectingClient] for
// automatic reconnection, or [NewClient] with a custom transport.
type Client struct {
	transport  transport.Transport[*Session]
	logger     logging.Logger
	retries    int
	retryDelay time.Duration
}

// Compile-time check that Client implements plc.PLC.
var _ plc.PLC = (*Client)(nil)

// NewClient creates a new Client backed by the given transport. Apply
// [ClientOption] values to configure retries, logging, and other behavior.
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

// Connect is a convenience constructor that dials the given address over TCP,
// registers an EtherNet/IP session, and returns a ready-to-use Client. The
// connection is direct (non-reconnecting); if it drops, operations will fail
// until a new Client is created.
//
// Options may be [ClientOption], [ConnOption], or [transport.Option] values
// and are routed to the appropriate layer automatically.
func Connect(ctx context.Context, address string, opts ...any) (*Client, error) {
	var connOpts []ConnOption
	var clientOpts []ClientOption
	var transportOpts []transport.Option

	for _, o := range opts {
		switch v := o.(type) {
		case ConnOption:
			connOpts = append(connOpts, v)
		case ClientOption:
			clientOpts = append(clientOpts, v)
		case transport.Option:
			transportOpts = append(transportOpts, v)
		}
	}

	c := &Client{
		logger:     logging.NewNopLogger(),
		retries:    0,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range clientOpts {
		opt(c)
	}

	connector := NewSessionConnector(address, c.logger, connOpts...)
	closer := SessionCloser{}

	dt, err := transport.NewDirectTransport[*Session](ctx, connector, closer, transportOpts...)
	if err != nil {
		return nil, err
	}

	c.transport = dt
	return c, nil
}

// NewReconnectingClient creates a Client that connects lazily on the first
// operation and automatically reconnects after a transport failure. The
// constructor itself never dials, so it always returns successfully.
//
// Options may be [ClientOption], [ConnOption], or [transport.Option] values
// and are routed to the appropriate layer automatically.
func NewReconnectingClient(address string, opts ...any) *Client {
	var connOpts []ConnOption
	var clientOpts []ClientOption
	var transportOpts []transport.Option

	for _, o := range opts {
		switch v := o.(type) {
		case ConnOption:
			connOpts = append(connOpts, v)
		case ClientOption:
			clientOpts = append(clientOpts, v)
		case transport.Option:
			transportOpts = append(transportOpts, v)
		}
	}

	c := &Client{
		logger:     logging.NewNopLogger(),
		retries:    0,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range clientOpts {
		opt(c)
	}

	connector := NewSessionConnector(address, c.logger, connOpts...)
	closer := SessionCloser{}

	c.transport = transport.NewReconnectingTransport[*Session](connector, closer, transportOpts...)
	return c
}

// ---------- plc.PLC interface ----------

// Connect establishes the underlying TCP connection and EtherNet/IP session.
// For a direct transport this is a no-op (already connected). For a
// reconnecting transport the first call triggers the dial.
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.transport.Conn(ctx)
	return err
}

// Disconnect unregisters the EtherNet/IP session and closes the TCP connection.
func (c *Client) Disconnect(_ context.Context) error {
	return c.transport.Close()
}

// IsConnected returns true if a session is currently available. It does not
// attempt to establish a new connection — it only checks existing state.
func (c *Client) IsConnected() bool {
	if p, ok := c.transport.(transport.Peeker); ok {
		return p.Peek()
	}
	// Fallback for custom transports that don't implement Peeker.
	_, err := c.transport.Conn(context.Background())
	return err == nil
}

// Read reads one or more data points from the PLC. Each DataPoint must be a
// [Tag]. The returned Values contain the raw data bytes (with the CIP type
// code prefix stripped) and a protocol-agnostic type hint.
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

		val := plc.Value{
			DataPoint: dp,
			ByteOrder: plc.ByteOrderLittleEndian,
		}

		// The first 2 bytes of a CIP ReadTag response are the type code
		// (4 bytes for struct types). Extract the type hint and strip the
		// prefix so that Raw contains only the data payload.
		if len(raw) >= 2 {
			cipType := cip.DataType(binary.LittleEndian.Uint16(raw[0:2]))
			val.Type = cipTypeToPlcType(cipType)
			hdrLen := 2
			if cipType >= cip.TypeSTRUCT {
				hdrLen = 4
			}
			if len(raw) >= hdrLen {
				raw = raw[hdrLen:]
			}
		}
		val.Raw = raw

		values = append(values, val)
	}
	return values, nil
}

// cipTypeToPlcType maps CIP data type codes to protocol-agnostic plc.DataType.
func cipTypeToPlcType(dt cip.DataType) plc.DataType {
	switch dt {
	case cip.TypeBOOL:
		return plc.TypeBool
	case cip.TypeSINT:
		return plc.TypeInt16 // SINT is 8-bit but closest hint
	case cip.TypeINT:
		return plc.TypeInt16
	case cip.TypeDINT:
		return plc.TypeInt32
	case cip.TypeLINT:
		return plc.TypeInt64
	case cip.TypeUSINT:
		return plc.TypeUint16 // USINT is 8-bit but closest hint
	case cip.TypeUINT:
		return plc.TypeUint16
	case cip.TypeUDINT:
		return plc.TypeUint32
	case cip.TypeULINT:
		return plc.TypeUint64
	case cip.TypeREAL:
		return plc.TypeFloat32
	case cip.TypeLREAL:
		return plc.TypeFloat64
	case cip.TypeSTRING, cip.TypeSTRING2, cip.TypeSHORT_STRING:
		return plc.TypeString
	default:
		return plc.TypeBytes
	}
}

// Write writes raw bytes to a tag on the controller. The data slice must begin
// with a 2-byte CIP type code followed by the payload bytes.
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

// Close closes the underlying transport and releases all resources.
func (c *Client) Close() error {
	return c.transport.Close()
}

// ---------- Tag-level API ----------

// ReadTag reads a single element of a named tag from the PLC using the CIP
// Read Tag service (0x4C). The returned bytes include the 2-byte CIP type
// code prefix followed by the element data.
func (c *Client) ReadTag(ctx context.Context, tagName string) ([]byte, error) {
	return c.ReadTagElements(ctx, tagName, 1)
}

// ReadTagElements reads count elements of a named tag from the PLC using the
// CIP Read Tag service (0x4C). The returned bytes include the 2-byte CIP type
// code prefix followed by the element data. Count must be at least 1.
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

// WriteTag writes a Go value to a named tag on the PLC using the CIP Write
// Tag service (0x4D). The value must be a basic Go numeric type (bool, int32,
// float64, etc.) or implement [cip.Marshaler]. The CIP type code is inferred
// automatically from the Go type.
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

// ReadTagInto reads a single element of a named tag from the PLC and
// unmarshals it into dst. dst must be a pointer to a fixed-size type
// compatible with binary.Read or a type implementing [cip.Unmarshaler].
func (c *Client) ReadTagInto(ctx context.Context, tagName string, dst any) error {
	return c.ReadTagElementsInto(ctx, tagName, 1, dst)
}

// ReadTagElementsInto reads count elements of a named tag from the PLC and
// unmarshals them into dst. dst must be a pointer to a fixed-size type or
// slice compatible with binary.Read.
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

// ReadTimer reads a Rockwell Logix Timer tag (TON, TOF, or RTO) from the PLC
// and decodes it into a [cip.Timer] struct containing preset, accumulated, and
// status-bit fields.
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

// ListIdentity sends the EIP ListIdentity command via the current session and
// returns the device identity items reported by the target.
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

// ListServices sends the EIP ListServices command via the current session and
// returns the communication services supported by the target.
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

// ListTags enumerates all tags on the PLC by querying every instance of the
// CIP Symbol Object (class 0x6B). The entire enumeration runs inside a single
// retry scope so a mid-enumeration connection drop retries from scratch.
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

// Read is a generic helper that reads a single tag value from the PLC and
// returns it as type T. T must be a fixed-size type compatible with
// binary.Read (e.g. int32, float64) or implement [cip.Unmarshaler].
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

// ReadSlice is a generic helper that reads count elements of a tag and returns
// them as []T. T must be a fixed-size type compatible with binary.Read
// (e.g. int32, float64).
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
