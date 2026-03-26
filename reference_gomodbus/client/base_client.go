package client

import (
	"context"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/protocol"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// BaseClient provides common functionality for all Modbus clients.
// It uses a Transport for low-level communication.
type BaseClient struct {
	logger    common.LoggerInterface
	transport common.Transport
	protocol  common.Protocol
	unitID    common.UnitID
}

// Option is a function that configures a BaseClient
type Option func(*BaseClient)

// WithLogger sets the logger for the client
func WithLogger(logger common.LoggerInterface) Option {
	return func(c *BaseClient) {
		c.logger = logger

		// Propagate logger to transport and protocol if possible
		if c.transport != nil {
			c.transport = c.transport.WithLogger(logger)
		}
		if c.protocol != nil {
			c.protocol = c.protocol.WithLogger(logger)
		}
	}
}

// WithUnitID sets the unit ID for the client
func WithUnitID(unitID common.UnitID) Option {
	return func(c *BaseClient) {
		c.unitID = unitID
	}
}

// WithProtocol sets the protocol handler for the client
func WithProtocol(protocol common.Protocol) Option {
	return func(c *BaseClient) {
		c.protocol = protocol
	}
}

// NewBaseClient creates a new BaseClient.
func NewBaseClient(transport common.Transport, options ...Option) *BaseClient {
	client := &BaseClient{
		logger:    logging.NewLogger(),
		transport: transport,
		protocol:  protocol.NewProtocolHandler(),
		unitID:    0, // Default unit ID
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// WithLogger returns a new client with the given logger
func (c *BaseClient) WithLogger(logger common.LoggerInterface) common.Client {
	// Create a copy of the client with the new logger
	return NewBaseClient(
		c.transport,
		WithLogger(logger),
		WithUnitID(c.unitID),
		WithProtocol(c.protocol),
	)
}

// Connect establishes a connection to the Modbus server.
func (c *BaseClient) Connect(ctx context.Context) error {
	c.logger.Info(ctx, "Connecting to Modbus server with unit ID %d", c.unitID)
	return c.transport.Connect(ctx)
}

// Disconnect closes the connection to the Modbus server.
func (c *BaseClient) Disconnect(ctx context.Context) error {
	c.logger.Info(ctx, "Disconnecting from Modbus server")
	return c.transport.Disconnect(ctx)
}

// IsConnected returns true if the client is connected to the server.
func (c *BaseClient) IsConnected() bool {
	return c.transport.IsConnected()
}

// Send enqueues the request to the transport layer and awaits for the response.
func (c *BaseClient) Send(ctx context.Context, functionCode common.FunctionCode, data []byte) (common.Response, error) {
	if !c.IsConnected() {
		return nil, common.ErrNotConnected
	}

	// Create the request
	request := transport.NewRequest(c.unitID, functionCode, data)

	// Use the context or derive a new one with timeout
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		// Apply a default timeout if no deadline specified
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	c.logger.Debug(ctx, "Sending request: function=%s, data=%v", functionCode, data)

	// Send the request and get the response
	response, err := c.transport.Send(ctx, request)
	if err != nil {
		c.logger.Error(ctx, "Error sending request: %v", err)
		return nil, err
	}

	// Check for Modbus exception
	if response.IsException() {
		c.logger.Warn(ctx, "Received exception response: function=%s, exception=%d",
			response.GetPDU().FunctionCode, response.GetException())
		return nil, response.ToError()
	}

	c.logger.Debug(ctx, "Received successful response: function=%s", response.GetPDU().FunctionCode)
	return response, nil
}

// ReadCoils reads coils from the server.
func (c *BaseClient) ReadCoils(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.CoilValue, error) {
	c.logger.Debug(ctx, "Reading %d coils from address %d", quantity, address)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadCoilsRequest(address, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error generating read coils request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadCoils, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	values, err := c.protocol.ParseReadCoilsResponse(response.GetPDU().Data, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read coils response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read %d coils successfully", len(values))
	return values, nil
}

// ReadDiscreteInputs reads discrete inputs from the server.
func (c *BaseClient) ReadDiscreteInputs(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.DiscreteInputValue, error) {
	c.logger.Debug(ctx, "Reading %d discrete inputs from address %d", quantity, address)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadDiscreteInputsRequest(address, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error generating read discrete inputs request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadDiscreteInputs, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	values, err := c.protocol.ParseReadDiscreteInputsResponse(response.GetPDU().Data, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read discrete inputs response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read %d discrete inputs successfully", len(values))
	return values, nil
}

// ReadHoldingRegisters reads holding registers from the server.
func (c *BaseClient) ReadHoldingRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.RegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d holding registers from address %d", quantity, address)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadHoldingRegistersRequest(address, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error generating read holding registers request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadHoldingRegisters, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	values, err := c.protocol.ParseReadHoldingRegistersResponse(response.GetPDU().Data, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read holding registers response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read %d holding registers successfully", len(values))
	return values, nil
}

// ReadInputRegisters reads input registers from the server.
func (c *BaseClient) ReadInputRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.InputRegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d input registers from address %d", quantity, address)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadInputRegistersRequest(address, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error generating read input registers request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadInputRegisters, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	values, err := c.protocol.ParseReadInputRegistersResponse(response.GetPDU().Data, quantity)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read input registers response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read %d input registers successfully", len(values))
	return values, nil
}

// WriteSingleCoil writes a single coil to the server.
func (c *BaseClient) WriteSingleCoil(ctx context.Context, address common.Address, value common.CoilValue) error {
	c.logger.Info(ctx, "Writing coil at address %d with value %t", address, value)

	// Generate the request data
	requestData, err := c.protocol.GenerateWriteSingleCoilRequest(address, value)
	if err != nil {
		c.logger.Error(ctx, "Error generating write single coil request: %v", err)
		return err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncWriteSingleCoil, requestData)
	if err != nil {
		return err
	}

	// Parse the response
	_, _, err = c.protocol.ParseWriteSingleCoilResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing write single coil response: %v", err)
		return err
	}

	c.logger.Debug(ctx, "Wrote coil %d=%v successfully", address, value)
	return nil
}

// WriteSingleRegister writes a single register to the server.
func (c *BaseClient) WriteSingleRegister(ctx context.Context, address common.Address, value common.RegisterValue) error {
	c.logger.Info(ctx, "Writing register at address %d with value %d", address, value)

	// Generate the request data
	requestData, err := c.protocol.GenerateWriteSingleRegisterRequest(address, value)
	if err != nil {
		c.logger.Error(ctx, "Error generating write single register request: %v", err)
		return err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncWriteSingleRegister, requestData)
	if err != nil {
		return err
	}

	// Parse the response
	_, _, err = c.protocol.ParseWriteSingleRegisterResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing write single register response: %v", err)
		return err
	}

	c.logger.Debug(ctx, "Wrote register %d=%d successfully", address, value)
	return nil
}

// WriteMultipleCoils writes multiple coils to the server.
func (c *BaseClient) WriteMultipleCoils(ctx context.Context, address common.Address, values []common.CoilValue) error {
	c.logger.Info(ctx, "Writing %d coils starting at address %d", len(values), address)

	// Generate the request data
	requestData, err := c.protocol.GenerateWriteMultipleCoilsRequest(address, values)
	if err != nil {
		c.logger.Error(ctx, "Error generating write multiple coils request: %v", err)
		return err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncWriteMultipleCoils, requestData)
	if err != nil {
		return err
	}

	// Parse the response
	_, _, err = c.protocol.ParseWriteMultipleCoilsResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing write multiple coils response: %v", err)
		return err
	}

	c.logger.Debug(ctx, "Wrote %d coils successfully", len(values))
	return nil
}

// WriteMultipleRegisters writes multiple registers to the server.
func (c *BaseClient) WriteMultipleRegisters(ctx context.Context, address common.Address, values []common.RegisterValue) error {
	c.logger.Info(ctx, "Writing %d registers starting at address %d", len(values), address)

	// Generate the request data
	requestData, err := c.protocol.GenerateWriteMultipleRegistersRequest(address, values)
	if err != nil {
		c.logger.Error(ctx, "Error generating write multiple registers request: %v", err)
		return err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncWriteMultipleRegisters, requestData)
	if err != nil {
		return err
	}

	// Parse the response
	_, _, err = c.protocol.ParseWriteMultipleRegistersResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing write multiple registers response: %v", err)
		return err
	}

	c.logger.Debug(ctx, "Wrote %d registers successfully", len(values))
	return nil
}

// ReadWriteMultipleRegisters reads and writes multiple registers to the server.
func (c *BaseClient) ReadWriteMultipleRegisters(ctx context.Context, readAddress common.Address, readQuantity common.Quantity, writeAddress common.Address, writeValues []common.RegisterValue) ([]common.RegisterValue, error) {
	c.logger.Debug(ctx, "Reading %d registers from %d and writing %d registers to %d",
		readQuantity, readAddress, len(writeValues), writeAddress)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadWriteMultipleRegistersRequest(readAddress, readQuantity, writeAddress, writeValues)
	if err != nil {
		c.logger.Error(ctx, "Error generating read/write multiple registers request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadWriteMultipleRegisters, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	values, err := c.protocol.ParseReadWriteMultipleRegistersResponse(response.GetPDU().Data, readQuantity)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read/write multiple registers response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read/write operation completed successfully, read %d registers", len(values))
	return values, nil
}

// ReadExceptionStatus reads the exception status from the server.
func (c *BaseClient) ReadExceptionStatus(ctx context.Context) (common.ExceptionStatus, error) {
	c.logger.Info(ctx, "Reading exception status")

	// Generate the request data
	requestData, err := c.protocol.GenerateReadExceptionStatusRequest()
	if err != nil {
		c.logger.Error(ctx, "Error generating read exception status request: %v", err)
		return common.ExceptionStatus(0), err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadExceptionStatus, requestData)
	if err != nil {
		return common.ExceptionStatus(0), err
	}

	// Parse the response
	status, err := c.protocol.ParseReadExceptionStatusResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read exception status response: %v", err)
		return common.ExceptionStatus(0), err
	}

	c.logger.Debug(ctx, "Read exception status successfully: %s", status)
	return status, nil
}

// ReadDeviceIdentification reads device identification data from the server.
// The readDeviceIDCode specifies which identification data to read:
//   - ReadDeviceIDBasic: Basic device identification (stream access)
//   - ReadDeviceIDRegular: Regular device identification (stream access)
//   - ReadDeviceIDExtended: Extended device identification (stream access)
//   - ReadDeviceIDSpecific: Specific identification object
// When using ReadDeviceIDSpecific, the objectID specifies which object to read.
// For other read device ID codes, objectID should be DeviceIDObjectCode(0).
func (c *BaseClient) ReadDeviceIdentification(ctx context.Context, readDeviceIDCode common.ReadDeviceIDCode, objectID common.DeviceIDObjectCode) (*common.DeviceIdentification, error) {
	c.logger.Debug(ctx, "Reading device identification: code=%d, objectID=%d", readDeviceIDCode, objectID)

	// Generate the request data
	requestData, err := c.protocol.GenerateReadDeviceIdentificationRequest(readDeviceIDCode, objectID)
	if err != nil {
		c.logger.Error(ctx, "Error generating read device identification request: %v", err)
		return nil, err
	}

	// Send the request
	response, err := c.Send(ctx, common.FuncReadDeviceIdentification, requestData)
	if err != nil {
		return nil, err
	}

	// Parse the response
	deviceID, err := c.protocol.ParseReadDeviceIdentificationResponse(response.GetPDU().Data)
	if err != nil {
		c.logger.Error(ctx, "Error parsing read device identification response: %v", err)
		return nil, err
	}

	c.logger.Debug(ctx, "Read device identification successfully: %d objects", len(deviceID.Objects))
	return deviceID, nil
}
