package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
)

// ProtocolHandler implements the common.Protocol interface for Modbus protocol
type ProtocolHandler struct {
	logger common.LoggerInterface
}

// Option is a function that configures a ProtocolHandler
type Option func(*ProtocolHandler)

// WithLogger sets the logger for the protocol handler
func WithLogger(logger common.LoggerInterface) Option {
	return func(p *ProtocolHandler) {
		p.logger = logger
	}
}

// NewProtocolHandler creates a new ProtocolHandler with options
func NewProtocolHandler(options ...Option) *ProtocolHandler {
	handler := &ProtocolHandler{
		logger: logging.NewLogger(), // Default logger
	}

	// Apply options
	for _, option := range options {
		option(handler)
	}

	return handler
}

// WithLogger returns a new ProtocolHandler with the given logger
func (h *ProtocolHandler) WithLogger(logger common.LoggerInterface) common.Protocol {
	return NewProtocolHandler(WithLogger(logger))
}

// generateReadRequest is a helper function for generating read requests that follow the same pattern
// (read coils, read discrete inputs, read holding registers, read input registers)
func (h *ProtocolHandler) generateReadRequest(itemType string, address common.Address, quantity common.Quantity, maxQuantity common.Quantity) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read %s request: address=%d, quantity=%d", itemType, address, quantity)

	if quantity == 0 || quantity > maxQuantity {
		h.logger.Error(ctx, "Invalid quantity for read %s request: %d (max %d)", itemType, quantity, maxQuantity)
		return nil, common.ErrInvalidQuantity
	}

	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], uint16(quantity))

	h.logger.Debug(ctx, "Generated read %s request data: %v", itemType, data)
	return data, nil
}

// parseBitResponse is a helper function for parsing responses that contain bit values
// (coils and discrete inputs)
func (h *ProtocolHandler) parseBitResponse(itemType string, data []byte, quantity common.Quantity) ([]bool, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read %s response: data=%v, quantity=%d", itemType, data, quantity)

	if len(data) == 0 {
		h.logger.Error(ctx, "Empty response for read %s", itemType)
		return nil, common.ErrEmptyResponse
	}

	// First byte is the byte count
	byteCount := int(data[0])
	if len(data) != byteCount+1 {
		h.logger.Error(ctx, "Invalid response length for read %s: expected %d, got %d",
			itemType, byteCount+1, len(data))
		return nil, common.ErrInvalidResponseLength
	}

	// Calculate the expected byte count
	expectedByteCount := int(math.Ceil(float64(quantity) / 8.0))
	if byteCount != expectedByteCount {
		h.logger.Error(ctx, "Invalid byte count for read %s: expected %d, got %d",
			itemType, expectedByteCount, byteCount)
		return nil, common.ErrInvalidResponseLength
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
func (h *ProtocolHandler) parseRegisterResponse(itemType string, data []byte, quantity common.Quantity) ([]uint16, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read %s response: data=%v, quantity=%d", itemType, data, quantity)

	if len(data) == 0 {
		h.logger.Error(ctx, "Empty response for read %s", itemType)
		return nil, common.ErrEmptyResponse
	}

	// First byte is the byte count
	byteCount := int(data[0])
	if len(data) != byteCount+1 {
		h.logger.Error(ctx, "Invalid response length for read %s: expected %d, got %d",
			itemType, byteCount+1, len(data))
		return nil, common.ErrInvalidResponseLength
	}

	// Calculate the expected byte count
	expectedByteCount := int(quantity) * 2
	if byteCount != expectedByteCount {
		h.logger.Error(ctx, "Invalid byte count for read %s: expected %d, got %d",
			itemType, expectedByteCount, byteCount)
		return nil, common.ErrInvalidResponseLength
	}

	// Parse the values
	values := make([]uint16, quantity)
	for i := 0; i < int(quantity); i++ {
		values[i] = binary.BigEndian.Uint16(data[1+i*2 : 1+i*2+2])
	}

	h.logger.Debug(ctx, "Parsed %d %s values", len(values), itemType)
	return values, nil
}

// GenerateReadCoilsRequest generates a request to read coils
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Read Coils)
//
// PDU Data:
// Starting Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1
// Quantity of Coils (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1
// Quantity constraints: 1 to 2000 - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1
func (h *ProtocolHandler) GenerateReadCoilsRequest(address common.Address, quantity common.Quantity) ([]byte, error) {
	return h.generateReadRequest("coils", address, quantity, common.MaxCoilCount)
}

// ParseReadCoilsResponse parses a response to a read coils request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1 (Read Coils)
//
// PDU Data:
// Byte Count (1 byte) - Number of data bytes to follow (N) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1
// Coil Status (Byte Count bytes, packed bits, LSB of first byte = lowest address) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1
func (h *ProtocolHandler) ParseReadCoilsResponse(data []byte, quantity common.Quantity) ([]common.CoilValue, error) {
	// Use the parseBitResponse helper and cast the result to the expected type
	values, err := h.parseBitResponse("coils", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []bool to []common.CoilValue (type alias, so this is a no-op in Go)
	coilValues := make([]common.CoilValue, len(values))
	for i, v := range values {
		coilValues[i] = common.CoilValue(v)
	}

	return coilValues, nil
}

// GenerateReadDiscreteInputsRequest generates a request to read discrete inputs
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.2 (Read Discrete Inputs)
func (h *ProtocolHandler) GenerateReadDiscreteInputsRequest(address common.Address, quantity common.Quantity) ([]byte, error) {
	return h.generateReadRequest("discrete inputs", address, quantity, common.MaxCoilCount)
}

// ParseReadDiscreteInputsResponse parses a response to a read discrete inputs request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.2 (Read Discrete Inputs)
func (h *ProtocolHandler) ParseReadDiscreteInputsResponse(data []byte, quantity common.Quantity) ([]common.DiscreteInputValue, error) {
	// Use the parseBitResponse helper and cast the result to the expected type
	values, err := h.parseBitResponse("discrete inputs", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []bool to []common.DiscreteInputValue (type alias, so this is a no-op in Go)
	discreteValues := make([]common.DiscreteInputValue, len(values))
	for i, v := range values {
		discreteValues[i] = common.DiscreteInputValue(v)
	}

	return discreteValues, nil
}

// GenerateReadHoldingRegistersRequest generates a request to read holding registers
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Read Holding Registers)
func (h *ProtocolHandler) GenerateReadHoldingRegistersRequest(address common.Address, quantity common.Quantity) ([]byte, error) {
	return h.generateReadRequest("holding registers", address, quantity, common.MaxRegisterCount)
}

// ParseReadHoldingRegistersResponse parses a response to a read holding registers request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3 (Read Holding Registers)
func (h *ProtocolHandler) ParseReadHoldingRegistersResponse(data []byte, quantity common.Quantity) ([]common.RegisterValue, error) {
	// Use the parseRegisterResponse helper and cast the result to the expected type
	values, err := h.parseRegisterResponse("holding registers", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []uint16 to []common.RegisterValue (type alias, so this is a no-op in Go)
	registerValues := make([]common.RegisterValue, len(values))
	for i, v := range values {
		registerValues[i] = common.RegisterValue(v)
	}

	return registerValues, nil
}

// GenerateReadInputRegistersRequest generates a request to read input registers
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Read Input Registers)
func (h *ProtocolHandler) GenerateReadInputRegistersRequest(address common.Address, quantity common.Quantity) ([]byte, error) {
	return h.generateReadRequest("input registers", address, quantity, common.MaxRegisterCount)
}

// ParseReadInputRegistersResponse parses a response to a read input registers request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.4 (Read Input Registers)
func (h *ProtocolHandler) ParseReadInputRegistersResponse(data []byte, quantity common.Quantity) ([]common.InputRegisterValue, error) {
	// Use the parseRegisterResponse helper and cast the result to the expected type
	values, err := h.parseRegisterResponse("input registers", data, quantity)
	if err != nil {
		return nil, err
	}

	// Convert []uint16 to []common.InputRegisterValue (type alias, so this is a no-op in Go)
	inputValues := make([]common.InputRegisterValue, len(values))
	for i, v := range values {
		inputValues[i] = common.InputRegisterValue(v)
	}

	return inputValues, nil
}

// GenerateWriteSingleCoilRequest generates a request to write a single coil
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Write Single Coil)
//
// PDU Data:
// Output Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
// Output Value (2 bytes: 0xFF00 for ON, 0x0000 for OFF) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
func (h *ProtocolHandler) GenerateWriteSingleCoilRequest(address common.Address, value common.CoilValue) ([]byte, error) {
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
		binary.BigEndian.PutUint16(data[2:4], common.CoilOnU16)
	} else {
		binary.BigEndian.PutUint16(data[2:4], common.CoilOffU16)
	}

	h.logger.Debug(ctx, "Generated write single coil request data: %v", data)
	return data, nil
}

// ParseWriteSingleCoilResponse parses a response to a write single coil request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Write Single Coil)
//
// PDU Data (Echo of request):
// Output Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
// Output Value (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
func (h *ProtocolHandler) ParseWriteSingleCoilResponse(data []byte) (common.Address, common.CoilValue, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write single coil response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write single coil: expected 4, got %d", len(data))
		return 0, false, common.ErrInvalidResponseLength
	}

	// Parse address from big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(data[0:2]))
	value := binary.BigEndian.Uint16(data[2:4])

	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
	// The normal response is an echo of the request
	switch value {
	case common.CoilOnU16:
		h.logger.Debug(ctx, "Parsed write single coil response: address=%d, value=true", address)
		return address, true, nil
	case common.CoilOffU16:
		h.logger.Debug(ctx, "Parsed write single coil response: address=%d, value=false", address)
		return address, false, nil
	default:
		h.logger.Error(ctx, "Invalid coil value in response: %d", value)
		return address, false, fmt.Errorf("invalid coil value: %d", value)
	}
}

// GenerateWriteSingleRegisterRequest generates a request to write a single register
func (h *ProtocolHandler) GenerateWriteSingleRegisterRequest(address common.Address, value common.RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write single register request: address=%d, value=%d", address, value)

	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], value)

	h.logger.Debug(ctx, "Generated write single register request data: %v", data)
	return data, nil
}

// ParseWriteSingleRegisterResponse parses a response to a write single register request
func (h *ProtocolHandler) ParseWriteSingleRegisterResponse(data []byte) (common.Address, common.RegisterValue, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write single register response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write single register: expected 4, got %d", len(data))
		return 0, 0, common.ErrInvalidResponseLength
	}

	address := common.Address(binary.BigEndian.Uint16(data[0:2]))
	value := common.RegisterValue(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write single register response: address=%d, value=%d", address, value)
	return address, value, nil
}

// GenerateWriteMultipleCoilsRequest generates a request to write multiple coils
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Write Multiple Coils)
//
// PDU Data:
// Starting Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
// Quantity of Outputs (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
// Byte Count (1 byte) - Number of data bytes to follow - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
// Output Value (Byte Count bytes, packed bits) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
// Quantity constraints: 1 to 1968 (0x07B0) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Table of Constraints)
func (h *ProtocolHandler) GenerateWriteMultipleCoilsRequest(address common.Address, values []common.CoilValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write multiple coils request: address=%d, count=%d",
		address, len(values))

	if len(values) == 0 || len(values) > common.MaxWriteCoilCount {
		h.logger.Error(ctx, "Invalid quantity for write multiple coils request: %d", len(values))
		return nil, common.ErrInvalidQuantity
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

// ParseWriteMultipleCoilsResponse parses a response to a write multiple coils request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Write Multiple Coils)
//
// PDU Data:
// Starting Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
// Quantity of Outputs (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
func (h *ProtocolHandler) ParseWriteMultipleCoilsResponse(data []byte) (common.Address, common.Quantity, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write multiple coils response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write multiple coils: expected 4, got %d", len(data))
		return 0, 0, common.ErrInvalidResponseLength
	}

	// Parse address and quantity from big-endian format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write multiple coils response: address=%d, quantity=%d", address, quantity)
	return address, quantity, nil
}

// GenerateWriteMultipleRegistersRequest generates a request to write multiple registers
func (h *ProtocolHandler) GenerateWriteMultipleRegistersRequest(address common.Address, values []common.RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating write multiple registers request: address=%d, count=%d",
		address, len(values))

	if len(values) == 0 || len(values) > common.MaxWriteRegisterCount {
		h.logger.Error(ctx, "Invalid quantity for write multiple registers request: %d", len(values))
		return nil, common.ErrInvalidQuantity
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

// ParseWriteMultipleRegistersResponse parses a response to a write multiple registers request
func (h *ProtocolHandler) ParseWriteMultipleRegistersResponse(data []byte) (common.Address, common.Quantity, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing write multiple registers response: data=%v", data)

	if len(data) != 4 {
		h.logger.Error(ctx, "Invalid response length for write multiple registers: expected 4, got %d", len(data))
		return 0, 0, common.ErrInvalidResponseLength
	}

	address := common.Address(binary.BigEndian.Uint16(data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(data[2:4]))

	h.logger.Debug(ctx, "Parsed write multiple registers response: address=%d, quantity=%d", address, quantity)
	return address, quantity, nil
}

// GenerateReadWriteMultipleRegistersRequest generates a request to read and write multiple registers
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Read/Write Multiple Registers)
//
// PDU Data:
// Read Starting Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Quantity to Read (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Write Starting Address (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Quantity to Write (2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Write Byte Count (1 byte) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Write Registers Value (N * 2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Read Quantity constraints: 1 to 125 (0x007D) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Table of Constraints)
// Write Quantity constraints: 1 to 121 (0x0079) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Table of Constraints)
func (h *ProtocolHandler) GenerateReadWriteMultipleRegistersRequest(readAddress common.Address, readQuantity common.Quantity, writeAddress common.Address, writeValues []common.RegisterValue) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read/write multiple registers request: readAddress=%d, readQuantity=%d, writeAddress=%d, writeCount=%d",
		readAddress, readQuantity, writeAddress, len(writeValues))

	if readQuantity == 0 || readQuantity > common.MaxReadWriteReadCount {
		h.logger.Error(ctx, "Invalid read quantity for read/write multiple registers request: %d", readQuantity)
		return nil, common.ErrInvalidQuantity
	}
	if len(writeValues) == 0 || len(writeValues) > common.MaxReadWriteWriteCount {
		h.logger.Error(ctx, "Invalid write quantity for read/write multiple registers request: %d", len(writeValues))
		return nil, common.ErrInvalidQuantity
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

// ParseReadWriteMultipleRegistersResponse parses a response to a read/write multiple registers request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Read/Write Multiple Registers)
//
// PDU Data:
// Byte Count (1 byte) - N*2 bytes of read data - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
// Read Registers Value (N * 2 bytes) - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
func (h *ProtocolHandler) ParseReadWriteMultipleRegistersResponse(data []byte, readQuantity common.Quantity) ([]common.RegisterValue, error) {
	// Same implementation as ParseReadHoldingRegistersResponse
	// Reading holding registers and the read part of ReadWriteMultipleRegisters use the same response format
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
	return h.ParseReadHoldingRegistersResponse(data, readQuantity)
}

// GenerateReadExceptionStatusRequest generates a request to read the exception status
func (h *ProtocolHandler) GenerateReadExceptionStatusRequest() ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read exception status request")

	// No data for this request
	return []byte{}, nil
}

// ParseReadExceptionStatusResponse parses a response to a read exception status request
func (h *ProtocolHandler) ParseReadExceptionStatusResponse(data []byte) (common.ExceptionStatus, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read exception status response: data=%v", data)

	if len(data) != 1 {
		h.logger.Error(ctx, "Invalid response length for read exception status: expected 1, got %d", len(data))
		return common.ExceptionStatus(0), common.ErrInvalidResponseLength
	}

	status := common.ExceptionStatus(data[0])
	h.logger.Debug(ctx, "Parsed read exception status response: status=%s", status)
	return status, nil
}

// GenerateReadDeviceIdentificationRequest generates a request to read device identification
func (h *ProtocolHandler) GenerateReadDeviceIdentificationRequest(readDeviceIDCode common.ReadDeviceIDCode, objectID common.DeviceIDObjectCode) ([]byte, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Generating read device identification request: code=%d, objectID=%d", readDeviceIDCode, objectID)

	// Validate read device ID code
	if readDeviceIDCode < common.ReadDeviceIDBasic || readDeviceIDCode > common.ReadDeviceIDSpecific {
		h.logger.Error(ctx, "Invalid read device ID code: %d", readDeviceIDCode)
		return nil, common.ErrInvalidValue
	}

	// Data format:
	// Byte 0: MEI Type (0x0E for ReadDeviceID)
	// Byte 1: ReadDeviceID code (0x01-0x04)
	// Byte 2: Object ID
	data := []byte{byte(common.MEIReadDeviceID), byte(readDeviceIDCode), byte(objectID)}

	h.logger.Debug(ctx, "Generated read device identification request data: %v", data)
	return data, nil
}

// ParseReadDeviceIdentificationResponse parses a response from a read device identification request
func (h *ProtocolHandler) ParseReadDeviceIdentificationResponse(data []byte) (*common.DeviceIdentification, error) {
	ctx := context.Background()
	h.logger.Debug(ctx, "Parsing read device identification response: %v", data)

	// Check data length - minimum is 6 bytes (for a response with no objects)
	// MEI Type (1) + ReadDeviceID code (1) + Conformity level (1) + More Follows (1) +
	// Next Object ID (1) + Number of Objects (1)
	if len(data) < 6 {
		h.logger.Error(ctx, "Invalid response length for read device identification: %d", len(data))
		return nil, common.ErrInvalidResponseLength
	}

	// Check MEI Type
	if common.MEIType(data[0]) != common.MEIReadDeviceID {
		h.logger.Error(ctx, "Invalid MEI type: 0x%02X, expected 0x%02X", data[0], common.MEIReadDeviceID)
		return nil, common.ErrInvalidValue
	}

	// Create device identification object
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU Format)
	result := &common.DeviceIdentification{
		ReadDeviceIDCode: common.ReadDeviceIDCode(data[1]), // Echoes the request's ReadDeviceIDCode
		ConformityLevel:  common.ConformityLevel(data[2]),    // Conformity level of the device
		MoreFollows:      common.MoreFollows(data[3]),         // Indicates if more objects follow in subsequent requests
		NextObjectID:     common.DeviceIDObjectCode(data[4]), // Object ID to request next if MoreFollows is true
		NumberOfObjects:  data[5],                           // Number of objects in this response
		Objects:          make([]common.DeviceIDObject, 0, data[5]), // The actual objects
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
			return nil, common.ErrInvalidResponseFormat
		}

		// Get object ID and length
		objectID := common.DeviceIDObjectCode(data[offset])
		objectLength := data[offset+1]
		offset += 2

		// Check if we have enough data for the object value
		if offset+int(objectLength) > len(data) {
			h.logger.Error(ctx, "Invalid response format for read device identification: not enough data for object value")
			return nil, common.ErrInvalidResponseFormat
		}

		// Get object value (convert bytes to string)
		objectValue := string(data[offset : offset+int(objectLength)])
		offset += int(objectLength)

		// Add object to result
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 72
		// Create object with ID, length, and value fields as per the specification
		result.Objects = append(result.Objects, common.DeviceIDObject{
			ID:     objectID,     // Object ID code as defined in Table 72
			Length: objectLength, // Length of the object value
			Value:  objectValue,  // String value of the object
		})
	}

	h.logger.Debug(ctx, "Parsed read device identification response: %d objects", len(result.Objects))
	return result, nil
}
