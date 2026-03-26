package test

import (
	"context"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// MockDataStore implements common.DataStore for testing
type MockDataStore struct {
	mu                sync.RWMutex
	coils             map[common.Address]common.CoilValue
	discreteInputs    map[common.Address]common.DiscreteInputValue
	holdingRegisters  map[common.Address]common.RegisterValue
	inputRegisters    map[common.Address]common.InputRegisterValue
	failFlag          bool
	failOnAddress     *common.Address // If set, fail when this address is accessed
	failOnQuantity    *common.Quantity // If set, fail when this quantity is requested
}

// NewMockDataStore creates a new mock data store
func NewMockDataStore() *MockDataStore {
	return &MockDataStore{
		coils:            make(map[common.Address]common.CoilValue),
		discreteInputs:   make(map[common.Address]common.DiscreteInputValue),
		holdingRegisters: make(map[common.Address]common.RegisterValue),
		inputRegisters:   make(map[common.Address]common.InputRegisterValue),
		failFlag:         false,
	}
}

// SetFail sets the fail flag
func (ds *MockDataStore) SetFail(fail bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.failFlag = fail
}

// SetFailOnAddress sets an address that will cause operations to fail
func (ds *MockDataStore) SetFailOnAddress(address common.Address) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.failOnAddress = &address
}

// ClearFailOnAddress clears the fail-on-address flag
func (ds *MockDataStore) ClearFailOnAddress() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.failOnAddress = nil
}

// SetFailOnQuantity sets a quantity that will cause operations to fail
func (ds *MockDataStore) SetFailOnQuantity(quantity common.Quantity) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.failOnQuantity = &quantity
}

// ClearFailOnQuantity clears the fail-on-quantity flag
func (ds *MockDataStore) ClearFailOnQuantity() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.failOnQuantity = nil
}

// ReadCoils reads coil values from the data store
func (ds *MockDataStore) ReadCoils(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.CoilValue, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.failFlag {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return nil, common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxCoilCount {
		return nil, common.ErrInvalidQuantity
	}

	values := make([]common.CoilValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := ds.coils[addr]; ok {
			values[i] = value
		}
		// Default is false for unset coils
	}

	return values, nil
}

// ReadDiscreteInputs reads discrete input values from the data store
func (ds *MockDataStore) ReadDiscreteInputs(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.DiscreteInputValue, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.failFlag {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return nil, common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxCoilCount {
		return nil, common.ErrInvalidQuantity
	}

	values := make([]common.DiscreteInputValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := ds.discreteInputs[addr]; ok {
			values[i] = value
		}
		// Default is false for unset discrete inputs
	}

	return values, nil
}

// ReadHoldingRegisters reads holding register values from the data store
func (ds *MockDataStore) ReadHoldingRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.RegisterValue, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.failFlag {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return nil, common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxRegisterCount {
		return nil, common.ErrInvalidQuantity
	}

	values := make([]common.RegisterValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := ds.holdingRegisters[addr]; ok {
			values[i] = value
		}
		// Default is 0 for unset registers
	}

	return values, nil
}

// ReadInputRegisters reads input register values from the data store
func (ds *MockDataStore) ReadInputRegisters(ctx context.Context, address common.Address, quantity common.Quantity) ([]common.InputRegisterValue, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.failFlag {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return nil, common.ErrServerDeviceFailure
	}

	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return nil, common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxRegisterCount {
		return nil, common.ErrInvalidQuantity
	}

	values := make([]common.InputRegisterValue, quantity)
	for i := common.Quantity(0); i < quantity; i++ {
		addr := address + common.Address(i)
		if value, ok := ds.inputRegisters[addr]; ok {
			values[i] = value
		}
		// Default is 0 for unset registers
	}

	return values, nil
}

// WriteSingleCoil writes a single coil value to the data store
func (ds *MockDataStore) WriteSingleCoil(ctx context.Context, address common.Address, value common.CoilValue) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.failFlag {
		return common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return common.ErrServerDeviceFailure
	}

	ds.coils[address] = value
	return nil
}

// WriteSingleRegister writes a single register value to the data store
func (ds *MockDataStore) WriteSingleRegister(ctx context.Context, address common.Address, value common.RegisterValue) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.failFlag {
		return common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return common.ErrServerDeviceFailure
	}

	ds.holdingRegisters[address] = value
	return nil
}

// WriteMultipleCoils writes multiple coil values to the data store
func (ds *MockDataStore) WriteMultipleCoils(ctx context.Context, address common.Address, values []common.CoilValue) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.failFlag {
		return common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return common.ErrServerDeviceFailure
	}

	quantity := common.Quantity(len(values))
	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxCoilCount {
		return common.ErrInvalidQuantity
	}

	for i, value := range values {
		addr := address + common.Address(i)
		ds.coils[addr] = value
	}

	return nil
}

// WriteMultipleRegisters writes multiple register values to the data store
func (ds *MockDataStore) WriteMultipleRegisters(ctx context.Context, address common.Address, values []common.RegisterValue) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.failFlag {
		return common.ErrServerDeviceFailure
	}

	if ds.failOnAddress != nil && *ds.failOnAddress == address {
		return common.ErrServerDeviceFailure
	}

	quantity := common.Quantity(len(values))
	if ds.failOnQuantity != nil && *ds.failOnQuantity == quantity {
		return common.ErrInvalidQuantity
	}

	// Validate quantity
	if quantity == 0 || quantity > common.MaxRegisterCount {
		return common.ErrInvalidQuantity
	}

	for i, value := range values {
		addr := address + common.Address(i)
		ds.holdingRegisters[addr] = value
	}

	return nil
}

// SetCoil sets a coil value directly (for test setup)
func (ds *MockDataStore) SetCoil(address common.Address, value common.CoilValue) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.coils[address] = value
}

// SetDiscreteInput sets a discrete input value directly (for test setup)
func (ds *MockDataStore) SetDiscreteInput(address common.Address, value common.DiscreteInputValue) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.discreteInputs[address] = value
}

// SetHoldingRegister sets a holding register value directly (for test setup)
func (ds *MockDataStore) SetHoldingRegister(address common.Address, value common.RegisterValue) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.holdingRegisters[address] = value
}

// SetInputRegister sets an input register value directly (for test setup)
func (ds *MockDataStore) SetInputRegister(address common.Address, value common.InputRegisterValue) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.inputRegisters[address] = value
}

// GetCoil gets a coil value directly (for verification)
func (ds *MockDataStore) GetCoil(address common.Address) (common.CoilValue, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, ok := ds.coils[address]
	return value, ok
}

// GetDiscreteInput gets a discrete input value directly (for verification)
func (ds *MockDataStore) GetDiscreteInput(address common.Address) (common.DiscreteInputValue, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, ok := ds.discreteInputs[address]
	return value, ok
}

// GetHoldingRegister gets a holding register value directly (for verification)
func (ds *MockDataStore) GetHoldingRegister(address common.Address) (common.RegisterValue, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, ok := ds.holdingRegisters[address]
	return value, ok
}

// GetInputRegister gets an input register value directly (for verification)
func (ds *MockDataStore) GetInputRegister(address common.Address) (common.InputRegisterValue, bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	value, ok := ds.inputRegisters[address]
	return value, ok
}