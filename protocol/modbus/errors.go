package modbus

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by the client, server, and protocol layers.
var (
	// ErrNotConnected is returned when an operation is attempted on a
	// connection that has not been established.
	ErrNotConnected = errors.New("client not connected")

	// ErrAlreadyConnected is returned when Connect is called on a
	// connection that is already active.
	ErrAlreadyConnected = errors.New("client already connected")

	// ErrInvalidQuantity is returned when the requested number of coils or
	// registers is outside the range allowed by the Modbus specification.
	ErrInvalidQuantity = errors.New("invalid quantity")

	// ErrInvalidAddress is returned when a register or coil address is
	// outside the valid 0-65535 range.
	ErrInvalidAddress = errors.New("invalid address")

	// ErrInvalidResponseLength is returned when a response PDU has an
	// unexpected byte count or overall length.
	ErrInvalidResponseLength = errors.New("invalid response length")

	// ErrInvalidCRC is returned when an RTU-mode CRC check fails.
	ErrInvalidCRC = errors.New("invalid CRC")

	// ErrInvalidFunction is returned when a request contains an
	// unsupported or unrecognised function code.
	ErrInvalidFunction = errors.New("invalid function code")

	// ErrInvalidValue is returned when a request or response contains a
	// value that does not conform to the Modbus specification.
	ErrInvalidValue = errors.New("invalid value")

	// ErrInvalidResponseFormat is returned when a response PDU cannot be
	// parsed because its structure does not match the expected format.
	ErrInvalidResponseFormat = errors.New("invalid response format")

	// ErrTimeout is returned when a network operation exceeds its deadline.
	ErrTimeout = errors.New("timeout")

	// ErrContextCanceled is returned when a context is cancelled before
	// the operation completes.
	ErrContextCanceled = errors.New("context canceled")

	// ErrInvalidProtocolHeader is returned when the MBAP header protocol
	// identifier is not the expected Modbus TCP value (0x0000).
	ErrInvalidProtocolHeader = errors.New("invalid protocol header")

	// ErrTooManyRegisters is returned when a request exceeds the per-
	// transaction register limit (125 for reads, 123 for writes).
	ErrTooManyRegisters = errors.New("too many registers requested")

	// ErrTooManyCoils is returned when a request exceeds the per-
	// transaction coil limit (2000 for reads, 1968 for writes).
	ErrTooManyCoils = errors.New("too many coils requested")

	// ErrEmptyResponse is returned when the server sends a response with
	// no data bytes.
	ErrEmptyResponse = errors.New("empty response")

	// ErrResponseTooLarge is returned when a response exceeds the maximum
	// ADU size.
	ErrResponseTooLarge = errors.New("response too large")

	// ErrRequestTooLarge is returned when a request exceeds the maximum
	// ADU size.
	ErrRequestTooLarge = errors.New("request too large")

	// ErrTransactionTimeout is returned when a transaction's response is
	// not received within the configured timeout.
	ErrTransactionTimeout = errors.New("transaction timeout")

	// ErrTransportClosing is returned when the underlying transport is
	// shutting down and can no longer process transactions.
	ErrTransportClosing = errors.New("transport closing")

	// ErrServerDeviceFailure is returned when the server encounters an
	// unrecoverable internal error (Modbus exception code 0x04).
	ErrServerDeviceFailure = errors.New("server device failure")

	// ErrNoResponse is returned when no response is received from the
	// server after all retry attempts.
	ErrNoResponse = errors.New("no response from server")
)

// ModbusError represents an exception response from a Modbus server. When a
// server cannot fulfil a request it replies with the original function code
// (with the high bit set) and an [ExceptionCode] indicating the reason. Use
// [IsModbusError] to test whether an error is a protocol-level exception.
type ModbusError struct {
	FunctionCode  FunctionCode  // Function code from the request (with exception bit set)
	ExceptionCode ExceptionCode // Exception code indicating the error reason
}

// Error implements the error interface.
func (e *ModbusError) Error() string {
	return fmt.Sprintf("modbus: exception response: function: %s, exception code: %#x (%s)",
		e.FunctionCode, e.ExceptionCode, GetExceptionString(e.ExceptionCode))
}

// IsModbusError reports whether err is a [ModbusError] (a protocol-level
// exception from the server).
func IsModbusError(err error) bool {
	_, ok := err.(*ModbusError)
	return ok
}

// IsExceptionError reports whether err is a [ModbusError] with the given
// exception code.
func IsExceptionError(err error, exceptionCode ExceptionCode) bool {
	if modbusErr, ok := err.(*ModbusError); ok {
		return modbusErr.ExceptionCode == exceptionCode
	}
	return false
}

// IsFunctionNotSupportedError reports whether err is a [ModbusError] with
// exception code 0x01 (function code not supported).
func IsFunctionNotSupportedError(err error) bool {
	return IsExceptionError(err, ExceptionFunctionCodeNotSupported)
}

// NewModbusError creates a new [ModbusError] with the given function and
// exception codes.
func NewModbusError(functionCode FunctionCode, exceptionCode ExceptionCode) *ModbusError {
	return &ModbusError{
		FunctionCode:  functionCode,
		ExceptionCode: exceptionCode,
	}
}

// GetExceptionString returns a human-readable description of an [ExceptionCode].
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
