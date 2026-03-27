package modbus

import "fmt"

// TransactionID is a 16-bit identifier in the MBAP header used to correlate
// Modbus TCP requests with their responses.
type TransactionID uint16

// ProtocolID is the protocol identifier field in the MBAP header. For Modbus
// TCP the value is always 0x0000 (see [TCPProtocolIdentifier]).
type ProtocolID uint16

// UnitID is the unit identifier (also called slave address) in the MBAP header.
// It selects the target device on multi-drop or gateway networks. For a direct
// TCP connection the value is typically 0 or 1.
type UnitID byte

// ExceptionCode is a one-byte code in a Modbus exception response indicating
// why the server could not process the request (e.g. illegal function, illegal
// data address).
type ExceptionCode byte

// FunctionCode is a one-byte code identifying the Modbus operation to perform
// (e.g. read coils, write registers). In exception responses the high bit
// (0x80) is set.
type FunctionCode byte

// Address is a 16-bit Modbus data address (0-65535) that identifies a specific
// coil, discrete input, holding register, or input register.
type Address uint16

// Quantity is the number of coils or registers to read or write in a single
// Modbus request. Maximum values depend on the function code (e.g. 125 for
// register reads, 2000 for coil reads).
type Quantity uint16

// CoilValue is a boolean representing the ON/OFF state of a single coil
// (a single-bit read/write output in a Modbus device).
type CoilValue = bool

// DiscreteInputValue is a boolean representing the ON/OFF state of a single
// discrete input (a single-bit read-only input in a Modbus device).
type DiscreteInputValue = bool

// RegisterValue is a 16-bit unsigned integer representing the value of a single
// holding register (a read/write data location in a Modbus device).
type RegisterValue = uint16

// InputRegisterValue is a 16-bit unsigned integer representing the value of a
// single input register (a read-only data location in a Modbus device).
type InputRegisterValue = uint16

// ExceptionStatus is the 8-bit bitmask returned by the Read Exception Status
// function (FC 0x07). Each bit represents a device-specific status coil.
type ExceptionStatus byte

// ReadDeviceIDCode selects which category of device identification objects to
// retrieve in a Read Device Identification request (FC 0x2B / MEI 0x0E).
type ReadDeviceIDCode byte

// DeviceIDObjectCode identifies a specific device identification object (e.g.
// vendor name = 0x00, product code = 0x01). Standard objects use codes
// 0x00-0x06; vendor-specific extended objects use 0x80-0xFF.
type DeviceIDObjectCode byte

// Standard Modbus function codes and exception codes.
const (
	// FuncReadCoils reads one or more coil outputs (FC 0x01).
	FuncReadCoils FunctionCode = 0x01
	// FuncReadDiscreteInputs reads one or more discrete inputs (FC 0x02).
	FuncReadDiscreteInputs FunctionCode = 0x02
	// FuncReadHoldingRegisters reads one or more holding registers (FC 0x03).
	FuncReadHoldingRegisters FunctionCode = 0x03
	// FuncReadInputRegisters reads one or more input registers (FC 0x04).
	FuncReadInputRegisters FunctionCode = 0x04
	// FuncWriteSingleCoil writes a single coil output (FC 0x05).
	FuncWriteSingleCoil FunctionCode = 0x05
	// FuncWriteSingleRegister writes a single holding register (FC 0x06).
	FuncWriteSingleRegister FunctionCode = 0x06
	// FuncReadExceptionStatus reads the eight exception status coils (FC 0x07).
	FuncReadExceptionStatus FunctionCode = 0x07
	// FuncWriteMultipleCoils writes a block of coil outputs (FC 0x0F).
	FuncWriteMultipleCoils FunctionCode = 0x0F
	// FuncWriteMultipleRegisters writes a block of holding registers (FC 0x10).
	FuncWriteMultipleRegisters FunctionCode = 0x10
	// FuncReadWriteMultipleRegisters atomically writes and reads holding registers (FC 0x17).
	FuncReadWriteMultipleRegisters FunctionCode = 0x17
	// FuncReadDeviceIdentification reads device ID objects via MEI transport (FC 0x2B).
	FuncReadDeviceIdentification FunctionCode = 0x2B

	// ExceptionFunctionCodeNotSupported indicates the function code is not supported (0x01).
	ExceptionFunctionCodeNotSupported ExceptionCode = 0x01
	// ExceptionDataAddressNotAvailable indicates the requested data address is out of range (0x02).
	ExceptionDataAddressNotAvailable ExceptionCode = 0x02
	// ExceptionInvalidDataValue indicates a value in the request data field is invalid (0x03).
	ExceptionInvalidDataValue ExceptionCode = 0x03
	// ExceptionServerDeviceFailure indicates an unrecoverable server error (0x04).
	ExceptionServerDeviceFailure ExceptionCode = 0x04
	// ExceptionAcknowledge indicates the server accepted the request but needs time to process it (0x05).
	ExceptionAcknowledge ExceptionCode = 0x05
	// ExceptionServerDeviceBusy indicates the server is busy processing another request (0x06).
	ExceptionServerDeviceBusy ExceptionCode = 0x06
	// ExceptionMemoryParityError indicates a memory parity error was detected (0x08).
	ExceptionMemoryParityError ExceptionCode = 0x08
	// ExceptionGatewayPathUnavailable indicates the gateway path is not available (0x0A).
	ExceptionGatewayPathUnavailable ExceptionCode = 0x0A
	// ExceptionGatewayTargetNoResponse indicates the target device did not respond via the gateway (0x0B).
	ExceptionGatewayTargetNoResponse ExceptionCode = 0x0B
)

// MEIType is the Modbus Encapsulated Interface sub-function type, used with
// function code 0x2B to select a specific encapsulated service.
type MEIType byte

// MEI type constants.
const (
	// MEIReadDeviceID is the MEI type for Read Device Identification (0x0E).
	MEIReadDeviceID MEIType = 0x0E
)

// Read Device ID access codes for the Read Device Identification function.
const (
	// ReadDeviceIDBasicStream requests the three mandatory basic objects
	// (vendor name, product code, revision) via stream access.
	ReadDeviceIDBasicStream ReadDeviceIDCode = 0x01
	// ReadDeviceIDRegularStream requests basic and regular identification
	// objects (through user application name) via stream access.
	ReadDeviceIDRegularStream ReadDeviceIDCode = 0x02
	// ReadDeviceIDExtendedStream requests all identification objects
	// including vendor-specific extended objects via stream access.
	ReadDeviceIDExtendedStream ReadDeviceIDCode = 0x03
	// ReadDeviceIDSpecificObject requests a single identification object
	// by its object ID via individual access.
	ReadDeviceIDSpecificObject ReadDeviceIDCode = 0x04

	// ReadDeviceIDBasic is an alias for [ReadDeviceIDBasicStream] (deprecated).
	ReadDeviceIDBasic = ReadDeviceIDBasicStream
	// ReadDeviceIDRegular is an alias for [ReadDeviceIDRegularStream] (deprecated).
	ReadDeviceIDRegular = ReadDeviceIDRegularStream
	// ReadDeviceIDExtended is an alias for [ReadDeviceIDExtendedStream] (deprecated).
	ReadDeviceIDExtended = ReadDeviceIDExtendedStream
	// ReadDeviceIDSpecific is an alias for [ReadDeviceIDSpecificObject] (deprecated).
	ReadDeviceIDSpecific = ReadDeviceIDSpecificObject
)

// ConformityLevel indicates which categories of device identification objects a
// device supports and whether individual access is available.
type ConformityLevel byte

// Conformity levels for device identification responses.
const (
	// ConformityLevelBasic indicates the device supports basic identification via stream access only.
	ConformityLevelBasic ConformityLevel = 0x01
	// ConformityLevelRegular indicates the device supports regular identification via stream access only.
	ConformityLevelRegular ConformityLevel = 0x02
	// ConformityLevelExtended indicates the device supports extended identification via stream access only.
	ConformityLevelExtended ConformityLevel = 0x03
	// ConformityLevelBasicIndividual indicates basic identification with both stream and individual access.
	ConformityLevelBasicIndividual ConformityLevel = 0x81
	// ConformityLevelRegularIndividual indicates regular identification with both stream and individual access.
	ConformityLevelRegularIndividual ConformityLevel = 0x82
	// ConformityLevelExtendedIndividual indicates extended identification with both stream and individual access.
	ConformityLevelExtendedIndividual ConformityLevel = 0x83
)

// String returns the string representation of a ConformityLevel.
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

// MoreFollows values for device identification responses.
const (
	// MoreFollowsNo indicates that no additional identification objects are available.
	MoreFollowsNo MoreFollows = 0x00
	// MoreFollowsYes indicates that more objects are available and should be
	// retrieved in a subsequent request starting at NextObjectID.
	MoreFollowsYes MoreFollows = 0xFF
)

// String returns the string representation of a MoreFollows value.
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

// Standard device identification object IDs. Objects 0x00-0x02 are mandatory
// basic objects; 0x03-0x06 are optional regular objects; 0x80-0xFF are
// vendor-specific extended objects.
const (
	// DeviceIDVendorName is the mandatory vendor name object (0x00).
	DeviceIDVendorName DeviceIDObjectCode = 0x00
	// DeviceIDProductCode is the mandatory product code object (0x01).
	DeviceIDProductCode DeviceIDObjectCode = 0x01
	// DeviceIDMajorMinorRevision is the mandatory revision object (0x02).
	DeviceIDMajorMinorRevision DeviceIDObjectCode = 0x02
	// DeviceIDVendorURL is the optional vendor URL object (0x03).
	DeviceIDVendorURL DeviceIDObjectCode = 0x03
	// DeviceIDProductName is the optional product name object (0x04).
	DeviceIDProductName DeviceIDObjectCode = 0x04
	// DeviceIDModelName is the optional model name object (0x05).
	DeviceIDModelName DeviceIDObjectCode = 0x05
	// DeviceIDUserAppName is the optional user application name object (0x06).
	DeviceIDUserAppName DeviceIDObjectCode = 0x06
)

// String returns the string representation of a FunctionCode.
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

// String returns the string representation of an ExceptionCode.
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

// String returns the string representation of a MEIType.
func (m MEIType) String() string {
	switch m {
	case MEIReadDeviceID:
		return "ReadDeviceIdentification"
	default:
		return fmt.Sprintf("UnknownMEIType(0x%02X)", byte(m))
	}
}

// String returns the string representation of a ReadDeviceIDCode.
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

// String returns the string representation of a DeviceIDObjectCode.
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

// String returns a string representation of the ExceptionStatus bitmask.
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

// Protocol-level constants for Modbus TCP framing and specification limits.
const (
	// TCPHeaderLength is the MBAP header size in bytes:
	// Transaction ID (2) + Protocol ID (2) + Length (2) + Unit ID (1).
	TCPHeaderLength = 7

	// MaxPDULength is the maximum Protocol Data Unit length (253 bytes).
	MaxPDULength = 253

	// MaxADULength is the maximum Application Data Unit length for Modbus TCP
	// (MBAP header + PDU = 260 bytes).
	MaxADULength = 260

	// DefaultTCPPort is the standard Modbus TCP port (502).
	DefaultTCPPort = 502

	// BytesPerCoil is the storage size for a single coil status. In multi-coil
	// responses the values are bit-packed (one coil per bit).
	BytesPerCoil = 1

	// BytesPerDiscreteInput is the storage size for a single discrete input
	// status. In multi-input responses the values are bit-packed.
	BytesPerDiscreteInput = 1

	// BytesPerRegister is the size of a single holding register (2 bytes, big-endian).
	BytesPerRegister = 2

	// BytesPerInputRegister is the size of a single input register (2 bytes, big-endian).
	BytesPerInputRegister = 2

	// MaxCoilCount is the maximum number of coils that can be read in a single
	// Read Coils or Read Discrete Inputs request (2000).
	MaxCoilCount = 2000

	// MaxWriteCoilCount is the maximum number of coils that can be written in a
	// single Write Multiple Coils request (1968).
	MaxWriteCoilCount = 1968

	// MaxRegisterCount is the maximum number of registers that can be read in a
	// single Read Holding Registers or Read Input Registers request (125).
	MaxRegisterCount = 125

	// MaxWriteRegisterCount is the maximum number of registers that can be
	// written in a single Write Multiple Registers request (123).
	MaxWriteRegisterCount = 123

	// MaxReadWriteReadCount is the maximum number of registers to read in a
	// Read/Write Multiple Registers request (125).
	MaxReadWriteReadCount = 125

	// MaxReadWriteWriteCount is the maximum number of registers to write in a
	// Read/Write Multiple Registers request (121).
	MaxReadWriteWriteCount = 121

	// CoilOnU16 is the 16-bit wire encoding for a coil in the ON state (0xFF00).
	CoilOnU16 = 0xFF00

	// CoilOffU16 is the 16-bit wire encoding for a coil in the OFF state (0x0000).
	CoilOffU16 = 0x0000
)

// TCPProtocolIdentifier is the MBAP protocol identifier for Modbus TCP (0x0000).
const TCPProtocolIdentifier = ProtocolID(0)

// ExceptionBit is the high bit (0x80) set in a function code to indicate that
// the response is a Modbus exception.
const ExceptionBit byte = 0x80

// IsException reports whether a raw function code byte has the exception bit set.
func IsException(functionCode byte) bool {
	return (functionCode & ExceptionBit) != 0
}

// IsFunctionException reports whether a [FunctionCode] has the exception bit set.
func IsFunctionException(functionCode FunctionCode) bool {
	return IsException(byte(functionCode))
}

// GetOriginalFunctionCode extracts the original function code from an exception
// response by clearing the high bit.
func GetOriginalFunctionCode(exceptionCode byte) byte {
	return exceptionCode & ^ExceptionBit
}

// GetOriginalFunction extracts the original [FunctionCode] from an exception
// response by clearing the high bit.
func GetOriginalFunction(exceptionCode FunctionCode) FunctionCode {
	return FunctionCode(GetOriginalFunctionCode(byte(exceptionCode)))
}
