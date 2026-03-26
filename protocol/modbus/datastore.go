package modbus

import (
	"context"
	"fmt"
	"sync"
)

// HandlerFunc is a function that processes a Modbus request and returns a response.
type HandlerFunc func(ctx context.Context, request *Request) (*Response, error)

// DefaultHandlerFunc returns a "not implemented" error for any request.
func DefaultHandlerFunc(ctx context.Context, request *Request) (*Response, error) {
	return nil, NewModbusError(
		request.GetPDU().FunctionCode,
		ExceptionFunctionCodeNotSupported,
	)
}

// DataStore represents a Modbus data store with read/write capabilities.
type DataStore interface {
	// ReadCoils reads coil values from the data store.
	ReadCoils(ctx context.Context, address Address, quantity Quantity) ([]CoilValue, error)

	// ReadDiscreteInputs reads discrete input values from the data store.
	ReadDiscreteInputs(ctx context.Context, address Address, quantity Quantity) ([]DiscreteInputValue, error)

	// ReadHoldingRegisters reads holding register values from the data store.
	ReadHoldingRegisters(ctx context.Context, address Address, quantity Quantity) ([]RegisterValue, error)

	// ReadInputRegisters reads input register values from the data store.
	ReadInputRegisters(ctx context.Context, address Address, quantity Quantity) ([]InputRegisterValue, error)

	// WriteSingleCoil writes a single coil value to the data store.
	WriteSingleCoil(ctx context.Context, address Address, value CoilValue) error

	// WriteSingleRegister writes a single register value to the data store.
	WriteSingleRegister(ctx context.Context, address Address, value RegisterValue) error

	// WriteMultipleCoils writes multiple coil values to the data store.
	WriteMultipleCoils(ctx context.Context, address Address, values []CoilValue) error

	// WriteMultipleRegisters writes multiple register values to the data store.
	WriteMultipleRegisters(ctx context.Context, address Address, values []RegisterValue) error
}

// MemoryStore implements DataStore with in-memory storage.
// Provides storage for all four Modbus data types as defined in the specification.
type MemoryStore struct {
	coils            map[Address]CoilValue
	discreteInputs   map[Address]DiscreteInputValue
	holdingRegisters map[Address]RegisterValue
	inputRegisters   map[Address]InputRegisterValue
	mu               sync.RWMutex
}

// NewMemoryStore creates a new memory-based data store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		coils:            make(map[Address]CoilValue),
		discreteInputs:   make(map[Address]DiscreteInputValue),
		holdingRegisters: make(map[Address]RegisterValue),
		inputRegisters:   make(map[Address]InputRegisterValue),
	}
}

// ReadCoils reads coil values from the data store.
func (s *MemoryStore) ReadCoils(ctx context.Context, address Address, quantity Quantity) ([]CoilValue, error) {
	if quantity == 0 || quantity > MaxCoilCount {
		return nil, ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]CoilValue, quantity)
	for i := Quantity(0); i < quantity; i++ {
		addr := address + Address(i)
		if value, ok := s.coils[addr]; ok {
			values[i] = value
		}
	}

	return values, nil
}

// ReadDiscreteInputs reads discrete input values from the data store.
func (s *MemoryStore) ReadDiscreteInputs(ctx context.Context, address Address, quantity Quantity) ([]DiscreteInputValue, error) {
	if quantity == 0 || quantity > MaxCoilCount {
		return nil, ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]DiscreteInputValue, quantity)
	for i := Quantity(0); i < quantity; i++ {
		addr := address + Address(i)
		if value, ok := s.discreteInputs[addr]; ok {
			values[i] = value
		}
	}

	return values, nil
}

// ReadHoldingRegisters reads holding register values from the data store.
func (s *MemoryStore) ReadHoldingRegisters(ctx context.Context, address Address, quantity Quantity) ([]RegisterValue, error) {
	if quantity == 0 || quantity > MaxRegisterCount {
		return nil, ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]RegisterValue, quantity)
	for i := Quantity(0); i < quantity; i++ {
		addr := address + Address(i)
		if value, ok := s.holdingRegisters[addr]; ok {
			values[i] = value
		}
	}

	return values, nil
}

// ReadInputRegisters reads input register values from the data store.
func (s *MemoryStore) ReadInputRegisters(ctx context.Context, address Address, quantity Quantity) ([]InputRegisterValue, error) {
	if quantity == 0 || quantity > MaxRegisterCount {
		return nil, ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]InputRegisterValue, quantity)
	for i := Quantity(0); i < quantity; i++ {
		addr := address + Address(i)
		if value, ok := s.inputRegisters[addr]; ok {
			values[i] = value
		}
	}

	return values, nil
}

// WriteSingleCoil writes a single coil value to the data store.
func (s *MemoryStore) WriteSingleCoil(ctx context.Context, address Address, value CoilValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.coils[address] = value
	return nil
}

// WriteSingleRegister writes a single register value to the data store.
func (s *MemoryStore) WriteSingleRegister(ctx context.Context, address Address, value RegisterValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.holdingRegisters[address] = value
	return nil
}

// WriteMultipleCoils writes multiple coil values to the data store.
func (s *MemoryStore) WriteMultipleCoils(ctx context.Context, address Address, values []CoilValue) error {
	if len(values) == 0 || len(values) > int(MaxWriteCoilCount) {
		return ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, value := range values {
		addr := address + Address(i)
		s.coils[addr] = value
	}

	return nil
}

// WriteMultipleRegisters writes multiple register values to the data store.
func (s *MemoryStore) WriteMultipleRegisters(ctx context.Context, address Address, values []RegisterValue) error {
	if len(values) == 0 || len(values) > int(MaxWriteRegisterCount) {
		return ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, value := range values {
		addr := address + Address(i)
		s.holdingRegisters[addr] = value
	}

	return nil
}

// GetCoil gets a single coil value.
func (s *MemoryStore) GetCoil(address Address) (CoilValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.coils[address]
	return value, exists
}

// SetCoil sets a single coil value.
func (s *MemoryStore) SetCoil(address Address, value CoilValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.coils[address] = value
}

// GetDiscreteInput gets a single discrete input value.
func (s *MemoryStore) GetDiscreteInput(address Address) (DiscreteInputValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.discreteInputs[address]
	return value, exists
}

// SetDiscreteInput sets a single discrete input value.
func (s *MemoryStore) SetDiscreteInput(address Address, value DiscreteInputValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.discreteInputs[address] = value
}

// GetHoldingRegister gets a single holding register value.
func (s *MemoryStore) GetHoldingRegister(address Address) (RegisterValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.holdingRegisters[address]
	return value, exists
}

// SetHoldingRegister sets a single holding register value.
func (s *MemoryStore) SetHoldingRegister(address Address, value RegisterValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.holdingRegisters[address] = value
}

// GetInputRegister gets a single input register value.
func (s *MemoryStore) GetInputRegister(address Address) (InputRegisterValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.inputRegisters[address]
	return value, exists
}

// SetInputRegister sets a single input register value.
func (s *MemoryStore) SetInputRegister(address Address, value InputRegisterValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inputRegisters[address] = value
}

// DumpRegisters returns a string representation of the memory store's content.
func (s *MemoryStore) DumpRegisters() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := "Memory Store Content:\n"

	if len(s.coils) > 0 {
		result += "Coils:\n"
		for addr, val := range s.coils {
			result += fmt.Sprintf("  %d: %t\n", uint16(addr), val)
		}
	}

	if len(s.discreteInputs) > 0 {
		result += "Discrete Inputs:\n"
		for addr, val := range s.discreteInputs {
			result += fmt.Sprintf("  %d: %t\n", uint16(addr), val)
		}
	}

	if len(s.holdingRegisters) > 0 {
		result += "Holding Registers:\n"
		for addr, val := range s.holdingRegisters {
			result += fmt.Sprintf("  %d: %d (0x%04X)\n", uint16(addr), val, val)
		}
	}

	if len(s.inputRegisters) > 0 {
		result += "Input Registers:\n"
		for addr, val := range s.inputRegisters {
			result += fmt.Sprintf("  %d: %d (0x%04X)\n", uint16(addr), val, val)
		}
	}

	return result
}
