package common

import (
	"errors"
	"fmt"
)

// Common errors
var (
	// Client state errors
	ErrNotConnected     = errors.New("client not connected")
	ErrAlreadyConnected = errors.New("client already connected")

	// Protocol constraint errors (related to Modbus specification)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (Function Codes) - Various constraints
	ErrInvalidQuantity  = errors.New("invalid quantity") // Quantity constraints from spec
	ErrInvalidAddress   = errors.New("invalid address")  // Address range constraints from spec

	// Protocol format errors
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
	ErrInvalidResponseLength = errors.New("invalid response length") // Packet length issues
	ErrInvalidCRC            = errors.New("invalid CRC")             // For RTU mode

	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (MODBUS Function Codes)
	ErrInvalidFunction       = errors.New("invalid function code") // Unsupported function code

	ErrInvalidValue          = errors.New("invalid value")
	ErrInvalidResponseFormat = errors.New("invalid response format")

	// Communication errors
	ErrTimeout         = errors.New("timeout")
	ErrContextCanceled = errors.New("context canceled")

	// Protocol header errors
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
	ErrInvalidProtocolHeader = errors.New("invalid protocol header")

	// Request constraint errors
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Write Multiple Registers)
	ErrTooManyRegisters = errors.New("too many registers requested") // Max 125 registers per request

	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Write Multiple Coils)
	ErrTooManyCoils     = errors.New("too many coils requested")     // Max 2000 coils per request

	// Response errors
	ErrEmptyResponse     = errors.New("empty response")
	ErrResponseTooLarge  = errors.New("response too large")
	ErrRequestTooLarge   = errors.New("request too large")

	// Transaction errors
	ErrTransactionTimeout = errors.New("transaction timeout")
	ErrTransportClosing   = errors.New("transport closing")

	// Server errors
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
	ErrServerDeviceFailure = errors.New("server device failure") // Related to exception code 0x04
	ErrNoResponse          = errors.New("no response from server")
)

// ModbusError represents an error from a Modbus exception response
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
// "When a Client sends a request to a Server device, it expects a normal response.
// One of four possible events can occur from the Master's perspective:
// ..."
// "If the Server returns an Exception Response, the Exception Code field contains
// the reason why the Server is unable to process the requested function."
type ModbusError struct {
	FunctionCode  FunctionCode  // Function code from the request (with exception bit set)
	ExceptionCode ExceptionCode // Exception code indicating the error reason
}

// Error implements the error interface
func (e *ModbusError) Error() string {
	return fmt.Sprintf("modbus: exception response: function: %s, exception code: %#x (%s)",
		e.FunctionCode, e.ExceptionCode, GetExceptionString(e.ExceptionCode))
}

// IsModbusError checks if an error is a ModbusError
func IsModbusError(err error) bool {
	_, ok := err.(*ModbusError)
	return ok
}

// IsExceptionError checks if an error is a specific Modbus exception
func IsExceptionError(err error, exceptionCode ExceptionCode) bool {
	if modbusErr, ok := err.(*ModbusError); ok {
		return modbusErr.ExceptionCode == exceptionCode
	}
	return false
}

// IsFunctionNotSupportedError checks if an error is due to a function not being supported
func IsFunctionNotSupportedError(err error) bool {
	return IsExceptionError(err, ExceptionFunctionCodeNotSupported)
}

// NewModbusError creates a new ModbusError
func NewModbusError(functionCode FunctionCode, exceptionCode ExceptionCode) *ModbusError {
	return &ModbusError{
		FunctionCode:  functionCode,
		ExceptionCode: exceptionCode,
	}
}

// GetExceptionString returns a human-readable description of an exception code
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func GetExceptionString(exceptionCode ExceptionCode) string {
	switch exceptionCode {
	case ExceptionFunctionCodeNotSupported:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.1
		return "function code not supported"
	case ExceptionDataAddressNotAvailable:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.2
		return "data address not available"
	case ExceptionInvalidDataValue:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.3
		return "invalid data value"
	case ExceptionServerDeviceFailure:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.4
		return "server device failure"
	case ExceptionAcknowledge:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.5
		return "acknowledge"
	case ExceptionServerDeviceBusy:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.6
		return "server device busy"
	case ExceptionMemoryParityError:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.8
		return "memory parity error"
	case ExceptionGatewayPathUnavailable:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.9
		return "gateway path unavailable"
	case ExceptionGatewayTargetNoResponse:
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7.10
		return "gateway target no response"
	default:
		return fmt.Sprintf("unknown exception code: %#x", exceptionCode)
	}
}
