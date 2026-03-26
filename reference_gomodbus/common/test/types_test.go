package test

import (
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
)

func TestTypeAliases(t *testing.T) {
	// Test Address type
	var address common.Address = 100
	if uint16(address) != 100 {
		t.Errorf("Address conversion failed, expected 100, got %d", uint16(address))
	}

	// Test Quantity type
	var quantity common.Quantity = 10
	if uint16(quantity) != 10 {
		t.Errorf("Quantity conversion failed, expected 10, got %d", uint16(quantity))
	}

	// Test CoilValue type
	var coilValue common.CoilValue = true
	if bool(coilValue) != true {
		t.Errorf("CoilValue conversion failed, expected true, got %t", bool(coilValue))
	}

	// Test DiscreteInputValue type
	var discreteInputValue common.DiscreteInputValue = false
	if bool(discreteInputValue) != false {
		t.Errorf("DiscreteInputValue conversion failed, expected false, got %t", bool(discreteInputValue))
	}

	// Test RegisterValue type
	var registerValue common.RegisterValue = 12345
	if uint16(registerValue) != 12345 {
		t.Errorf("RegisterValue conversion failed, expected 12345, got %d", uint16(registerValue))
	}

	// Test InputRegisterValue type
	var inputRegisterValue common.InputRegisterValue = 54321
	if uint16(inputRegisterValue) != 54321 {
		t.Errorf("InputRegisterValue conversion failed, expected 54321, got %d", uint16(inputRegisterValue))
	}
}

func TestAddressArithmetic(t *testing.T) {
	// Test Address arithmetic
	var baseAddress common.Address = 100
	var offset common.Address = 50
	
	// Addition
	result := baseAddress + offset
	if result != 150 {
		t.Errorf("Address addition failed, expected 150, got %d", result)
	}
	
	// Addition with constant
	result = baseAddress + 25
	if result != 125 {
		t.Errorf("Address addition with constant failed, expected 125, got %d", result)
	}
	
	// Addition with different type that gets converted
	result = baseAddress + common.Address(10)
	if result != 110 {
		t.Errorf("Address addition with converted type failed, expected 110, got %d", result)
	}
}

func TestQuantityArithmetic(t *testing.T) {
	// Test Quantity arithmetic
	var baseQuantity common.Quantity = 100
	var offset common.Quantity = 50
	
	// Addition
	result := baseQuantity + offset
	if result != 150 {
		t.Errorf("Quantity addition failed, expected 150, got %d", result)
	}
	
	// Subtraction
	result = baseQuantity - offset
	if result != 50 {
		t.Errorf("Quantity subtraction failed, expected 50, got %d", result)
	}
	
	// Comparison
	if !(baseQuantity > offset) {
		t.Errorf("Quantity comparison failed, expected %d > %d", baseQuantity, offset)
	}
}

func TestFunctionCodeString(t *testing.T) {
	// Test FunctionCode.String() method
	testCases := []struct {
		code     common.FunctionCode
		expected string
	}{
		{common.FuncReadCoils, "ReadCoils"},
		{common.FuncReadDiscreteInputs, "ReadDiscreteInputs"},
		{common.FuncReadHoldingRegisters, "ReadHoldingRegisters"},
		{common.FuncReadInputRegisters, "ReadInputRegisters"},
		{common.FuncWriteSingleCoil, "WriteSingleCoil"},
		{common.FuncWriteSingleRegister, "WriteSingleRegister"},
		{common.FuncReadExceptionStatus, "ReadExceptionStatus"},
		{common.FuncWriteMultipleCoils, "WriteMultipleCoils"},
		{common.FuncWriteMultipleRegisters, "WriteMultipleRegisters"},
		{common.FuncReadWriteMultipleRegisters, "ReadWriteMultipleRegisters"},
		{common.FunctionCode(0x7F), "Unknown(0x7F)"}, // Unknown function code (not an exception)
	}

	for _, tc := range testCases {
		if tc.code.String() != tc.expected {
			t.Errorf("FunctionCode.String() for code 0x%02X, expected %s, got %s",
				byte(tc.code), tc.expected, tc.code.String())
		}
	}
}

func TestExceptionCodeString(t *testing.T) {
	// Test ExceptionCode.String() method
	testCases := []struct {
		code     common.ExceptionCode
		expected string
	}{
		{common.ExceptionFunctionCodeNotSupported, "FunctionCodeNotSupported"},
		{common.ExceptionDataAddressNotAvailable, "DataAddressNotAvailable"},
		{common.ExceptionInvalidDataValue, "InvalidDataValue"},
		{common.ExceptionServerDeviceFailure, "ServerDeviceFailure"},
		{common.ExceptionAcknowledge, "Acknowledge"},
		{common.ExceptionServerDeviceBusy, "ServerDeviceBusy"},
		{common.ExceptionMemoryParityError, "MemoryParityError"},
		{common.ExceptionGatewayPathUnavailable, "GatewayPathUnavailable"},
		{common.ExceptionGatewayTargetNoResponse, "GatewayTargetNoResponse"},
		{0xFF, "Unknown(0xFF)"}, // Unknown exception code
	}

	for _, tc := range testCases {
		if tc.code.String() != tc.expected {
			t.Errorf("ExceptionCode.String() for code 0x%02X, expected %s, got %s", 
				byte(tc.code), tc.expected, tc.code.String())
		}
	}
}

func TestExceptionFunctions(t *testing.T) {
	// Test IsException, IsFunctionException, GetOriginalFunctionCode, and GetOriginalFunction
	
	// Test normal function code
	normalCode := byte(common.FuncReadCoils)
	if common.IsException(normalCode) {
		t.Errorf("IsException() incorrectly identified 0x%02X as an exception", normalCode)
	}
	
	// Test exception function code
	exceptionCode := byte(common.FuncReadCoils) | common.ExceptionBit
	if !common.IsException(exceptionCode) {
		t.Errorf("IsException() failed to identify 0x%02X as an exception", exceptionCode)
	}
	
	// Test GetOriginalFunctionCode
	if common.GetOriginalFunctionCode(exceptionCode) != normalCode {
		t.Errorf("GetOriginalFunctionCode() failed, expected 0x%02X, got 0x%02X", 
			normalCode, common.GetOriginalFunctionCode(exceptionCode))
	}
	
	// Test IsFunctionException and GetOriginalFunction
	normalFuncCode := common.FuncReadCoils
	exceptionFuncCode := common.FunctionCode(exceptionCode)
	
	if common.IsFunctionException(normalFuncCode) {
		t.Errorf("IsFunctionException() incorrectly identified 0x%02X as an exception", normalFuncCode)
	}
	
	if !common.IsFunctionException(exceptionFuncCode) {
		t.Errorf("IsFunctionException() failed to identify 0x%02X as an exception", exceptionFuncCode)
	}
	
	if common.GetOriginalFunction(exceptionFuncCode) != normalFuncCode {
		t.Errorf("GetOriginalFunction() failed, expected 0x%02X, got 0x%02X", 
			normalFuncCode, common.GetOriginalFunction(exceptionFuncCode))
	}
}