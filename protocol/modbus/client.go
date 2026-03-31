package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/transport"
)

// Client is a Modbus TCP client that communicates with a remote device over
// a managed TCP connection. It exposes Modbus-specific read/write methods
// (e.g. [Client.ReadHoldingRegisters]) as well as the generic
// [github.com/iceisfun/goindustrial/plc.PLC] interface for protocol-agnostic use.
// Transport-level reconnection and retry logic are handled automatically.
type Client struct {
	logger    logging.Logger
	transport transport.Transport[*TCPConn]
	protocol  *ProtocolHandler
	unitID    UnitID
	retries   int
	retryDelay time.Duration
}

// Compile-time assertion that Client implements plc.PLC.
var _ plc.PLC = (*Client)(nil)

// Connect dials a Modbus TCP server at host (e.g. "192.168.1.10") and returns
// a ready-to-use Client. It creates a reconnecting transport, verifies
// reachability with an initial connection, and applies any provided options.
// Options may be [TCPConnOption], [ClientOption], or [transport.Option] values
// and are routed to the appropriate layer automatically.
func Connect(ctx context.Context, host string, opts ...any) (*Client, error) {
	var connOpts []TCPConnOption
	var clientOpts []ClientOption
	var transportOpts []transport.Option

	for _, o := range opts {
		switch v := o.(type) {
		case TCPConnOption:
			connOpts = append(connOpts, v)
		case ClientOption:
			clientOpts = append(clientOpts, v)
		case transport.Option:
			transportOpts = append(transportOpts, v)
		}
	}

	connector := NewTCPConnector(host, connOpts...)
	closer := NewTCPCloser()

	tp, err := transport.DialReconnectingTransport(ctx, connector, closer, transportOpts...)
	if err != nil {
		return nil, fmt.Errorf("modbus connect: %w", err)
	}

	return NewClient(tp, clientOpts...), nil
}

// NewClient creates a Client from an existing transport. Use this when you
// need full control over the transport layer; for most cases [Connect] is
// simpler.
func NewClient(tp transport.Transport[*TCPConn], opts ...ClientOption) *Client {
	c := &Client{
		logger:     logging.NewNopLogger(),
		transport:  tp,
		protocol:   NewProtocolHandler(),
		unitID:     0,
		retries:    0,
		retryDelay: 500 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Close permanently shuts down the underlying transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

// ---------------------------------------------------------------------------
// plc.PLC interface
// ---------------------------------------------------------------------------

// Connect satisfies plc.PLC. For a Client backed by a ReconnectingTransport
// this is effectively a no-op because Conn() connects lazily.
func (c *Client) Connect(ctx context.Context) error {
	_, err := c.transport.Conn(ctx)
	return err
}

// Disconnect satisfies plc.PLC.
func (c *Client) Disconnect(ctx context.Context) error {
	return c.transport.Close()
}

// IsConnected satisfies plc.PLC. It does not attempt to establish a new
// connection — it only checks existing state.
func (c *Client) IsConnected() bool {
	if p, ok := c.transport.(transport.Peeker); ok {
		return p.Peek()
	}
	// Fallback for custom transports that don't implement Peeker.
	conn, err := c.transport.Conn(context.Background())
	if err != nil {
		return false
	}
	return conn.IsConnected()
}

// Read satisfies [plc.PLC]. It dispatches each DataPoint to the appropriate
// Modbus read function based on its concrete type ([HoldingRegister],
// [InputRegister], [Coil], or [DiscreteInput]) and returns the values.
func (c *Client) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	results := make([]plc.Value, 0, len(points))

	for _, dp := range points {
		val, err := c.readDataPoint(ctx, dp)
		if err != nil {
			return nil, err
		}
		val.DataPoint = dp
		results = append(results, val)
	}

	return results, nil
}

// Write satisfies [plc.PLC]. It dispatches the write to the appropriate Modbus
// write function based on the DataPoint's concrete type ([HoldingRegister] or
// [Coil]). Input registers and discrete inputs are read-only and cannot be
// written.
func (c *Client) Write(ctx context.Context, point plc.DataPoint, data []byte) error {
	switch dp := point.(type) {
	case HoldingRegister:
		if dp.Qty == 1 && len(data) == 2 {
			value := RegisterValue(binary.BigEndian.Uint16(data))
			return c.WriteSingleRegister(ctx, dp.Addr, value)
		}
		// Multiple registers: convert raw bytes to []RegisterValue.
		if len(data)%2 != 0 {
			return fmt.Errorf("modbus write: data length %d is not a multiple of 2", len(data))
		}
		values := make([]RegisterValue, len(data)/2)
		for i := range values {
			values[i] = binary.BigEndian.Uint16(data[i*2 : i*2+2])
		}
		return c.WriteMultipleRegisters(ctx, dp.Addr, values)

	case Coil:
		if dp.Qty == 1 && len(data) >= 1 {
			return c.WriteSingleCoil(ctx, dp.Addr, data[0] != 0)
		}
		// Multiple coils.
		values := make([]CoilValue, dp.Qty)
		for i := range values {
			byteIdx := i / 8
			bitIdx := i % 8
			if byteIdx < len(data) {
				values[i] = (data[byteIdx]>>uint(bitIdx))&1 == 1
			}
		}
		return c.WriteMultipleCoils(ctx, dp.Addr, values)

	default:
		return fmt.Errorf("modbus write: unsupported data point type %T", point)
	}
}

// ---------------------------------------------------------------------------
// readDataPoint
// ---------------------------------------------------------------------------

func (c *Client) readDataPoint(ctx context.Context, dp plc.DataPoint) (plc.Value, error) {
	switch p := dp.(type) {
	case HoldingRegister:
		regs, err := c.ReadHoldingRegisters(ctx, p.Addr, p.Qty)
		if err != nil {
			return plc.Value{}, err
		}
		dt := plc.TypeUint16
		if p.Qty > 1 {
			dt = plc.TypeBytes
		}
		return plc.Value{
			Raw:       registersToBytes(regs),
			Type:      dt,
			ByteOrder: plc.ByteOrderBigEndian,
		}, nil

	case InputRegister:
		regs, err := c.ReadInputRegisters(ctx, p.Addr, p.Qty)
		if err != nil {
			return plc.Value{}, err
		}
		dt := plc.TypeUint16
		if p.Qty > 1 {
			dt = plc.TypeBytes
		}
		return plc.Value{
			Raw:       registersToBytes(regs),
			Type:      dt,
			ByteOrder: plc.ByteOrderBigEndian,
		}, nil

	case Coil:
		vals, err := c.ReadCoils(ctx, p.Addr, p.Qty)
		if err != nil {
			return plc.Value{}, err
		}
		return plc.Value{
			Raw:       boolsToBytes(vals),
			Type:      plc.TypeBool,
			ByteOrder: plc.ByteOrderBigEndian,
		}, nil

	case DiscreteInput:
		vals, err := c.ReadDiscreteInputs(ctx, p.Addr, p.Qty)
		if err != nil {
			return plc.Value{}, err
		}
		return plc.Value{
			Raw:       boolsToBytes(vals),
			Type:      plc.TypeBool,
			ByteOrder: plc.ByteOrderBigEndian,
		}, nil

	default:
		return plc.Value{}, fmt.Errorf("modbus read: unsupported data point type %T", dp)
	}
}

func registersToBytes(regs []RegisterValue) []byte {
	buf := make([]byte, len(regs)*2)
	for i, v := range regs {
		binary.BigEndian.PutUint16(buf[i*2:], v)
	}
	return buf
}

func boolsToBytes(vals []bool) []byte {
	n := (len(vals) + 7) / 8
	buf := make([]byte, n)
	for i, v := range vals {
		if v {
			buf[i/8] |= 1 << uint(i%8)
		}
	}
	return buf
}

// ---------------------------------------------------------------------------
// send is the core helper with retry logic
// ---------------------------------------------------------------------------

func (c *Client) send(ctx context.Context, functionCode FunctionCode, data []byte) (*Response, error) {
	request := NewRequest(c.unitID, functionCode, data)

	var lastErr error
	attempts := 1 + c.retries

	for i := 0; i < attempts; i++ {
		conn, err := c.transport.Conn(ctx)
		if err != nil {
			lastErr = err
			if i < attempts-1 {
				c.logger.Warn(ctx, "Transport error (attempt %d/%d): %v", i+1, attempts, err)
				time.Sleep(c.retryDelay)
			}
			continue
		}

		// Apply a default timeout if none set.
		sendCtx := ctx
		var cancel context.CancelFunc
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			sendCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		}

		response, err := conn.Send(sendCtx, request)
		if cancel != nil {
			cancel()
		}

		if err != nil {
			// If it's a ModbusError (protocol-level), do not retry.
			if IsModbusError(err) {
				return nil, err
			}

			lastErr = err
			c.logger.Warn(ctx, "Send error (attempt %d/%d): %v", i+1, attempts, err)

			// Transport-level error: reset and retry.
			_ = c.transport.Reset(conn)
			if i < attempts-1 {
				time.Sleep(c.retryDelay)
			}
			continue
		}

		// Check for Modbus exception in the response.
		if response.IsException() {
			return nil, response.ToError()
		}

		return response, nil
	}

	return nil, fmt.Errorf("modbus: all %d attempts failed: %w", attempts, lastErr)
}

// ---------------------------------------------------------------------------
// Modbus-specific methods
// ---------------------------------------------------------------------------

// ReadCoils reads one or more coil values starting at address (function code 0x01).
// Coils are single-bit read/write outputs. The quantity must be between 1 and 2000.
func (c *Client) ReadCoils(ctx context.Context, address Address, quantity Quantity) ([]CoilValue, error) {
	c.logger.Debug(ctx, "Reading %d coils from address %d", quantity, address)

	requestData, err := c.protocol.GenerateReadCoilsRequest(address, quantity)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadCoils, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadCoilsResponse(response.GetPDU().Data, quantity)
}

// ReadDiscreteInputs reads one or more discrete input values starting at address
// (function code 0x02). Discrete inputs are single-bit read-only values.
// The quantity must be between 1 and 2000.
func (c *Client) ReadDiscreteInputs(ctx context.Context, address Address, quantity Quantity) ([]DiscreteInputValue, error) {
	c.logger.Debug(ctx, "Reading %d discrete inputs from address %d", quantity, address)

	requestData, err := c.protocol.GenerateReadDiscreteInputsRequest(address, quantity)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadDiscreteInputs, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadDiscreteInputsResponse(response.GetPDU().Data, quantity)
}

// ReadHoldingRegisters reads one or more 16-bit holding register values starting
// at address (function code 0x03). Holding registers are the primary read/write
// data storage in a Modbus device. The quantity must be between 1 and 125.
func (c *Client) ReadHoldingRegisters(ctx context.Context, address Address, quantity Quantity) ([]RegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d holding registers from address %d", quantity, address)

	requestData, err := c.protocol.GenerateReadHoldingRegistersRequest(address, quantity)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadHoldingRegisters, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadHoldingRegistersResponse(response.GetPDU().Data, quantity)
}

// ReadInputRegisters reads one or more 16-bit input register values starting at
// address (function code 0x04). Input registers are read-only data typically
// sourced from sensors or process values. The quantity must be between 1 and 125.
func (c *Client) ReadInputRegisters(ctx context.Context, address Address, quantity Quantity) ([]InputRegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d input registers from address %d", quantity, address)

	requestData, err := c.protocol.GenerateReadInputRegistersRequest(address, quantity)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadInputRegisters, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadInputRegistersResponse(response.GetPDU().Data, quantity)
}

// WriteSingleCoil writes a single coil output at address (function code 0x05).
// A coil is a single-bit value; true turns it ON, false turns it OFF.
func (c *Client) WriteSingleCoil(ctx context.Context, address Address, value CoilValue) error {
	c.logger.Info(ctx, "Writing coil at address %d with value %t", address, value)

	requestData, err := c.protocol.GenerateWriteSingleCoilRequest(address, value)
	if err != nil {
		return err
	}

	response, err := c.send(ctx, FuncWriteSingleCoil, requestData)
	if err != nil {
		return err
	}

	_, _, err = c.protocol.ParseWriteSingleCoilResponse(response.GetPDU().Data)
	return err
}

// WriteSingleRegister writes a single 16-bit holding register at address
// (function code 0x06).
func (c *Client) WriteSingleRegister(ctx context.Context, address Address, value RegisterValue) error {
	c.logger.Info(ctx, "Writing register at address %d with value %d", address, value)

	requestData, err := c.protocol.GenerateWriteSingleRegisterRequest(address, value)
	if err != nil {
		return err
	}

	response, err := c.send(ctx, FuncWriteSingleRegister, requestData)
	if err != nil {
		return err
	}

	_, _, err = c.protocol.ParseWriteSingleRegisterResponse(response.GetPDU().Data)
	return err
}

// WriteMultipleCoils writes a contiguous block of coil outputs starting at
// address (function code 0x0F). Up to 1968 coils may be written per request.
func (c *Client) WriteMultipleCoils(ctx context.Context, address Address, values []CoilValue) error {
	c.logger.Info(ctx, "Writing %d coils starting at address %d", len(values), address)

	requestData, err := c.protocol.GenerateWriteMultipleCoilsRequest(address, values)
	if err != nil {
		return err
	}

	response, err := c.send(ctx, FuncWriteMultipleCoils, requestData)
	if err != nil {
		return err
	}

	_, _, err = c.protocol.ParseWriteMultipleCoilsResponse(response.GetPDU().Data)
	return err
}

// WriteMultipleRegisters writes a contiguous block of 16-bit holding registers
// starting at address (function code 0x10). Up to 123 registers may be written
// per request.
func (c *Client) WriteMultipleRegisters(ctx context.Context, address Address, values []RegisterValue) error {
	c.logger.Info(ctx, "Writing %d registers starting at address %d", len(values), address)

	requestData, err := c.protocol.GenerateWriteMultipleRegistersRequest(address, values)
	if err != nil {
		return err
	}

	response, err := c.send(ctx, FuncWriteMultipleRegisters, requestData)
	if err != nil {
		return err
	}

	_, _, err = c.protocol.ParseWriteMultipleRegistersResponse(response.GetPDU().Data)
	return err
}

// ReadWriteMultipleRegisters atomically writes holding registers and reads
// holding registers in a single Modbus transaction (function code 0x17).
// The write is performed before the read on the server side.
func (c *Client) ReadWriteMultipleRegisters(ctx context.Context, readAddress Address, readQuantity Quantity, writeAddress Address, writeValues []RegisterValue) ([]RegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d registers from %d and writing %d registers to %d",
		readQuantity, readAddress, len(writeValues), writeAddress)

	requestData, err := c.protocol.GenerateReadWriteMultipleRegistersRequest(readAddress, readQuantity, writeAddress, writeValues)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadWriteMultipleRegisters, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadWriteMultipleRegistersResponse(response.GetPDU().Data, readQuantity)
}

// ReadExceptionStatus reads the eight exception status coils from the server
// (function code 0x07). The returned ExceptionStatus is a bitmask of eight
// device-specific status bits.
func (c *Client) ReadExceptionStatus(ctx context.Context) (ExceptionStatus, error) {
	c.logger.Info(ctx, "Reading exception status")

	requestData, err := c.protocol.GenerateReadExceptionStatusRequest()
	if err != nil {
		return 0, err
	}

	response, err := c.send(ctx, FuncReadExceptionStatus, requestData)
	if err != nil {
		return 0, err
	}

	return c.protocol.ParseReadExceptionStatusResponse(response.GetPDU().Data)
}

// ReadDeviceIdentification reads device identification objects such as vendor
// name, product code, and revision from the server (function code 0x2B / MEI
// type 0x0E). The readDeviceIDCode selects which category of objects to
// retrieve, and objectID specifies the starting object.
func (c *Client) ReadDeviceIdentification(ctx context.Context, readDeviceIDCode ReadDeviceIDCode, objectID DeviceIDObjectCode) (*DeviceIdentification, error) {
	c.logger.Debug(ctx, "Reading device identification: code=%d, objectID=%d", readDeviceIDCode, objectID)

	requestData, err := c.protocol.GenerateReadDeviceIdentificationRequest(readDeviceIDCode, objectID)
	if err != nil {
		return nil, err
	}

	response, err := c.send(ctx, FuncReadDeviceIdentification, requestData)
	if err != nil {
		return nil, err
	}

	return c.protocol.ParseReadDeviceIdentificationResponse(response.GetPDU().Data)
}
