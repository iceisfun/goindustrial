package common

import "fmt"

// TransactionID is a unique identifier for a transaction
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header), Field 1
type TransactionID uint16

// ProtocolID identifies the protocol used (e.g., Modbus TCP, RTU)
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header), Field 2
type ProtocolID uint16

// UnitID identifies a specific device on a Modbus network
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header), Field 4
type UnitID byte

// ExceptionCode represents an exception code in a Modbus response
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
type ExceptionCode byte

// FunctionCode represents a Modbus function code
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (MODBUS Function Codes)
type FunctionCode byte

// Address represents a Modbus address (coil, register, etc.)
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (MODBUS Data Model)
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.4 (Addressing Model - specifies 0-65535 range)
type Address uint16

// Quantity represents the number of coils or registers to read/write
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, e.g., Section 6.1 (Read Coils Request PDU defines "Quantity of Coils")
type Quantity uint16

// CoilValue alias represents a coil value
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Read Coils) and 6.5 (Write Single Coil)
type CoilValue = bool

// DiscreteInputValue alias represents a discrete input value
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.2 (Read Discrete Inputs)
type DiscreteInputValue = bool

// RegisterValue alias represents a holding register value
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Read Holding Registers)
type RegisterValue = uint16

// InputRegisterValue alias represents an input register value
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Read Input Registers)
type InputRegisterValue = uint16

// ExceptionStatus represents the return value from ReadExceptionStatus
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.7 (Read Exception Status)
type ExceptionStatus byte

// ReadDeviceIDCode represents a device identification access type
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Read Device Identification)
type ReadDeviceIDCode byte

// DeviceIDObjectCode represents a device identification object code
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Read Device Identification)
type DeviceIDObjectCode byte

// Function codes as defined by the Modbus specification
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (Function Codes)
const (
	// Standard function codes
	FuncReadCoils                  FunctionCode = 0x01 // Ref: Section 6.1
	FuncReadDiscreteInputs         FunctionCode = 0x02 // Ref: Section 6.2
	FuncReadHoldingRegisters       FunctionCode = 0x03 // Ref: Section 6.3
	FuncReadInputRegisters         FunctionCode = 0x04 // Ref: Section 6.4
	FuncWriteSingleCoil            FunctionCode = 0x05 // Ref: Section 6.5
	FuncWriteSingleRegister        FunctionCode = 0x06 // Ref: Section 6.6
	FuncReadExceptionStatus        FunctionCode = 0x07 // Ref: Section 6.7
	FuncWriteMultipleCoils         FunctionCode = 0x0F // Ref: Section 6.11
	FuncWriteMultipleRegisters     FunctionCode = 0x10 // Ref: Section 6.12
	FuncReadWriteMultipleRegisters FunctionCode = 0x17 // Ref: Section 6.17
	FuncReadDeviceIdentification   FunctionCode = 0x2B // MEI Transport, Ref: Section 6.21

	// Exception codes
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Codes)
	ExceptionFunctionCodeNotSupported ExceptionCode = 0x01 // Ref: Section 7.1
	ExceptionDataAddressNotAvailable  ExceptionCode = 0x02 // Ref: Section 7.2
	ExceptionInvalidDataValue         ExceptionCode = 0x03 // Ref: Section 7.3
	ExceptionServerDeviceFailure      ExceptionCode = 0x04 // Ref: Section 7.4
	ExceptionAcknowledge              ExceptionCode = 0x05 // Ref: Section 7.5
	ExceptionServerDeviceBusy         ExceptionCode = 0x06 // Ref: Section 7.6
	ExceptionMemoryParityError        ExceptionCode = 0x08 // Ref: Section 7.8
	ExceptionGatewayPathUnavailable   ExceptionCode = 0x0A // Ref: Section 7.9
	ExceptionGatewayTargetNoResponse  ExceptionCode = 0x0B // Ref: Section 7.10
)

// MEIType represents a Modbus Encapsulated Interface type
// Used in function code 0x2B (Modbus Encapsulated Interface)
// MEIType indicates which sub-function to execute
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21
type MEIType byte

// MEI Types
const (
	// MEIReadDeviceID is the MEI type for reading device identification (0x0E)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21
	MEIReadDeviceID MEIType = 0x0E

	// Other MEI types from the specification could be added here:
	// 0x0D - CANopen General Reference Request and Response PDU
	// All other values (0x00-0x0C, 0x0F-0xFF) are reserved.
)

// Read Device ID codes
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21
const (
	// ReadDeviceIDBasicStream requests basic device identification (stream access for objects 0x00-0x02)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 73
	ReadDeviceIDBasicStream ReadDeviceIDCode = 0x01
	// ReadDeviceIDRegularStream requests regular device identification (stream access through UserApplicationName)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 73
	ReadDeviceIDRegularStream ReadDeviceIDCode = 0x02
	// ReadDeviceIDExtendedStream requests extended device identification (stream access for all objects)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 73
	ReadDeviceIDExtendedStream ReadDeviceIDCode = 0x03
	// ReadDeviceIDSpecificObject requests a specific identification object (individual access)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 73
	ReadDeviceIDSpecificObject ReadDeviceIDCode = 0x04

	// Alias the old names for backwards compatibility
	ReadDeviceIDBasic    = ReadDeviceIDBasicStream
	ReadDeviceIDRegular  = ReadDeviceIDRegularStream
	ReadDeviceIDExtended = ReadDeviceIDExtendedStream
	ReadDeviceIDSpecific = ReadDeviceIDSpecificObject
)

// ConformityLevel indicates the device's identification conformity level
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 74
type ConformityLevel byte

// Conformity levels for device identification
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 74
const (
	// Stream access only
	ConformityLevelBasic    ConformityLevel = 0x01 // Basic identification (stream access only)
	ConformityLevelRegular  ConformityLevel = 0x02 // Regular identification (stream access only)
	ConformityLevelExtended ConformityLevel = 0x03 // Extended identification (stream access only)

	// Stream access and individual access
	ConformityLevelBasicIndividual    ConformityLevel = 0x81 // Basic identification (stream + individual access)
	ConformityLevelRegularIndividual  ConformityLevel = 0x82 // Regular identification (stream + individual access)
	ConformityLevelExtendedIndividual ConformityLevel = 0x83 // Extended identification (stream + individual access)
)

// String returns the string representation of a ConformityLevel
func (c ConformityLevel) String() string {
	switch c {
	case ConformityLevelBasic:
		return "Basic (stream)"
	case ConformityLevelRegular:
		return "Regular (stream)"
	case ConformityLevelExtended:
		return "Extended (stream)"
	case ConformityLevelBasicIndividual:
		return "Basic (stream+individual)"
	case ConformityLevelRegularIndividual:
		return "Regular (stream+individual)"
	case ConformityLevelExtendedIndividual:
		return "Extended (stream+individual)"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", byte(c))
	}
}

// MoreFollows indicates whether additional device identification objects are available
// in a subsequent request. Used in Read Device Identification (FC 0x2B/0x0E) responses.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU)
type MoreFollows byte

// MoreFollows values for device identification responses
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU)
const (
	MoreFollowsNo  MoreFollows = 0x00 // No more objects available
	MoreFollowsYes MoreFollows = 0xFF // More objects available, request again with NextObjectID
)

// String returns the string representation of a MoreFollows value
func (m MoreFollows) String() string {
	switch m {
	case MoreFollowsNo:
		return "No"
	case MoreFollowsYes:
		return "Yes"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", byte(m))
	}
}

// Device identification object IDs
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 72
const (
	// Basic identification objects (mandatory)
	DeviceIDVendorName         DeviceIDObjectCode = 0x00 // VendorName - Mandatory basic object
	DeviceIDProductCode        DeviceIDObjectCode = 0x01 // ProductCode - Mandatory basic object
	DeviceIDMajorMinorRevision DeviceIDObjectCode = 0x02 // Revision - Mandatory basic object

	// Regular identification objects (optional)
	DeviceIDVendorURL   DeviceIDObjectCode = 0x03 // VendorURL - Standard regular object
	DeviceIDProductName DeviceIDObjectCode = 0x04 // ProductName - Standard regular object
	DeviceIDModelName   DeviceIDObjectCode = 0x05 // ModelName - Standard regular object
	DeviceIDUserAppName DeviceIDObjectCode = 0x06 // UserApplicationName - Standard regular object

	// Private objects (vendor-specific)
	// Objects in the range 0x07-0x7F are reserved for future standard objects
	// Objects in the range 0x80-0xFF are vendor-specific extended objects
)

// String returns the string representation of a FunctionCode
func (f FunctionCode) String() string {
	switch f {
	case FuncReadCoils:
		return "ReadCoils"
	case FuncReadDiscreteInputs:
		return "ReadDiscreteInputs"
	case FuncReadHoldingRegisters:
		return "ReadHoldingRegisters"
	case FuncReadInputRegisters:
		return "ReadInputRegisters"
	case FuncWriteSingleCoil:
		return "WriteSingleCoil"
	case FuncWriteSingleRegister:
		return "WriteSingleRegister"
	case FuncReadExceptionStatus:
		return "ReadExceptionStatus"
	case FuncWriteMultipleCoils:
		return "WriteMultipleCoils"
	case FuncWriteMultipleRegisters:
		return "WriteMultipleRegisters"
	case FuncReadWriteMultipleRegisters:
		return "ReadWriteMultipleRegisters"
	case FuncReadDeviceIdentification:
		return "ReadDeviceIdentification"
	default:
		// If it's an exception response
		if IsException(byte(f)) {
			original := GetOriginalFunctionCode(byte(f))
			return fmt.Sprintf("Exception(%s)", FunctionCode(original).String())
		}
		return fmt.Sprintf("Unknown(0x%02X)", byte(f))
	}
}

func (e ExceptionCode) String() string {
	switch e {
	case ExceptionFunctionCodeNotSupported:
		return "FunctionCodeNotSupported"
	case ExceptionDataAddressNotAvailable:
		return "DataAddressNotAvailable"
	case ExceptionInvalidDataValue:
		return "InvalidDataValue"
	case ExceptionServerDeviceFailure:
		return "ServerDeviceFailure"
	case ExceptionAcknowledge:
		return "Acknowledge"
	case ExceptionServerDeviceBusy:
		return "ServerDeviceBusy"
	case ExceptionMemoryParityError:
		return "MemoryParityError"
	case ExceptionGatewayPathUnavailable:
		return "GatewayPathUnavailable"
	case ExceptionGatewayTargetNoResponse:
		return "GatewayTargetNoResponse"
	default:
		return fmt.Sprintf("Unknown(0x%02X)", byte(e))
	}
}

// String returns the string representation of a MEIType
func (m MEIType) String() string {
	switch m {
	case MEIReadDeviceID:
		return "ReadDeviceIdentification"
	default:
		return fmt.Sprintf("UnknownMEIType(0x%02X)", byte(m))
	}
}

// String returns the string representation of a ReadDeviceIDCode
func (c ReadDeviceIDCode) String() string {
	switch c {
	case ReadDeviceIDBasicStream:
		return "BasicStream"
	case ReadDeviceIDRegularStream:
		return "RegularStream"
	case ReadDeviceIDExtendedStream:
		return "ExtendedStream"
	case ReadDeviceIDSpecificObject:
		return "SpecificObject"
	default:
		return fmt.Sprintf("UnknownReadDeviceIDCode(0x%02X)", byte(c))
	}
}

// String returns the string representation of a DeviceIDObjectCode
func (c DeviceIDObjectCode) String() string {
	switch c {
	case DeviceIDVendorName:
		return "VendorName"
	case DeviceIDProductCode:
		return "ProductCode"
	case DeviceIDMajorMinorRevision:
		return "MajorMinorRevision"
	case DeviceIDVendorURL:
		return "VendorURL"
	case DeviceIDProductName:
		return "ProductName"
	case DeviceIDModelName:
		return "ModelName"
	case DeviceIDUserAppName:
		return "UserApplicationName"
	default:
		if c >= 0x80 {
			return fmt.Sprintf("ExtendedObject(0x%02X)", byte(c))
		}
		return fmt.Sprintf("UnknownObject(0x%02X)", byte(c))
	}
}

// String returns a string representation of the ExceptionStatus
func (s ExceptionStatus) String() string {
	// Since ExceptionStatus is a bit field (8 coils), show which bits are set
	var bits []int
	for i := 0; i < 8; i++ {
		if (s & (1 << i)) != 0 {
			bits = append(bits, i)
		}
	}

	if len(bits) == 0 {
		return "ExceptionStatus(None)"
	}

	return fmt.Sprintf("ExceptionStatus(Bits: %v, Value: 0x%02X)", bits, byte(s))
}

// Protocol-specific constants
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (Data Model)
const (
	// Modbus TCP
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
	TCPHeaderLength = 7   // Transaction ID (2) + Protocol ID (2) + Length (2) + Unit ID (1)
	MaxPDULength    = 253 // Maximum PDU length
	MaxADULength    = 260 // Maximum ADU length (TCP with header)
	DefaultTCPPort  = 502 // Default Modbus TCP port

	// Data sizes
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	// BytesPerCoil and BytesPerDiscreteInput refer to how individual statuses are packed,
	// not that each coil/input uses a full byte in a multi-item request/response.
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Read Coils Response - "coil status ... packed as one coil per bit")
	BytesPerCoil          = 1 // Represents a single status bit; multiple are packed.
	BytesPerDiscreteInput = 1 // Represents a single status bit; multiple are packed.
	BytesPerRegister      = 2 // Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Read Holding Registers Response - "Each register data in two bytes")
	BytesPerInputRegister = 2 // Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Read Input Registers Response)

	// Modbus limits
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.x (various function specific limits)
	MaxCoilCount            = 2000 // Maximum number of coils in Read Coils/Discrete Inputs (0x07D0), Ref: Section 6.1, 6.2
	MaxWriteCoilCount       = 1968 // Maximum number of coils in Write Multiple Coils (0x07B0), Ref: Section 6.11
	MaxRegisterCount        = 125  // Maximum number of registers in Read requests, Ref: Section 6.3, 6.4
	MaxWriteRegisterCount   = 123  // Maximum number of registers in Write Multiple Registers (0x007B), Ref: Section 6.12
	MaxReadWriteReadCount   = 125  // Maximum number of registers to read in Read/Write Multiple (0x007D), Ref: Section 6.17
	MaxReadWriteWriteCount  = 121  // Maximum number of registers to write in Read/Write Multiple (0x0079), Ref: Section 6.17

	// Coil Values as defined in the Modbus specification
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Write Single Coil)
	//
	// "The requested ON/OFF state is specified by a constant in the Coil Value field.
	// A value of 0xFF00 requests the coil to be ON.
	// A value of 0x0000 requests the coil to be OFF.
	// All other values are illegal and will not affect the coil."
	//
	CoilOnU16  = 0xFF00 // ON value for coils in register format
	CoilOffU16 = 0x0000 // OFF value for coils in register format
)

// TCPProtocolIdentifier is the standard identifier for Modbus TCP
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1
const TCPProtocolIdentifier = ProtocolID(0)

// ExceptionBit is the bit that is set in the function code to indicate an exception response
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
const ExceptionBit byte = 0x80

// IsException checks if a function code represents an exception
func IsException(functionCode byte) bool {
	return (functionCode & ExceptionBit) != 0
}

// IsFunctionException checks if a FunctionCode represents an exception
func IsFunctionException(functionCode FunctionCode) bool {
	return IsException(byte(functionCode))
}

// GetOriginalFunctionCode extracts the original function code from an exception
func GetOriginalFunctionCode(exceptionCode byte) byte {
	return exceptionCode & ^ExceptionBit
}

// GetOriginalFunction extracts the original FunctionCode from an exception
func GetOriginalFunction(exceptionCode FunctionCode) FunctionCode {
	return FunctionCode(GetOriginalFunctionCode(byte(exceptionCode)))
}
