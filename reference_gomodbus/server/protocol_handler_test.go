package server

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/common/test"
)

func TestHandleReadCoils(t *testing.T) {
	handler := newServerProtocolHandler()
	ctx := context.Background()
	
	// Create mock datastore with test data
	store := test.NewMockDataStore()
	store.SetCoil(common.Address(100), true)
	store.SetCoil(common.Address(101), false)
	store.SetCoil(common.Address(102), true)
	
	// Create a valid read coils request
	address := common.Address(100)
	quantity := common.Quantity(3)
	
	// Create request data
	reqData := make([]byte, 4)
	binary.BigEndian.PutUint16(reqData[0:2], uint16(address))
	binary.BigEndian.PutUint16(reqData[2:4], uint16(quantity))
	
	// Create the request
	req := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncReadCoils,
		reqData,
	)
	
	// Process the request
	resp, err := handler.HandleReadCoils(ctx, req, store)
	if err != nil {
		t.Fatalf("HandleReadCoils returned error: %v", err)
	}
	
	// Verify response
	respData := resp.GetPDU().Data
	
	// First byte should be the byte count (1 for 3 coils)
	if respData[0] != 1 {
		t.Errorf("Expected byte count 1, got %d", respData[0])
	}
	
	// Second byte should have bits 0 and 2 set (true), bit 1 clear (false)
	// 0b00000101 = 5
	if respData[1] != 5 {
		t.Errorf("Expected coil data 0x05, got 0x%02X", respData[1])
	}
	
	// Test invalid request (wrong data length)
	invalidReq := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncReadCoils,
		[]byte{0x00, 0x64}, // Too short
	)
	
	_, err = handler.HandleReadCoils(ctx, invalidReq, store)
	if err == nil {
		t.Error("HandleReadCoils with invalid data length should return error")
	}
	
	// Test with an invalid quantity
	invalidQuantityReq := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncReadCoils,
		[]byte{0x00, 0x64, 0x00, 0x00}, // Quantity = 0
	)
	
	_, err = handler.HandleReadCoils(ctx, invalidQuantityReq, store)
	if err == nil {
		t.Error("HandleReadCoils with quantity=0 should return error")
	}
	
	// Test with datastore failure
	store.SetFailOnAddress(address)
	
	_, err = handler.HandleReadCoils(ctx, req, store)
	if err == nil {
		t.Error("HandleReadCoils with datastore failure should return error")
	}
	
	store.ClearFailOnAddress()
}

func TestHandleReadDiscreteInputs(t *testing.T) {
	handler := newServerProtocolHandler()
	ctx := context.Background()
	
	// Create mock datastore with test data
	store := test.NewMockDataStore()
	store.SetDiscreteInput(common.Address(100), true)
	store.SetDiscreteInput(common.Address(101), true)
	store.SetDiscreteInput(common.Address(102), false)
	
	// Create a valid read discrete inputs request
	address := common.Address(100)
	quantity := common.Quantity(3)
	
	// Create request data
	reqData := make([]byte, 4)
	binary.BigEndian.PutUint16(reqData[0:2], uint16(address))
	binary.BigEndian.PutUint16(reqData[2:4], uint16(quantity))
	
	// Create the request
	req := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncReadDiscreteInputs,
		reqData,
	)
	
	// Process the request
	resp, err := handler.HandleReadDiscreteInputs(ctx, req, store)
	if err != nil {
		t.Fatalf("HandleReadDiscreteInputs returned error: %v", err)
	}
	
	// Verify response
	respData := resp.GetPDU().Data
	
	// First byte should be the byte count (1 for 3 discrete inputs)
	if respData[0] != 1 {
		t.Errorf("Expected byte count 1, got %d", respData[0])
	}
	
	// Second byte should have bits 0 and 1 set (true), bit 2 clear (false)
	// 0b00000011 = 3
	if respData[1] != 3 {
		t.Errorf("Expected discrete input data 0x03, got 0x%02X", respData[1])
	}
}

func TestHandleReadHoldingRegisters(t *testing.T) {
	handler := newServerProtocolHandler()
	ctx := context.Background()
	
	// Create mock datastore with test data
	store := test.NewMockDataStore()
	store.SetHoldingRegister(common.Address(100), 0x1234)
	store.SetHoldingRegister(common.Address(101), 0x5678)
	
	// Create a valid read holding registers request
	address := common.Address(100)
	quantity := common.Quantity(2)
	
	// Create request data
	reqData := make([]byte, 4)
	binary.BigEndian.PutUint16(reqData[0:2], uint16(address))
	binary.BigEndian.PutUint16(reqData[2:4], uint16(quantity))
	
	// Create the request
	req := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncReadHoldingRegisters,
		reqData,
	)
	
	// Process the request
	resp, err := handler.HandleReadHoldingRegisters(ctx, req, store)
	if err != nil {
		t.Fatalf("HandleReadHoldingRegisters returned error: %v", err)
	}
	
	// Verify response
	respData := resp.GetPDU().Data
	
	// First byte should be the byte count (4 for 2 registers)
	if respData[0] != 4 {
		t.Errorf("Expected byte count 4, got %d", respData[0])
	}
	
	// First register value (0x1234)
	reg1 := binary.BigEndian.Uint16(respData[1:3])
	if reg1 != 0x1234 {
		t.Errorf("Expected first register value 0x1234, got 0x%04X", reg1)
	}
	
	// Second register value (0x5678)
	reg2 := binary.BigEndian.Uint16(respData[3:5])
	if reg2 != 0x5678 {
		t.Errorf("Expected second register value 0x5678, got 0x%04X", reg2)
	}
}

func TestHandleWriteSingleCoil(t *testing.T) {
	handler := newServerProtocolHandler()
	ctx := context.Background()
	
	// Create mock datastore
	store := test.NewMockDataStore()
	
	// Create a valid write single coil request (ON)
	address := common.Address(100)
	
	// Create request data
	reqData := make([]byte, 4)
	binary.BigEndian.PutUint16(reqData[0:2], uint16(address))
	binary.BigEndian.PutUint16(reqData[2:4], common.CoilOnU16) // ON = 0xFF00
	
	// Create the request
	req := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncWriteSingleCoil,
		reqData,
	)
	
	// Process the request
	resp, err := handler.HandleWriteSingleCoil(ctx, req, store)
	if err != nil {
		t.Fatalf("HandleWriteSingleCoil returned error: %v", err)
	}
	
	// Verify response (should echo the request)
	respData := resp.GetPDU().Data
	
	if len(respData) != 4 {
		t.Errorf("Expected response data length 4, got %d", len(respData))
	}
	
	respAddress := binary.BigEndian.Uint16(respData[0:2])
	if respAddress != uint16(address) {
		t.Errorf("Expected response address %d, got %d", address, respAddress)
	}
	
	respValue := binary.BigEndian.Uint16(respData[2:4])
	if respValue != common.CoilOnU16 {
		t.Errorf("Expected response value 0xFF00, got 0x%04X", respValue)
	}
	
	// Verify that the value was actually written to the datastore
	value, ok := store.GetCoil(address)
	if !ok {
		t.Fatalf("Coil value at address %d was not written", address)
	}
	
	if !value {
		t.Errorf("Expected coil value true, got false")
	}
	
	// Test with an invalid value
	invalidValueReq := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncWriteSingleCoil,
		[]byte{0x00, 0x64, 0x12, 0x34}, // Neither ON nor OFF
	)
	
	_, err = handler.HandleWriteSingleCoil(ctx, invalidValueReq, store)
	if err == nil {
		t.Error("HandleWriteSingleCoil with invalid value should return error")
	}
}

func TestHandleWriteMultipleRegisters(t *testing.T) {
	handler := newServerProtocolHandler()
	ctx := context.Background()
	
	// Create mock datastore
	store := test.NewMockDataStore()
	
	// Create a valid write multiple registers request
	address := common.Address(100)
	quantity := common.Quantity(2)
	values := []common.RegisterValue{0x1234, 0x5678}
	
	// Create request data
	reqData := make([]byte, 9) // Address (2) + Quantity (2) + Byte count (1) + Values (2*2)
	binary.BigEndian.PutUint16(reqData[0:2], uint16(address))
	binary.BigEndian.PutUint16(reqData[2:4], uint16(quantity))
	reqData[4] = 4 // Byte count (2 registers * 2 bytes each)
	binary.BigEndian.PutUint16(reqData[5:7], values[0])
	binary.BigEndian.PutUint16(reqData[7:9], values[1])
	
	// Create the request
	req := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncWriteMultipleRegisters,
		reqData,
	)
	
	// Process the request
	resp, err := handler.HandleWriteMultipleRegisters(ctx, req, store)
	if err != nil {
		t.Fatalf("HandleWriteMultipleRegisters returned error: %v", err)
	}
	
	// Verify response
	respData := resp.GetPDU().Data
	
	if len(respData) != 4 {
		t.Errorf("Expected response data length 4, got %d", len(respData))
	}
	
	respAddress := binary.BigEndian.Uint16(respData[0:2])
	if respAddress != uint16(address) {
		t.Errorf("Expected response address %d, got %d", address, respAddress)
	}
	
	respQuantity := binary.BigEndian.Uint16(respData[2:4])
	if respQuantity != uint16(quantity) {
		t.Errorf("Expected response quantity %d, got %d", quantity, respQuantity)
	}
	
	// Verify that the values were actually written to the datastore
	for i, expectedValue := range values {
		addr := address + common.Address(i)
		value, ok := store.GetHoldingRegister(addr)
		if !ok {
			t.Fatalf("Register value at address %d was not written", addr)
		}
		
		if value != expectedValue {
			t.Errorf("Expected register value 0x%04X at address %d, got 0x%04X", 
				expectedValue, addr, value)
		}
	}
	
	// Test with an invalid quantity
	invalidQuantityReq := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncWriteMultipleRegisters,
		[]byte{0x00, 0x64, 0x00, 0x00, 0x00}, // Quantity = 0
	)
	
	_, err = handler.HandleWriteMultipleRegisters(ctx, invalidQuantityReq, store)
	if err == nil {
		t.Error("HandleWriteMultipleRegisters with quantity=0 should return error")
	}
	
	// Test with mismatched byte count
	mismatchedByteCountReq := test.NewMockRequest(
		1, // Transaction ID
		1, // Unit ID
		common.FuncWriteMultipleRegisters,
		[]byte{0x00, 0x64, 0x00, 0x02, 0x03, 0x12, 0x34, 0x56}, // Byte count 3 for 2 registers
	)
	
	_, err = handler.HandleWriteMultipleRegisters(ctx, mismatchedByteCountReq, store)
	if err == nil {
		t.Error("HandleWriteMultipleRegisters with mismatched byte count should return error")
	}
}