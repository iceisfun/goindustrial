package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/iceisfun/goindustrial/logging"
)

// ProtocolHandler implements Modbus protocol encoding and decoding. It generates
// request PDU payloads and parses response PDU payloads for every supported
// function code.
type ProtocolHandler struct {
	logger logging.Logger
}

// ProtocolOption is a functional option for configuring a [ProtocolHandler].
type ProtocolOption func(*ProtocolHandler)

// WithProtocolLogger sets the logger for the protocol handler.
func WithProtocolLogger(logger logging.Logger) ProtocolOption {
	return func(p *ProtocolHandler) {
		p.logger = logger
	}
}

// NewProtocolHandler creates a new ProtocolHandler with the given options.
func NewProtocolHandler(options ...ProtocolOption) *ProtocolHandler {
	handler := &ProtocolHandler{
		logger: logging.NewNopLogger(), // Default logger
	}

	// Apply options
	for _, option := range options {
		option(handler)
	}

	return handler
}

// generateReadRequest is a helper function for generating read requests that follow the same pattern
// (read coils, read discrete inputs, read holding registers, read input registers)
func (h *ProtocolHandler) generateReadRequest(itemType string, address Address, quantity Quantity, maxQuantity Quantity) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read %s request: address=%d, quantity=%d", itemType, address, quantity)

	if quantity == 0 || quantity > maxQuantity {
		h.logger.Error(ctx, "Invalid quantity for read %s request: %d (max %d)", itemType, quantity, maxQuantity)
		return nil, ErrInvalidQuantity
	}

	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], uint16(quantity))

	h.logger.Debug(ctx, "Generated read %s request data: %v", itemType, data)
	return data, nil
}

// parseBitResponse is a helper function for parsing responses that contain bit values
// (coils and discrete inputs)
func (h *ProtocolHandler) parseBitResponse(itemType string, data []byte, quantity Quantity) ([]bool, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read %s response: data=%v, quantity=%d", itemType, data, quantity)

	if len(data) == 0 {
		h.logger.Error(ctx, "Empty response for read %s", itemType)
		return nil, ErrEmptyResponse
	}

	// First byte is the byte count
	byteCount := int(data[0])
	if len(data) != byteCount+1 {
		h.logger.Error(ctx, "Invalid response length for read %s: expected %d, got %d",
			itemType, byteCount+1, len(data))
		return nil, ErrInvalidResponseLength
	}

	// Calculate the expected byte count
	expectedByteCount := int(math.Ceil(float64(quantity) / 8.0))
	if byteCount != expectedByteCount {
		h.logger.Error(ctx, "Invalid byte count for read %s: expected %d, got %d",
			itemType, expectedByteCount, byteCount)
		return nil, ErrInvalidResponseLength
	}

	// Parse the values
	values := make([]bool, quantity)
	for i := 0; i < int(quantity); i++ {
		byteIndex := i / 8
		bitIndex := i % 8
		byteValue := data[1+byteIndex]
		values[i] = ((byteValue >> uint(bitIndex)) & 0x01) == 1
	}

	h.logger.Debug(ctx, "Parsed %d %s values", len(values), itemType)
	return values, nil
}

// parseRegisterResponse is a helper function for parsing responses that contain register values
// (holding registers and input registers)
func (h *ProtocolHandler) parseRegisterResponse(itemType string, data []byte, quantity Quantity) ([]uint16, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read %s response: data=%v, quantity=%d", itemType, data, quantity)

	if len(data) == 0 {
		h.logger.Error(ctx, "Empty response for read %s", itemType)
		return nil, ErrEmptyResponse
	}

	// First byte is the byte count
	byteCount := int(data[0])
	if len(data) != byteCount+1 {
		h.logger.Error(ctx, "Invalid response length for read %s: expected %d, got %d",
			itemType, byteCount+1, len(data))
		return nil, ErrInvalidResponseLength
	}

	// Calculate the expected byte count
	expectedByteCount := int(quantity) * 2
	if byteCount != expectedByteCount {
		h.logger.Error(ctx, "Invalid byte count for read %s: expected %d, got %d",
			itemType, expectedByteCount, byteCount)
		return nil, ErrInvalidResponseLength
	}

	// Parse the values
	values := make([]uint16, quantity)
	for i := 0; i < int(quantity); i++ {
		values[i] = binary.BigEndian.Uint16(data[1+i*2 : 1+i*2+2])
	}

	h.logger.Debug(ctx, "Parsed %d %s values", len(values), itemType)
	return values, nil
}

// GenerateReadCoilsRequest generates the PDU data for a Read Coils request
// (function code 0x01). Quantity must be between 1 and 2000.
func (h *ProtocolHandler) GenerateReadCoilsRequest(address Address, quantity Quantity) ([]byte, error) {
	return h.generateReadRequest("coils", address, quantity, MaxCoilCount)
}

// ParseReadCoilsResponse parses the PDU data from a Read Coils response
// (function code 0x01) and returns the coil values as booleans.
func (h *ProtocolHandler) ParseReadCoilsResponse(data []byte, quantity Quantity) ([]CoilValue, error) {
	// Use the parseBitResponse helper and cast the result to the expected type
	values, err := h.parseBitResponse("coils", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []bool to []CoilValue (type alias, so this is a no-op in Go)
	coilValues := make([]CoilValue, len(values))
	for i, v := range values {
		coilValues[i] = CoilValue(v)
	}

	return coilValues, nil
}

// GenerateReadDiscreteInputsRequest generates the PDU data for a Read Discrete
// Inputs request (function code 0x02). Quantity must be between 1 and 2000.
func (h *ProtocolHandler) GenerateReadDiscreteInputsRequest(address Address, quantity Quantity) ([]byte, error) {
	return h.generateReadRequest("discrete inputs", address, quantity, MaxCoilCount)
}

// ParseReadDiscreteInputsResponse parses the PDU data from a Read Discrete
// Inputs response (function code 0x02) and returns the input values as booleans.
func (h *ProtocolHandler) ParseReadDiscreteInputsResponse(data []byte, quantity Quantity) ([]DiscreteInputValue, error) {
	// Use the parseBitResponse helper and cast the result to the expected type
	values, err := h.parseBitResponse("discrete inputs", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []bool to []DiscreteInputValue (type alias, so this is a no-op in Go)
	discreteValues := make([]DiscreteInputValue, len(values))
	for i, v := range values {
		discreteValues[i] = DiscreteInputValue(v)
	}

	return discreteValues, nil
}

// GenerateReadHoldingRegistersRequest generates the PDU data for a Read Holding
// Registers request (function code 0x03). Quantity must be between 1 and 125.
func (h *ProtocolHandler) GenerateReadHoldingRegistersRequest(address Address, quantity Quantity) ([]byte, error) {
	return h.generateReadRequest("holding registers", address, quantity, MaxRegisterCount)
}

// ParseReadHoldingRegistersResponse parses the PDU data from a Read Holding
// Registers response (function code 0x03) and returns the register values.
func (h *ProtocolHandler) ParseReadHoldingRegistersResponse(data []byte, quantity Quantity) ([]RegisterValue, error) {
	// Use the parseRegisterResponse helper and cast the result to the expected type
	values, err := h.parseRegisterResponse("holding registers", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []uint16 to []RegisterValue (type alias, so this is a no-op in Go)
	registerValues := make([]RegisterValue, len(values))
	for i, v := range values {
		registerValues[i] = RegisterValue(v)
	}

	return registerValues, nil
}

// GenerateReadInputRegistersRequest generates the PDU data for a Read Input
// Registers request (function code 0x04). Quantity must be between 1 and 125.
func (h *ProtocolHandler) GenerateReadInputRegistersRequest(address Address, quantity Quantity) ([]byte, error) {
	return h.generateReadRequest("input registers", address, quantity, MaxRegisterCount)
}

// ParseReadInputRegistersResponse parses the PDU data from a Read Input
// Registers response (function code 0x04) and returns the register values.
func (h *ProtocolHandler) ParseReadInputRegistersResponse(data []byte, quantity Quantity) ([]InputRegisterValue, error) {
	// Use the parseRegisterResponse helper and cast the result to the expected type
	values, err := h.parseRegisterResponse("input registers", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []uint16 to []InputRegisterValue (type alias, so this is a no-op in Go)
	inputValues := make([]InputRegisterValue, len(values))
	for i, v := range values {
		inputValues[i] = InputRegisterValue(v)
	}

	return inputValues, nil
}

// GenerateWriteSingleCoilRequest generates the PDU data for a Write Single Coil
// request (function code 0x05). The boolean value is encoded as 0xFF00 (ON) or
// 0x0000 (OFF) per the Modbus specification.
func (h *ProtocolHandler) GenerateWriteSingleCoilRequest(address Address, value CoilValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write single coil request: address=%d, value=%t", address, value)

	data := make([]byte, 4)
	// Write address in big-endian format (most significant byte first)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))

	// Encode boolean as 0xFF00 (on) or 0x0000 (off)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
	// The requested ON/OFF state is specified by a constant in the Coil Value field.
	// A value of 0xFF00 requests the coil to be ON. A value of 0x0000 requests the coil to be OFF.
	if value {
		binary.BigEndian.PutUint16(data[2:4], CoilOnU16)
	} else {
		binary.BigEndian.PutUint16(data[2:4], CoilOffU16)
	}

	h.logger.Debug(ctx, "Generated write single coil request data: %v", data)
	return data, nil
}

// ParseWriteSingleCoilResponse parses the echo response from a Write Single
// Coil request (function code 0x05) and returns the confirmed address and value.
func (h *ProtocolHandler) ParseWriteSingleCoilResponse(data []byte) (Address, CoilValue, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write single coil response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write single coil: expected 4, got %d", len(data))
		return 0, false, ErrInvalidResponseLength
	}

	// Parse address from big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := Address(binary.BigEndian.Uint16(data[0:2]))
	value := binary.BigEndian.Uint16(data[2:4])

	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
	// The normal response is an echo of the request
	switch value {
	case CoilOnU16:
		h.logger.Debug(ctx, "Parsed write single coil response: address=%d, value=true", address)
		return address, true, nil
	case CoilOffU16:
		h.logger.Debug(ctx, "Parsed write single coil response: address=%d, value=false", address)
		return address, false, nil
	default:
		h.logger.Error(ctx, "Invalid coil value in response: %d", value)
		return address, false, fmt.Errorf("invalid coil value: %d", value)
	}
}

// GenerateWriteSingleRegisterRequest generates the PDU data for a Write Single
// Register request (function code 0x06).
func (h *ProtocolHandler) GenerateWriteSingleRegisterRequest(address Address, value RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write single register request: address=%d, value=%d", address, value)

	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], value)

	h.logger.Debug(ctx, "Generated write single register request data: %v", data)
	return data, nil
}

// ParseWriteSingleRegisterResponse parses the echo response from a Write Single
// Register request (function code 0x06) and returns the confirmed address and value.
func (h *ProtocolHandler) ParseWriteSingleRegisterResponse(data []byte) (Address, RegisterValue, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write single register response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write single register: expected 4, got %d", len(data))
		return 0, 0, ErrInvalidResponseLength
	}

	address := Address(binary.BigEndian.Uint16(data[0:2]))
	value := RegisterValue(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write single register response: address=%d, value=%d", address, value)
	return address, value, nil
}

// GenerateWriteMultipleCoilsRequest generates the PDU data for a Write Multiple
// Coils request (function code 0x0F). Up to 1968 coils may be written. The
// boolean values are packed into bytes with the LSB of the first byte
// corresponding to the lowest coil address.
func (h *ProtocolHandler) GenerateWriteMultipleCoilsRequest(address Address, values []CoilValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write multiple coils request: address=%d, count=%d",
		address, len(values))

	if len(values) == 0 || len(values) > MaxWriteCoilCount {
		h.logger.Error(ctx, "Invalid quantity for write multiple coils request: %d", len(values))
		return nil, ErrInvalidQuantity
	}

	// Calculate byte count and allocate data
	byteCount := int(math.Ceil(float64(len(values)) / 8.0))
	data := make([]byte, 5+byteCount)

	// Address - in big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	// Quantity - in big-endian format
	binary.BigEndian.PutUint16(data[2:4], uint16(len(values)))
	// Byte count
	data[4] = byte(byteCount)

	// Pack coil values - LSB of first byte is the lowest coil address
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
	for i, value := range values {
		byteIndex := i / 8
		bitIndex := i % 8

		if value {
			data[5+byteIndex] |= (1 << uint(bitIndex))
		}
	}

	h.logger.Debug(ctx, "Generated write multiple coils request data: %v", data)
	return data, nil
}

// ParseWriteMultipleCoilsResponse parses the response from a Write Multiple
// Coils request (function code 0x0F) and returns the confirmed starting address
// and quantity.
func (h *ProtocolHandler) ParseWriteMultipleCoilsResponse(data []byte) (Address, Quantity, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write multiple coils response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write multiple coils: expected 4, got %d", len(data))
		return 0, 0, ErrInvalidResponseLength
	}

	// Parse address and quantity from big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := Address(binary.BigEndian.Uint16(data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write multiple coils response: address=%d, quantity=%d", address, quantity)
	return address, quantity, nil
}

// GenerateWriteMultipleRegistersRequest generates the PDU data for a Write
// Multiple Registers request (function code 0x10). Up to 123 registers may
// be written per request.
func (h *ProtocolHandler) GenerateWriteMultipleRegistersRequest(address Address, values []RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write multiple registers request: address=%d, count=%d",
		address, len(values))

	if len(values) == 0 || len(values) > MaxWriteRegisterCount {
		h.logger.Error(ctx, "Invalid quantity for write multiple registers request: %d", len(values))
		return nil, ErrInvalidQuantity
	}

	// Calculate byte count
	byteCount := len(values) * 2

	// Allocate data
	data := make([]byte, 5+byteCount)

	// Address
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	// Quantity
	binary.BigEndian.PutUint16(data[2:4], uint16(len(values)))
	// Byte count
	data[4] = byte(byteCount)

	// Pack register values
	for i, value := range values {
		binary.BigEndian.PutUint16(data[5+i*2:5+i*2+2], value)
	}

	h.logger.Debug(ctx, "Generated write multiple registers request data: %v", data)
	return data, nil
}

// ParseWriteMultipleRegistersResponse parses the response from a Write Multiple
// Registers request (function code 0x10) and returns the confirmed starting
// address and quantity.
func (h *ProtocolHandler) ParseWriteMultipleRegistersResponse(data []byte) (Address, Quantity, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write multiple registers response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write multiple registers: expected 4, got %d", len(data))
		return 0, 0, ErrInvalidResponseLength
	}

	address := Address(binary.BigEndian.Uint16(data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write multiple registers response: address=%d, quantity=%d", address, quantity)
	return address, quantity, nil
}

// GenerateReadWriteMultipleRegistersRequest generates the PDU data for a
// Read/Write Multiple Registers request (function code 0x17). This function
// atomically writes registers and then reads registers in a single transaction.
// Read quantity must be between 1 and 125; write quantity between 1 and 121.
func (h *ProtocolHandler) GenerateReadWriteMultipleRegistersRequest(readAddress Address, readQuantity Quantity, writeAddress Address, writeValues []RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read/write multiple registers request: readAddress=%d, readQuantity=%d, writeAddress=%d, writeCount=%d",
		readAddress, readQuantity, writeAddress, len(writeValues))

	if readQuantity == 0 || readQuantity > MaxReadWriteReadCount {
		h.logger.Error(ctx, "Invalid read quantity for read/write multiple registers request: %d", readQuantity)
		return nil, ErrInvalidQuantity
	}
	if len(writeValues) == 0 || len(writeValues) > MaxReadWriteWriteCount {
		h.logger.Error(ctx, "Invalid write quantity for read/write multiple registers request: %d", len(writeValues))
		return nil, ErrInvalidQuantity
	}

	// Calculate byte count (2 bytes per register)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	byteCount := len(writeValues) * 2

	// Allocate data
	data := make([]byte, 9+byteCount)

	// Read address - in big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	binary.BigEndian.PutUint16(data[0:2], uint16(readAddress))
	// Read quantity - in big-endian format
	binary.BigEndian.PutUint16(data[2:4], uint16(readQuantity))
	// Write address - in big-endian format
	binary.BigEndian.PutUint16(data[4:6], uint16(writeAddress))
	// Write quantity - in big-endian format
	binary.BigEndian.PutUint16(data[6:8], uint16(len(writeValues)))
	// Byte count
	data[8] = byte(byteCount)

	// Pack register values - each value is 2 bytes in big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	for i, value := range writeValues {
		binary.BigEndian.PutUint16(data[9+i*2:9+i*2+2], value)
	}

	h.logger.Debug(ctx, "Generated read/write multiple registers request data: %v", data)
	return data, nil
}

// ParseReadWriteMultipleRegistersResponse parses the response from a Read/Write
// Multiple Registers request (function code 0x17) and returns the read register
// values. The response format is identical to a Read Holding Registers response.
func (h *ProtocolHandler) ParseReadWriteMultipleRegistersResponse(data []byte, readQuantity Quantity) ([]RegisterValue, error) {
	// Same implementation as ParseReadHoldingRegistersResponse
	// Reading holding registers and the read part of ReadWriteMultipleRegisters use the same response format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
	return h.ParseReadHoldingRegistersResponse(data, readQuantity)
}

// GenerateReadExceptionStatusRequest generates the PDU data for a Read
// Exception Status request (function code 0x07). This request has no payload.
func (h *ProtocolHandler) GenerateReadExceptionStatusRequest() ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read exception status request")

	// No data for this request
	return []byte{}, nil
}

// ParseReadExceptionStatusResponse parses the response from a Read Exception
// Status request (function code 0x07) and returns the 8-bit status bitmask.
func (h *ProtocolHandler) ParseReadExceptionStatusResponse(data []byte) (ExceptionStatus, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read exception status response: data=%v", data)

	if len(data) != 1 {
		h.logger.Error(ctx, "Invalid response length for read exception status: expected 1, got %d", len(data))
		return ExceptionStatus(0), ErrInvalidResponseLength
	}

	status := ExceptionStatus(data[0])
	h.logger.Debug(ctx, "Parsed read exception status response: status=%s", status)
	return status, nil
}

// GenerateReadDeviceIdentificationRequest generates the PDU data for a Read
// Device Identification request (function code 0x2B, MEI type 0x0E).
func (h *ProtocolHandler) GenerateReadDeviceIdentificationRequest(readDeviceIDCode ReadDeviceIDCode, objectID DeviceIDObjectCode) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read device identification request: code=%d, objectID=%d", readDeviceIDCode, objectID)

	// Validate read device ID code
	if readDeviceIDCode < ReadDeviceIDBasic || readDeviceIDCode > ReadDeviceIDSpecific {
		h.logger.Error(ctx, "Invalid read device ID code: %d", readDeviceIDCode)
		return nil, ErrInvalidValue
	}

	// Data format:
	// Byte 0: MEI Type (0x0E for ReadDeviceID)
	// Byte 1: ReadDeviceID code (0x01-0x04)
	// Byte 2: Object ID
	data := []byte{byte(MEIReadDeviceID), byte(readDeviceIDCode), byte(objectID)}

	h.logger.Debug(ctx, "Generated read device identification request data: %v", data)
	return data, nil
}

// ParseReadDeviceIdentificationResponse parses the response from a Read Device
// Identification request (function code 0x2B, MEI type 0x0E) and returns the
// parsed [DeviceIdentification] containing the device's identification objects.
func (h *ProtocolHandler) ParseReadDeviceIdentificationResponse(data []byte) (*DeviceIdentification, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read device identification response: %v", data)

	// Check data length - minimum is 6 bytes (for a response with no objects)
	// MEI Type (1) + ReadDeviceID code (1) + Conformity level (1) + More Follows (1) +
	// Next Object ID (1) + Number of Objects (1)
	if len(data) < 6 {
		h.logger.Error(ctx, "Invalid response length for read device identification: %d", len(data))
		return nil, ErrInvalidResponseLength
	}

	// Check MEI Type
	if MEIType(data[0]) != MEIReadDeviceID {
		h.logger.Error(ctx, "Invalid MEI type: 0x%02X, expected 0x%02X", data[0], MEIReadDeviceID)
		return nil, ErrInvalidValue
	}

	// Create device identification object
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU Format)
	result := &DeviceIdentification{
		ReadDeviceIDCode: ReadDeviceIDCode(data[1]),   // Echoes the request's ReadDeviceIDCode
		ConformityLevel:  ConformityLevel(data[2]),    // Conformity level of the device
		MoreFollows:      MoreFollows(data[3]),        // Indicates if more objects follow in subsequent requests
		NextObjectID:     DeviceIDObjectCode(data[4]), // Object ID to request next if MoreFollows is true
		NumberOfObjects:  data[5],                     // Number of objects in this response
		Objects:          make([]DeviceIDObject, 0, data[5]),
	}

	// Parse objects
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response Objects Format)
	// Each object has the format:
	// - Object ID (1 byte)
	// - Object Length (1 byte)
	// - Object Value (n bytes)
	offset := 6
	for i := 0; i < int(data[5]); i++ {
		// Check if we have enough data
		if offset+2 > len(data) {
			h.logger.Error(ctx, "Invalid response format for read device identification: not enough data for object header")
			return nil, ErrInvalidResponseFormat
		}

		// Get object ID and length
		objectID := DeviceIDObjectCode(data[offset])
		objectLength := data[offset+1]
		offset += 2

		// Check if we have enough data for the object value
		if offset+int(objectLength) > len(data) {
			h.logger.Error(ctx, "Invalid response format for read device identification: not enough data for object value")
			return nil, ErrInvalidResponseFormat
		}

		// Get object value (convert bytes to string)
		objectValue := string(data[offset : offset+int(objectLength)])
		offset += int(objectLength)

		// Add object to result
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 72
		// Create object with ID, length, and value fields as per the specification
		result.Objects = append(result.Objects, DeviceIDObject{
			ID:     objectID,     // Object ID code as defined in Table 72
			Length: objectLength, // Length of the object value
			Value:  objectValue,  // String value of the object
		})
	}

	h.logger.Debug(ctx, "Parsed read device identification response: %d objects", len(result.Objects))
	return result, nil
}
