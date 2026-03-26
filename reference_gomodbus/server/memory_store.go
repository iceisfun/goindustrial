package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// MemoryStore implements DataStore with in-memory storage
// Provides storage for all four Modbus data types as defined in the specification
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Model)
type MemoryStore struct {
	// Coils (read-write 1-bit outputs) - Function codes 0x01 (read) and 0x05/0x0F (write)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Coil/Output)
	coils            map[common.Address]common.CoilValue

	// Discrete Inputs (read-only 1-bit inputs) - Function code 0x02 (read)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Discrete Input)
	discreteInputs   map[common.Address]common.DiscreteInputValue

	// Holding Registers (read-write 16-bit registers) - Function codes 0x03 (read) and 0x06/0x10 (write)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Holding Register)
	holdingRegisters map[common.Address]common.RegisterValue

	// Input Registers (read-only 16-bit registers) - Function code 0x04 (read)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Input Register)
	inputRegisters   map[common.Address]common.InputRegisterValue

	// Mutex to protect concurrent access to maps
	mu               sync.RWMutex
}

// NewMemoryStore creates a new memory-based data store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		coils:            make(map[common.Address]common.CoilValue),
		discreteInputs:   make(map[common.Address]common.DiscreteInputValue),
		holdingRegisters: make(map[common.Address]common.RegisterValue),
		inputRegisters:   make(map[common.Address]common.InputRegisterValue),
	}
}

// ReadCoils reads coil values from the data store
// Implements function code 0x01 (Read Coils) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Read Coils)
func (s *MemoryStore) ReadCoils(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.CoilValue, error) {
	// Validate quantity within Modbus limits (1-2000 coils)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Quantity of Coils)
	if quantity == 0 || quantity > common.MaxCoilCount {
		return nil, common.ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]common.CoilValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := s.coils[addr]; ok {
			values[i] = value
		}
		// If not found in the map, the default value is false
	}

	return values, nil
}

// ReadDiscreteInputs reads discrete input values from the data store
// Implements function code 0x02 (Read Discrete Inputs) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.2 (Read Discrete Inputs)
func (s *MemoryStore) ReadDiscreteInputs(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.DiscreteInputValue, error) {
	// Validate quantity within Modbus limits (1-2000 inputs)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.2 (Quantity of Inputs)
	if quantity == 0 || quantity > common.MaxCoilCount {
		return nil, common.ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]common.DiscreteInputValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := s.discreteInputs[addr]; ok {
			values[i] = value
		}
		// If not found in the map, the default value is false
	}

	return values, nil
}

// ReadHoldingRegisters reads holding register values from the data store
// Implements function code 0x03 (Read Holding Registers) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Read Holding Registers)
func (s *MemoryStore) ReadHoldingRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.RegisterValue, error) {
	// Validate quantity within Modbus limits (1-125 registers)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Quantity of Registers)
	if quantity == 0 || quantity > common.MaxRegisterCount {
		return nil, common.ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]common.RegisterValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := s.holdingRegisters[addr]; ok {
			values[i] = value
		}
		// If not found in the map, the default value is 0
	}

	return values, nil
}

// ReadInputRegisters reads input register values from the data store
// Implements function code 0x04 (Read Input Registers) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Read Input Registers)
func (s *MemoryStore) ReadInputRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.InputRegisterValue, error) {
	// Validate quantity within Modbus limits (1-125 registers)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Quantity of Input Registers)
	if quantity == 0 || quantity > common.MaxRegisterCount {
		return nil, common.ErrInvalidQuantity
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	values := make([]common.InputRegisterValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := s.inputRegisters[addr]; ok {
			values[i] = value
		}
		// If not found in the map, the default value is 0
	}

	return values, nil
}

// WriteSingleCoil writes a single coil value to the data store
// Implements function code 0x05 (Write Single Coil) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Write Single Coil)
func (s *MemoryStore) WriteSingleCoil(ctx context.Context, address common.Address, value common.CoilValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.coils[address] = value
	return nil
}

// WriteSingleRegister writes a single register value to the data store
// Implements function code 0x06 (Write Single Register) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.6 (Write Single Register)
func (s *MemoryStore) WriteSingleRegister(ctx context.Context, address common.Address, value common.RegisterValue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.holdingRegisters[address] = value
	return nil
}

// WriteMultipleCoils writes multiple coil values to the data store
// Implements function code 0x0F (Write Multiple Coils) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Write Multiple Coils)
func (s *MemoryStore) WriteMultipleCoils(ctx context.Context, address common.Address, values []common.CoilValue) error {
	// Validate quantity within Modbus write limits (1-1968 coils)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Quantity of Outputs)
	if len(values) == 0 || len(values) > int(common.MaxWriteCoilCount) {
		return common.ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, value := range values {
		addr := address + common.Address(i)
		s.coils[addr] = value
	}

	return nil
}

// WriteMultipleRegisters writes multiple register values to the data store
// Implements function code 0x10 (Write Multiple Registers) data access
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Write Multiple Registers)
func (s *MemoryStore) WriteMultipleRegisters(ctx context.Context, address common.Address, values []common.RegisterValue) error {
	// Validate quantity within Modbus limits (1-123 registers)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Quantity of Registers)
	if len(values) == 0 || len(values) > int(common.MaxWriteRegisterCount) {
		return common.ErrInvalidQuantity
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, value := range values {
		addr := address + common.Address(i)
		s.holdingRegisters[addr] = value
	}

	return nil
}

// GetCoil gets a single coil value
func (s *MemoryStore) GetCoil(address common.Address) (common.CoilValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.coils[address]
	return value, exists
}

// SetCoil sets a single coil value
func (s *MemoryStore) SetCoil(address common.Address, value common.CoilValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.coils[address] = value
}

// GetDiscreteInput gets a single discrete input value
func (s *MemoryStore) GetDiscreteInput(address common.Address) (common.DiscreteInputValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.discreteInputs[address]
	return value, exists
}

// SetDiscreteInput sets a single discrete input value
func (s *MemoryStore) SetDiscreteInput(address common.Address, value common.DiscreteInputValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.discreteInputs[address] = value
}

// GetHoldingRegister gets a single holding register value
func (s *MemoryStore) GetHoldingRegister(address common.Address) (common.RegisterValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.holdingRegisters[address]
	return value, exists
}

// SetHoldingRegister sets a single holding register value
func (s *MemoryStore) SetHoldingRegister(address common.Address, value common.RegisterValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.holdingRegisters[address] = value
}

// GetInputRegister gets a single input register value
func (s *MemoryStore) GetInputRegister(address common.Address) (common.InputRegisterValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	value, exists := s.inputRegisters[address]
	return value, exists
}

// SetInputRegister sets a single input register value
func (s *MemoryStore) SetInputRegister(address common.Address, value common.InputRegisterValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inputRegisters[address] = value
}

// DumpRegisters returns a string representation of the memory store's content
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