package common

import "context"

// Client is the interface that all Modbus clients must implement.
type Client interface {
	// Connect establishes a connection to the Modbus server.
	Connect(ctx context.Context) error

	// Disconnect closes the connection to the Modbus server.
	Disconnect(ctx context.Context) error

	// IsConnected returns true if the client is connected to the server.
	IsConnected() bool

	// ReadCoils reads coils from the server.
	// The address is the starting address of the coils to read.
	// The quantity is the number of coils to read.
	ReadCoils(ctx context.Context, address Address, quantity Quantity) ([]CoilValue, error)

	// ReadDiscreteInputs reads discrete inputs from the server.
	// The address is the starting address of the discrete inputs to read.
	// The quantity is the number of discrete inputs to read.
	ReadDiscreteInputs(ctx context.Context, address Address, quantity Quantity) ([]DiscreteInputValue, error)

	// ReadHoldingRegisters reads holding registers from the server.
	// The address is the starting address of the registers to read.
	// The quantity is the number of registers to read.
	ReadHoldingRegisters(ctx context.Context, address Address, quantity Quantity) ([]RegisterValue, error)

	// ReadInputRegisters reads input registers from the server.
	// The address is the starting address of the registers to read.
	// The quantity is the number of registers to read.
	ReadInputRegisters(ctx context.Context, address Address, quantity Quantity) ([]InputRegisterValue, error)

	// WriteSingleCoil writes a single coil to the server.
	// The address is the address of the coil to write.
	// The value is the value to write.
	WriteSingleCoil(ctx context.Context, address Address, value CoilValue) error

	// WriteSingleRegister writes a single register to the server.
	// The address is the address of the register to write.
	// The value is the value to write.
	WriteSingleRegister(ctx context.Context, address Address, value RegisterValue) error

	// WriteMultipleCoils writes multiple coils to the server.
	// The address is the starting address of the coils to write.
	// The values are the values to write.
	WriteMultipleCoils(ctx context.Context, address Address, values []CoilValue) error

	// WriteMultipleRegisters writes multiple registers to the server.
	// The address is the starting address of the registers to write.
	// The values are the values to write.
	WriteMultipleRegisters(ctx context.Context, address Address, values []RegisterValue) error

	// ReadWriteMultipleRegisters reads and writes multiple registers to the server.
	// The readAddress is the starting address of the registers to read.
	// The readQuantity is the number of registers to read.
	// The writeAddress is the starting address of the registers to write.
	// The writeValues are the values to write.
	ReadWriteMultipleRegisters(ctx context.Context, readAddress Address, readQuantity Quantity, writeAddress Address, writeValues []RegisterValue) ([]RegisterValue, error)

	// ReadExceptionStatus reads the exception status from the server.
	// Returns the exception status as a typed value.
	ReadExceptionStatus(ctx context.Context) (ExceptionStatus, error)

	// ReadDeviceIdentification reads device identification data from the server.
	// The readDeviceIDCode specifies which identification data to read:
	//   - ReadDeviceIDBasic: Basic device identification (stream access)
	//   - ReadDeviceIDRegular: Regular device identification (stream access)
	//   - ReadDeviceIDExtended: Extended device identification (stream access)
	//   - ReadDeviceIDSpecific: Specific identification object
	// When using ReadDeviceIDSpecific, the objectID specifies which object to read.
	// For other read device ID codes, objectID should be DeviceIDObjectCode(0).
	ReadDeviceIdentification(ctx context.Context, readDeviceIDCode ReadDeviceIDCode, objectID DeviceIDObjectCode) (*DeviceIdentification, error)

	// WithLogger sets the logger for the client.
	WithLogger(logger LoggerInterface) Client
}

// Protocol defines the interface for a Modbus protocol handler.
type Protocol interface {
	// GenerateReadCoilsRequest generates a request PDU data to read coils.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadCoilsRequest(address Address, quantity Quantity) ([]byte, error)

	// ParseReadCoilsResponse parses a response PDU data from a read coils request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the coil values as a slice of booleans.
	ParseReadCoilsResponse(data []byte, quantity Quantity) ([]CoilValue, error)

	// GenerateReadDiscreteInputsRequest generates a request PDU data to read discrete inputs.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadDiscreteInputsRequest(address Address, quantity Quantity) ([]byte, error)

	// ParseReadDiscreteInputsResponse parses a response PDU data from a read discrete inputs request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the discrete input values as a slice of booleans.
	ParseReadDiscreteInputsResponse(data []byte, quantity Quantity) ([]DiscreteInputValue, error)

	// GenerateReadHoldingRegistersRequest generates a request PDU data to read holding registers.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadHoldingRegistersRequest(address Address, quantity Quantity) ([]byte, error)

	// ParseReadHoldingRegistersResponse parses a response PDU data from a read holding registers request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the register values as a slice of uint16.
	ParseReadHoldingRegistersResponse(data []byte, quantity Quantity) ([]RegisterValue, error)

	// GenerateReadInputRegistersRequest generates a request PDU data to read input registers.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadInputRegistersRequest(address Address, quantity Quantity) ([]byte, error)

	// ParseReadInputRegistersResponse parses a response PDU data from a read input registers request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the register values as a slice of uint16.
	ParseReadInputRegistersResponse(data []byte, quantity Quantity) ([]InputRegisterValue, error)

	// GenerateWriteSingleCoilRequest generates a request PDU data to write a single coil.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateWriteSingleCoilRequest(address Address, value CoilValue) ([]byte, error)

	// ParseWriteSingleCoilResponse parses a response PDU data from a write single coil request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the coil address, value, and any error.
	ParseWriteSingleCoilResponse(data []byte) (Address, CoilValue, error)

	// GenerateWriteSingleRegisterRequest generates a request PDU data to write a single register.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateWriteSingleRegisterRequest(address Address, value RegisterValue) ([]byte, error)

	// ParseWriteSingleRegisterResponse parses a response PDU data from a write single register request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the register address, value, and any error.
	ParseWriteSingleRegisterResponse(data []byte) (Address, RegisterValue, error)

	// GenerateWriteMultipleCoilsRequest generates a request PDU data to write multiple coils.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateWriteMultipleCoilsRequest(address Address, values []CoilValue) ([]byte, error)

	// ParseWriteMultipleCoilsResponse parses a response PDU data from a write multiple coils request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the starting address, quantity written, and any error.
	ParseWriteMultipleCoilsResponse(data []byte) (Address, Quantity, error)

	// GenerateWriteMultipleRegistersRequest generates a request PDU data to write multiple registers.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateWriteMultipleRegistersRequest(address Address, values []RegisterValue) ([]byte, error)

	// ParseWriteMultipleRegistersResponse parses a response PDU data from a write multiple registers request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the starting address, quantity written, and any error.
	ParseWriteMultipleRegistersResponse(data []byte) (Address, Quantity, error)

	// GenerateReadWriteMultipleRegistersRequest generates a request PDU data to read and write multiple registers.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadWriteMultipleRegistersRequest(readAddress Address, readQuantity Quantity, writeAddress Address, writeValues []RegisterValue) ([]byte, error)

	// ParseReadWriteMultipleRegistersResponse parses a response PDU data from a read/write multiple registers request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the read register values as a slice of uint16.
	ParseReadWriteMultipleRegistersResponse(data []byte, readQuantity Quantity) ([]RegisterValue, error)

	// GenerateReadExceptionStatusRequest generates a request PDU data to read the exception status.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadExceptionStatusRequest() ([]byte, error)

	// ParseReadExceptionStatusResponse parses a response PDU data from a read exception status request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the exception status as a typed value.
	ParseReadExceptionStatusResponse(data []byte) (ExceptionStatus, error)

	// GenerateReadDeviceIdentificationRequest generates a request PDU data to read device identification.
	// The returned byte slice contains only the PDU data (excluding function code).
	// This is used to construct the full Modbus request.
	GenerateReadDeviceIdentificationRequest(readDeviceIDCode ReadDeviceIDCode, objectID DeviceIDObjectCode) ([]byte, error)

	// ParseReadDeviceIdentificationResponse parses a response PDU data from a read device identification request.
	// The data parameter contains the PDU data (excluding function code).
	// Returns the device identification data.
	ParseReadDeviceIdentificationResponse(data []byte) (*DeviceIdentification, error)

	// WithLogger sets the logger for the protocol and returns a new Protocol instance.
	WithLogger(logger LoggerInterface) Protocol
}