package protocol

import (
	"encoding/binary"
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
)

func TestGenerateReadCoilsRequest(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test valid request
	address := common.Address(100)
	quantity := common.Quantity(10)
	
	data, err := handler.GenerateReadCoilsRequest(address, quantity)
	if err != nil {
		t.Errorf("GenerateReadCoilsRequest returned error: %v", err)
	}
	
	if len(data) != 4 {
		t.Errorf("GenerateReadCoilsRequest: expected data length 4, got %d", len(data))
	}
	
	// Check address field
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("GenerateReadCoilsRequest: expected address %d, got %d", address, addr)
	}
	
	// Check quantity field
	quant := binary.BigEndian.Uint16(data[2:4])
	if quant != uint16(quantity) {
		t.Errorf("GenerateReadCoilsRequest: expected quantity %d, got %d", quantity, quant)
	}
	
	// Test invalid quantity
	_, err = handler.GenerateReadCoilsRequest(address, 0)
	if err == nil {
		t.Error("GenerateReadCoilsRequest with quantity=0 should return error")
	}
	
	_, err = handler.GenerateReadCoilsRequest(address, common.MaxCoilCount+1)
	if err == nil {
		t.Error("GenerateReadCoilsRequest with quantity > MaxCoilCount should return error")
	}
}

func TestGenerateReadDiscreteInputsRequest(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test valid request
	address := common.Address(100)
	quantity := common.Quantity(10)
	
	data, err := handler.GenerateReadDiscreteInputsRequest(address, quantity)
	if err != nil {
		t.Errorf("GenerateReadDiscreteInputsRequest returned error: %v", err)
	}
	
	if len(data) != 4 {
		t.Errorf("GenerateReadDiscreteInputsRequest: expected data length 4, got %d", len(data))
	}
	
	// Check address field
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("GenerateReadDiscreteInputsRequest: expected address %d, got %d", address, addr)
	}
	
	// Check quantity field
	quant := binary.BigEndian.Uint16(data[2:4])
	if quant != uint16(quantity) {
		t.Errorf("GenerateReadDiscreteInputsRequest: expected quantity %d, got %d", quantity, quant)
	}
}

func TestGenerateReadHoldingRegistersRequest(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test valid request
	address := common.Address(100)
	quantity := common.Quantity(10)
	
	data, err := handler.GenerateReadHoldingRegistersRequest(address, quantity)
	if err != nil {
		t.Errorf("GenerateReadHoldingRegistersRequest returned error: %v", err)
	}
	
	if len(data) != 4 {
		t.Errorf("GenerateReadHoldingRegistersRequest: expected data length 4, got %d", len(data))
	}
	
	// Check address field
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("GenerateReadHoldingRegistersRequest: expected address %d, got %d", address, addr)
	}
	
	// Check quantity field
	quant := binary.BigEndian.Uint16(data[2:4])
	if quant != uint16(quantity) {
		t.Errorf("GenerateReadHoldingRegistersRequest: expected quantity %d, got %d", quantity, quant)
	}
	
	// Test invalid quantity
	_, err = handler.GenerateReadHoldingRegistersRequest(address, 0)
	if err == nil {
		t.Error("GenerateReadHoldingRegistersRequest with quantity=0 should return error")
	}
	
	_, err = handler.GenerateReadHoldingRegistersRequest(address, common.MaxRegisterCount+1)
	if err == nil {
		t.Error("GenerateReadHoldingRegistersRequest with quantity > MaxRegisterCount should return error")
	}
}

func TestParseReadCoilsResponse(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test valid response
	quantity := common.Quantity(10)
	byteCount := 2 // Ceiling of 10/8
	
	// Create a response with some coils on and some off
	responseData := []byte{byte(byteCount), 0b10101010, 0b00000011}
	
	values, err := handler.ParseReadCoilsResponse(responseData, quantity)
	if err != nil {
		t.Errorf("ParseReadCoilsResponse returned error: %v", err)
	}
	
	if len(values) != int(quantity) {
		t.Errorf("ParseReadCoilsResponse: expected %d values, got %d", quantity, len(values))
	}
	
	// Check first byte values (alternating true/false)
	expectedValues := []bool{false, true, false, true, false, true, false, true, true, true}
	for i, expected := range expectedValues {
		if values[i] != expected {
			t.Errorf("ParseReadCoilsResponse: value at index %d, expected %t, got %t", 
				i, expected, values[i])
		}
	}
	
	// Test invalid responses
	// Empty response
	_, err = handler.ParseReadCoilsResponse([]byte{}, quantity)
	if err == nil {
		t.Error("ParseReadCoilsResponse with empty data should return error")
	}
	
	// Wrong byte count
	_, err = handler.ParseReadCoilsResponse([]byte{3, 0, 0, 0}, quantity)
	if err == nil {
		t.Error("ParseReadCoilsResponse with incorrect byte count should return error")
	}
	
	// Data too short
	_, err = handler.ParseReadCoilsResponse([]byte{2, 0}, quantity)
	if err == nil {
		t.Error("ParseReadCoilsResponse with data too short should return error")
	}
}

func TestGenerateWriteSingleCoilRequest(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test ON value
	address := common.Address(100)
	value := common.CoilValue(true)
	
	data, err := handler.GenerateWriteSingleCoilRequest(address, value)
	if err != nil {
		t.Errorf("GenerateWriteSingleCoilRequest returned error: %v", err)
	}
	
	if len(data) != 4 {
		t.Errorf("GenerateWriteSingleCoilRequest: expected data length 4, got %d", len(data))
	}
	
	// Check address field
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("GenerateWriteSingleCoilRequest: expected address %d, got %d", address, addr)
	}
	
	// Check value field (ON = 0xFF00)
	val := binary.BigEndian.Uint16(data[2:4])
	if val != common.CoilOnU16 {
		t.Errorf("GenerateWriteSingleCoilRequest: expected value 0xFF00, got 0x%04X", val)
	}
	
	// Test OFF value
	value = common.CoilValue(false)
	
	data, err = handler.GenerateWriteSingleCoilRequest(address, value)
	if err != nil {
		t.Errorf("GenerateWriteSingleCoilRequest returned error: %v", err)
	}
	
	// Check value field (OFF = 0x0000)
	val = binary.BigEndian.Uint16(data[2:4])
	if val != common.CoilOffU16 {
		t.Errorf("GenerateWriteSingleCoilRequest: expected value 0x0000, got 0x%04X", val)
	}
}

func TestParseWriteSingleCoilResponse(t *testing.T) {
	handler := NewProtocolHandler()
	
	// Test valid ON response
	address := common.Address(100)
	value := common.CoilValue(true)
	
	// Create response data - should echo the request
	responseData := make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], common.CoilOnU16)
	
	respAddress, respValue, err := handler.ParseWriteSingleCoilResponse(responseData)
	if err != nil {
		t.Errorf("ParseWriteSingleCoilResponse returned error: %v", err)
	}
	
	if respAddress != address {
		t.Errorf("ParseWriteSingleCoilResponse: expected address %d, got %d", address, respAddress)
	}
	
	if respValue != value {
		t.Errorf("ParseWriteSingleCoilResponse: expected value %t, got %t", value, respValue)
	}
	
	// Test valid OFF response
	value = common.CoilValue(false)
	
	// Create response data - should echo the request
	responseData = make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], common.CoilOffU16)
	
	respAddress, respValue, err = handler.ParseWriteSingleCoilResponse(responseData)
	if err != nil {
		t.Errorf("ParseWriteSingleCoilResponse returned error: %v", err)
	}
	
	if respValue != value {
		t.Errorf("ParseWriteSingleCoilResponse: expected value %t, got %t", value, respValue)
	}
	
	// Test invalid responses
	// Data too short
	_, _, err = handler.ParseWriteSingleCoilResponse([]byte{0, 0})
	if err == nil {
		t.Error("ParseWriteSingleCoilResponse with data too short should return error")
	}
	
	// Invalid value
	responseData = make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], 0x1234) // Neither ON nor OFF
	
	_, _, err = handler.ParseWriteSingleCoilResponse(responseData)
	if err == nil {
		t.Error("ParseWriteSingleCoilResponse with invalid value should return error")
	}
}

func TestGenerateWriteSingleRegisterRequest(t *testing.T) {
	handler := NewProtocolHandler()
	
	address := common.Address(100)
	value := common.RegisterValue(12345)
	
	data, err := handler.GenerateWriteSingleRegisterRequest(address, value)
	if err != nil {
		t.Errorf("GenerateWriteSingleRegisterRequest returned error: %v", err)
	}
	
	if len(data) != 4 {
		t.Errorf("GenerateWriteSingleRegisterRequest: expected data length 4, got %d", len(data))
	}
	
	// Check address field
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("GenerateWriteSingleRegisterRequest: expected address %d, got %d", address, addr)
	}
	
	// Check value field
	val := binary.BigEndian.Uint16(data[2:4])
	if val != uint16(value) {
		t.Errorf("GenerateWriteSingleRegisterRequest: expected value %d, got %d", value, val)
	}
}

func TestProtocolHandler_WithLogger(t *testing.T) {
	// Create a protocol handler with a custom logger
	logger := logging.NewLogger()
	handler := NewProtocolHandler(WithLogger(logger))
	
	// Create a new handler with a different logger
	newLogger := logging.NewLogger()
	newHandler := handler.WithLogger(newLogger)
	
	if newHandler == handler {
		t.Error("WithLogger returned the same instance - expected a new instance")
	}
	
	// Test that the new handler works correctly
	address := common.Address(100)
	quantity := common.Quantity(10)
	
	data, err := newHandler.GenerateReadCoilsRequest(address, quantity)
	if err != nil {
		t.Errorf("New handler's GenerateReadCoilsRequest returned error: %v", err)
	}
	
	// Verify the data is correct
	addr := binary.BigEndian.Uint16(data[0:2])
	if addr != uint16(address) {
		t.Errorf("New handler's request: expected address %d, got %d", address, addr)
	}
}