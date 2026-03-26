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

// Client is a Modbus TCP client that wraps a transport.Transport[*TCPConn]
// and exposes both Modbus-specific methods and the plc.PLC interface.
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

// Connect is a convenience constructor that creates a ReconnectingTransport,
// obtains an initial connection, and returns a ready-to-use Client.
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

	tp := transport.NewReconnectingTransport[*TCPConn](connector, closer, transportOpts...)

	// Force an initial connection to verify reachability.
	if _, err := tp.Conn(ctx); err != nil {
		_ = tp.Close()
		return nil, fmt.Errorf("modbus connect: %w", err)
	}

	return NewClient(tp, clientOpts...), nil
}

// NewClient creates a Client from an existing transport.
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

// IsConnected satisfies plc.PLC.
func (c *Client) IsConnected() bool {
	conn, err := c.transport.Conn(context.Background())
	if err != nil {
		return false
	}
	return conn.IsConnected()
}

// Read dispatches to the appropriate Modbus read function based on the
// DataPoint type and returns the raw bytes.
func (c *Client) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	results := make([]plc.Value, 0, len(points))

	for _, dp := range points {
		raw, err := c.readDataPoint(ctx, dp)
		if err != nil {
			return nil, err
		}
		results = append(results, plc.Value{
			DataPoint: dp,
			Raw:       raw,
		})
	}

	return results, nil
}

// Write dispatches to the appropriate Modbus write function based on the
// DataPoint type.
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

func (c *Client) readDataPoint(ctx context.Context, dp plc.DataPoint) ([]byte, error) {
	switch p := dp.(type) {
	case HoldingRegister:
		regs, err := c.ReadHoldingRegisters(ctx, p.Addr, p.Qty)
		if err != nil {
			return nil, err
		}
		return registersToBytes(regs), nil

	case InputRegister:
		regs, err := c.ReadInputRegisters(ctx, p.Addr, p.Qty)
		if err != nil {
			return nil, err
		}
		return registersToBytes(regs), nil

	case Coil:
		vals, err := c.ReadCoils(ctx, p.Addr, p.Qty)
		if err != nil {
			return nil, err
		}
		return boolsToBytes(vals), nil

	case DiscreteInput:
		vals, err := c.ReadDiscreteInputs(ctx, p.Addr, p.Qty)
		if err != nil {
			return nil, err
		}
		return boolsToBytes(vals), nil

	default:
		return nil, fmt.Errorf("modbus read: unsupported data point type %T", dp)
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

// ReadCoils reads coils from the server.
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

// ReadDiscreteInputs reads discrete inputs from the server.
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

// ReadHoldingRegisters reads holding registers from the server.
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

// ReadInputRegisters reads input registers from the server.
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

// WriteSingleCoil writes a single coil to the server.
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

// WriteSingleRegister writes a single register to the server.
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

// WriteMultipleCoils writes multiple coils to the server.
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

// WriteMultipleRegisters writes multiple registers to the server.
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

// ReadWriteMultipleRegisters reads and writes multiple registers in a single transaction.
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

// ReadExceptionStatus reads the exception status from the server.
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

// ReadDeviceIdentification reads device identification data from the server.
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
