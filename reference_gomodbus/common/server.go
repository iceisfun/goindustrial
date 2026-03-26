package common

import (
	"context"
)

// HandlerFunc is a function that processes a Modbus request and returns a response.
type HandlerFunc func(ctx context.Context, request Request) (Response, error)

// DefaultHandlerFunc returns a "not implemented" error for any request.
func DefaultHandlerFunc(ctx context.Context, request Request) (Response, error) {
	return nil, NewModbusError(
		request.GetPDU().FunctionCode,
		ExceptionFunctionCodeNotSupported,
	)
}

// DataStore represents a Modbus data store with read/write capabilities
type DataStore interface {
	// ReadCoils reads coil values from the data store
	ReadCoils(ctx context.Context, address Address, quantity Quantity) ([]CoilValue, error)

	// ReadDiscreteInputs reads discrete input values from the data store
	ReadDiscreteInputs(ctx context.Context, address Address, quantity Quantity) ([]DiscreteInputValue, error)

	// ReadHoldingRegisters reads holding register values from the data store
	ReadHoldingRegisters(ctx context.Context, address Address, quantity Quantity) ([]RegisterValue, error)

	// ReadInputRegisters reads input register values from the data store
	ReadInputRegisters(ctx context.Context, address Address, quantity Quantity) ([]InputRegisterValue, error)

	// WriteSingleCoil writes a single coil value to the data store
	WriteSingleCoil(ctx context.Context, address Address, value CoilValue) error

	// WriteSingleRegister writes a single register value to the data store
	WriteSingleRegister(ctx context.Context, address Address, value RegisterValue) error

	// WriteMultipleCoils writes multiple coil values to the data store
	WriteMultipleCoils(ctx context.Context, address Address, values []CoilValue) error

	// WriteMultipleRegisters writes multiple register values to the data store
	WriteMultipleRegisters(ctx context.Context, address Address, values []RegisterValue) error
}

// Server is the interface that all Modbus servers must implement
type Server interface {
	// Start starts the server
	Start(ctx context.Context) error

	// Stop stops the server
	Stop(ctx context.Context) error

	// IsRunning returns true if the server is running
	IsRunning() bool

	// SetHandler sets the handler for a specific Modbus function code
	SetHandler(functionCode FunctionCode, handler HandlerFunc)

	// WithLogger sets the logger for the server
	WithLogger(logger LoggerInterface) Server

	// WithDataStore sets the data store for the server
	WithDataStore(dataStore DataStore) Server
}

// ServerOption is a function that configures a server
type ServerOption func(Server)

// WithServerLogger sets the logger for the server
func WithServerLogger(logger LoggerInterface) ServerOption {
	return func(s Server) {
		s.WithLogger(logger)
	}
}

// WithServerDataStore sets the data store for the server
func WithServerDataStore(dataStore DataStore) ServerOption {
	return func(s Server) {
		s.WithDataStore(dataStore)
	}
}
