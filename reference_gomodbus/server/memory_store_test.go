package server

import (
	"context"
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
)

func TestMemoryStore_ReadCoils(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Set up test data
	store.SetCoil(common.Address(100), true)
	store.SetCoil(common.Address(101), false)
	store.SetCoil(common.Address(102), true)
	
	// Test valid read
	values, err := store.ReadCoils(ctx, common.Address(100), common.Quantity(3))
	if err != nil {
		t.Fatalf("ReadCoils returned error: %v", err)
	}
	
	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}
	
	expected := []common.CoilValue{true, false, true}
	for i, expectedValue := range expected {
		if values[i] != expectedValue {
			t.Errorf("Value at index %d: expected %t, got %t", i, expectedValue, values[i])
		}
	}
	
	// Test reading unset values (should be false by default)
	values, err = store.ReadCoils(ctx, common.Address(200), common.Quantity(2))
	if err != nil {
		t.Fatalf("ReadCoils for unset values returned error: %v", err)
	}
	
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
	
	for i, value := range values {
		if value {
			t.Errorf("Unset value at index %d should be false, got true", i)
		}
	}
	
	// Test invalid quantity
	_, err = store.ReadCoils(ctx, common.Address(100), common.Quantity(0))
	if err == nil {
		t.Error("ReadCoils with quantity=0 should return error")
	}
	
	_, err = store.ReadCoils(ctx, common.Address(100), common.Quantity(common.MaxCoilCount+1))
	if err == nil {
		t.Error("ReadCoils with quantity > MaxCoilCount should return error")
	}
}

func TestMemoryStore_ReadDiscreteInputs(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Set up test data
	store.SetDiscreteInput(common.Address(100), true)
	store.SetDiscreteInput(common.Address(101), true)
	store.SetDiscreteInput(common.Address(102), false)
	
	// Test valid read
	values, err := store.ReadDiscreteInputs(ctx, common.Address(100), common.Quantity(3))
	if err != nil {
		t.Fatalf("ReadDiscreteInputs returned error: %v", err)
	}
	
	if len(values) != 3 {
		t.Fatalf("Expected 3 values, got %d", len(values))
	}
	
	expected := []common.DiscreteInputValue{true, true, false}
	for i, expectedValue := range expected {
		if values[i] != expectedValue {
			t.Errorf("Value at index %d: expected %t, got %t", i, expectedValue, values[i])
		}
	}
}

func TestMemoryStore_ReadHoldingRegisters(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Set up test data
	store.SetHoldingRegister(common.Address(100), 0x1234)
	store.SetHoldingRegister(common.Address(101), 0x5678)
	
	// Test valid read
	values, err := store.ReadHoldingRegisters(ctx, common.Address(100), common.Quantity(2))
	if err != nil {
		t.Fatalf("ReadHoldingRegisters returned error: %v", err)
	}
	
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
	
	expected := []common.RegisterValue{0x1234, 0x5678}
	for i, expectedValue := range expected {
		if values[i] != expectedValue {
			t.Errorf("Value at index %d: expected 0x%04X, got 0x%04X", 
				i, expectedValue, values[i])
		}
	}
	
	// Test reading unset values (should be 0 by default)
	values, err = store.ReadHoldingRegisters(ctx, common.Address(200), common.Quantity(2))
	if err != nil {
		t.Fatalf("ReadHoldingRegisters for unset values returned error: %v", err)
	}
	
	if len(values) != 2 {
		t.Fatalf("Expected 2 values, got %d", len(values))
	}
	
	for i, value := range values {
		if value != 0 {
			t.Errorf("Unset value at index %d should be 0, got %d", i, value)
		}
	}
	
	// Test invalid quantity
	_, err = store.ReadHoldingRegisters(ctx, common.Address(100), common.Quantity(0))
	if err == nil {
		t.Error("ReadHoldingRegisters with quantity=0 should return error")
	}
	
	_, err = store.ReadHoldingRegisters(ctx, common.Address(100), common.Quantity(common.MaxRegisterCount+1))
	if err == nil {
		t.Error("ReadHoldingRegisters with quantity > MaxRegisterCount should return error")
	}
}

func TestMemoryStore_WriteSingleCoil(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Test writing a value
	address := common.Address(100)
	value := common.CoilValue(true)
	
	err := store.WriteSingleCoil(ctx, address, value)
	if err != nil {
		t.Fatalf("WriteSingleCoil returned error: %v", err)
	}
	
	// Verify the value was written correctly
	storedValue, ok := store.GetCoil(address)
	if !ok {
		t.Fatalf("No value stored at address %d", address)
	}
	
	if storedValue != value {
		t.Errorf("Expected value %t at address %d, got %t", value, address, storedValue)
	}
	
	// Test overwriting a value
	newValue := common.CoilValue(false)
	
	err = store.WriteSingleCoil(ctx, address, newValue)
	if err != nil {
		t.Fatalf("WriteSingleCoil (overwrite) returned error: %v", err)
	}
	
	// Verify the value was updated
	storedValue, ok = store.GetCoil(address)
	if !ok {
		t.Fatalf("No value stored at address %d after overwrite", address)
	}
	
	if storedValue != newValue {
		t.Errorf("Expected value %t at address %d after overwrite, got %t", 
			newValue, address, storedValue)
	}
}

func TestMemoryStore_WriteMultipleCoils(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Test writing multiple values
	address := common.Address(100)
	values := []common.CoilValue{true, false, true}
	
	err := store.WriteMultipleCoils(ctx, address, values)
	if err != nil {
		t.Fatalf("WriteMultipleCoils returned error: %v", err)
	}
	
	// Verify values were written correctly
	for i, expectedValue := range values {
		addr := address + common.Address(i)
		storedValue, ok := store.GetCoil(addr)
		if !ok {
			t.Fatalf("No value stored at address %d", addr)
		}
		
		if storedValue != expectedValue {
			t.Errorf("Expected value %t at address %d, got %t", 
				expectedValue, addr, storedValue)
		}
	}
	
	// Test with an empty slice
	err = store.WriteMultipleCoils(ctx, address, []common.CoilValue{})
	if err == nil {
		t.Error("WriteMultipleCoils with empty slice should return error")
	}
	
	// Test with too many values
	tooManyValues := make([]common.CoilValue, common.MaxCoilCount+1)
	err = store.WriteMultipleCoils(ctx, address, tooManyValues)
	if err == nil {
		t.Error("WriteMultipleCoils with too many values should return error")
	}
}

func TestMemoryStore_WriteMultipleRegisters(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	
	// Test writing multiple values
	address := common.Address(100)
	values := []common.RegisterValue{0x1234, 0x5678}
	
	err := store.WriteMultipleRegisters(ctx, address, values)
	if err != nil {
		t.Fatalf("WriteMultipleRegisters returned error: %v", err)
	}
	
	// Verify values were written correctly
	for i, expectedValue := range values {
		addr := address + common.Address(i)
		storedValue, ok := store.GetHoldingRegister(addr)
		if !ok {
			t.Fatalf("No value stored at address %d", addr)
		}
		
		if storedValue != expectedValue {
			t.Errorf("Expected value 0x%04X at address %d, got 0x%04X", 
				expectedValue, addr, storedValue)
		}
	}
}

func TestMemoryStore_DumpRegisters(t *testing.T) {
	store := NewMemoryStore()
	
	// Set up some test data
	store.SetCoil(common.Address(100), true)
	store.SetDiscreteInput(common.Address(200), true)
	store.SetHoldingRegister(common.Address(300), 0x1234)
	store.SetInputRegister(common.Address(400), 0x5678)
	
	// Get the dump string
	dump := store.DumpRegisters()
	
	// Basic validation - the dump should contain all the values we set
	if dump == "" {
		t.Error("DumpRegisters returned empty string")
	}
	
	// Check that the dump contains the values we set
	expectedStrings := []string{
		"Coils:", "100: true",
		"Discrete Inputs:", "200: true",
		"Holding Registers:", "300: 4660 (0x1234)",
		"Input Registers:", "400: 22136 (0x5678)",
	}

	for _, expected := range expectedStrings {
		if !contains(dump, expected) {
			t.Errorf("Expected dump to contain '%s', but it doesn't", expected)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}