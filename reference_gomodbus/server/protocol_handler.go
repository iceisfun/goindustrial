package server

import (
	"context"
	"encoding/binary"
	"math"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// serverProtocolHandler processes Modbus requests and generates responses
type serverProtocolHandler struct{}

// newServerProtocolHandler creates a new protocol handler for server
func newServerProtocolHandler() *serverProtocolHandler {
	return &serverProtocolHandler{}
}

// handleReadBitValues is a helper function for handling bit value read requests (coils, discrete inputs)
// This handles both Read Coils (0x01) and Read Discrete Inputs (0x02) functions
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.1 and 6.2 (Read Coils/Discrete Inputs)
func (h *serverProtocolHandler) handleReadBitValues(
	ctx context.Context,
	req common.Request,
	store common.DataStore,
	itemType string,
	maxQuantity common.Quantity,
	readFunc func(context.Context, common.Address, common.Quantity) ([]bool, error)) (common.Response, error) {

	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1/6.2 (Request PDU)
	// Request format:
	// - Starting Address (2 bytes)
	// - Quantity of Coils/Inputs (2 bytes)
	if len(req.GetPDU().Data) != 4 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract starting address and quantity using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	// Validate quantity (between 1 and maxQuantity)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1/6.2 (Constraints)
	if quantity == 0 || quantity > maxQuantity {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Read values from data store
	values, err := readFunc(ctx, address, quantity)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Calculate response data size and create response data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1/6.2 (Response PDU)
	// Response format:
	// - Byte Count (1 byte)
	// - Coil/Input Status (N bytes, packed bits, LSB of first byte = lowest address)
	byteCount := int(math.Ceil(float64(quantity) / 8.0))
	responseData := make([]byte, 1+byteCount)
	responseData[0] = byte(byteCount) // First byte is the byte count

	// Pack bit values into bytes - LSB of first byte corresponds to lowest address
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1/6.2 (Response)
	// "The coil/input status in the response message is packed as one coil/input per bit of the data field."
	for i, value := range values {
		if value {
			byteIndex := i / 8
			bitOffset := i % 8
			responseData[1+byteIndex] |= (1 << uint(bitOffset))
		}
	}

	// Create the response
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.1/6.2 (Response)
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// handleReadRegisterValues is a helper function for handling register read requests (holding/input registers)
// This handles both Read Holding Registers (0x03) and Read Input Registers (0x04) functions
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.3 and 6.4 (Read Holding/Input Registers)
func (h *serverProtocolHandler) handleReadRegisterValues(
	ctx context.Context,
	req common.Request,
	store common.DataStore,
	itemType string,
	maxQuantity common.Quantity,
	readFunc func(context.Context, common.Address, common.Quantity) ([]uint16, error)) (common.Response, error) {

	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3/6.4 (Request PDU)
	// Request format:
	// - Starting Address (2 bytes)
	// - Quantity of Registers (2 bytes)
	if len(req.GetPDU().Data) != 4 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract starting address and quantity using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	// Validate quantity (between 1 and maxQuantity)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3/6.4 (Constraints)
	if quantity == 0 || quantity > maxQuantity {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Read registers from data store
	values, err := readFunc(ctx, address, quantity)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Calculate response data size and create response data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3/6.4 (Response PDU)
	// Response format:
	// - Byte Count (1 byte) - number of bytes to follow (2 × N for N registers)
	// - Register Values (N × 2 bytes) - each register as 2 bytes in big-endian format
	byteCount := len(values) * 2
	responseData := make([]byte, 1+byteCount)
	responseData[0] = byte(byteCount) // First byte is the byte count

	// Pack register values into bytes using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	// "Each MODBUS data type is packed into a 2 byte field in big-endian format"
	for i, value := range values {
		binary.BigEndian.PutUint16(responseData[1+i*2:1+i*2+2], value)
	}

	// Create the response
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.3/6.4 (Response)
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadCoils processes a read coils request
func (h *serverProtocolHandler) HandleReadCoils(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	return h.handleReadBitValues(
		ctx,
		req,
		store,
		"coils",
		common.MaxCoilCount,
		store.ReadCoils,
	)
}

// HandleReadDiscreteInputs processes a read discrete inputs request
func (h *serverProtocolHandler) HandleReadDiscreteInputs(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	return h.handleReadBitValues(
		ctx,
		req,
		store,
		"discrete inputs",
		common.MaxCoilCount,
		store.ReadDiscreteInputs,
	)
}

// HandleReadHoldingRegisters processes a read holding registers request
func (h *serverProtocolHandler) HandleReadHoldingRegisters(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	return h.handleReadRegisterValues(
		ctx,
		req,
		store,
		"holding registers",
		common.MaxRegisterCount,
		store.ReadHoldingRegisters,
	)
}

// HandleReadInputRegisters processes a read input registers request
func (h *serverProtocolHandler) HandleReadInputRegisters(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	return h.handleReadRegisterValues(
		ctx,
		req,
		store,
		"input registers",
		common.MaxRegisterCount,
		store.ReadInputRegisters,
	)
}

// HandleWriteSingleCoil processes a write single coil request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Write Single Coil)
func (h *serverProtocolHandler) HandleWriteSingleCoil(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Request PDU)
	// Request format:
	// - Output Address (2 bytes)
	// - Output Value (2 bytes: 0xFF00 for ON, 0x0000 for OFF)
	if len(req.GetPDU().Data) != 4 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract output address and value using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	value := binary.BigEndian.Uint16(req.GetPDU().Data[2:4])

	// Check that value is either 0x0000 (OFF) or 0xFF00 (ON)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Request)
	// "A value of FF00 hex requests the output to be ON. A value of 0000 requests it to be OFF."
	// "All other values are illegal and will not affect the output."
	var coilValue common.CoilValue
	if value == common.CoilOnU16 {
		coilValue = true
	} else if value == common.CoilOffU16 {
		coilValue = false
	} else {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Write the coil value to the data store
	err := store.WriteSingleCoil(ctx, address, coilValue)
	if err != nil {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Create the response (echo the request)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5 (Response PDU)
	// "The normal response is an echo of the request, returned after the coil state has been written."
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		req.GetPDU().Data,
	)

	return response, nil
}

// HandleWriteSingleRegister processes a write single register request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.6 (Write Single Register)
func (h *serverProtocolHandler) HandleWriteSingleRegister(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.6 (Request PDU)
	// Request format:
	// - Register Address (2 bytes)
	// - Register Value (2 bytes)
	if len(req.GetPDU().Data) != 4 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract register address and value using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	value := common.RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	// Write the register value to the data store
	err := store.WriteSingleRegister(ctx, address, value)
	if err != nil {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Create the response (echo the request)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.6 (Response PDU)
	// "The normal response is an echo of the request, returned after the register value has been written."
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		req.GetPDU().Data,
	)

	return response, nil
}

// HandleWriteMultipleCoils processes a write multiple coils request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Write Multiple Coils)
func (h *serverProtocolHandler) HandleWriteMultipleCoils(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Request PDU)
	// Request format:
	// - Starting Address (2 bytes)
	// - Quantity of Outputs (2 bytes)
	// - Byte Count (1 byte) - N bytes to follow
	// - Output Values (N bytes) - packed bits, LSB of first byte = lowest coil address
	if len(req.GetPDU().Data) < 5 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract starting address, quantity, and byte count using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	byteCount := int(req.GetPDU().Data[4])

	// Validate data length
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Request Validation)
	if len(req.GetPDU().Data) != 5+byteCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Check quantity limits
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Constraints)
	// "The quantity of outputs must be in the range of 1 to 1968 (0x07B0) both inclusive."
	if quantity == 0 || quantity > common.MaxWriteCoilCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Validate byte count matches quantity
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Request)
	// "The Byte Count field contains the number of complete bytes needed to contain the quantity of outputs."
	expectedByteCount := int(math.Ceil(float64(quantity) / 8.0))
	if byteCount != expectedByteCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract coil values from request
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Request Data Encoding)
	// "The outputs are packed one per bit of the data field. Status is indicated as 1=ON and 0=OFF."
	// "The LSB of the first data byte contains the output addressed in the request."
	values := make([]common.CoilValue, quantity)
	for i := uint16(0); i < uint16(quantity); i++ {
		byteIndex := i / 8
		bitOffset := i % 8
		values[i] = (req.GetPDU().Data[5+byteIndex]>>uint(bitOffset))&0x01 != 0
	}

	// Write the coil values to the data store
	err := store.WriteMultipleCoils(ctx, address, values)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Create the response
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11 (Response PDU)
	// Response format:
	// - Starting Address (2 bytes)
	// - Quantity of Outputs (2 bytes)
	responseData := make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], uint16(quantity))

	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleWriteMultipleRegisters processes a write multiple registers request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Write Multiple Registers)
func (h *serverProtocolHandler) HandleWriteMultipleRegisters(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Request PDU)
	// Request format:
	// - Starting Address (2 bytes)
	// - Quantity of Registers (2 bytes)
	// - Byte Count (1 byte) - N bytes to follow (N = 2 × Quantity of Registers)
	// - Register Values (N bytes) - 2 bytes per register, big-endian format
	if len(req.GetPDU().Data) < 5 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract starting address, quantity, and byte count using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	address := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	byteCount := int(req.GetPDU().Data[4])

	// Validate data length
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Request Validation)
	if len(req.GetPDU().Data) != 5+byteCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Check quantity limits
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Constraints)
	// "The quantity of registers must be in the range of 1 to 123 (0x7B) both inclusive."
	if quantity == 0 || quantity > common.MaxWriteRegisterCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Validate byte count matches quantity
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Request)
	// "The Byte Count field specifies the number of bytes to follow in the Register Values field."
	// "This must be equal to twice the Quantity of Registers value, as each register is 2 bytes."
	if byteCount != int(quantity)*2 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract register values from request
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Request Data Encoding)
	// "Each register value is transmitted as 2 bytes, with the high order byte first."
	values := make([]common.RegisterValue, quantity)
	for i := uint16(0); i < uint16(quantity); i++ {
		values[i] = common.RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[5+i*2 : 5+i*2+2]))
	}

	// Write the register values to the data store
	err := store.WriteMultipleRegisters(ctx, address, values)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Create the response
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.12 (Response PDU)
	// Response format:
	// - Starting Address (2 bytes)
	// - Quantity of Registers (2 bytes)
	responseData := make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], uint16(quantity))

	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadWriteMultipleRegisters processes a read/write multiple registers request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Read/Write Multiple Registers)
func (h *serverProtocolHandler) HandleReadWriteMultipleRegisters(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Request PDU)
	// Request format:
	// - Read Starting Address (2 bytes)
	// - Quantity to Read (2 bytes)
	// - Write Starting Address (2 bytes)
	// - Quantity to Write (2 bytes)
	// - Write Byte Count (1 byte) - N bytes to follow (N = 2 × Quantity to Write)
	// - Write Register Values (N bytes) - 2 bytes per register, big-endian format
	if len(req.GetPDU().Data) < 9 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract addresses, quantities, and byte count using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	readAddress := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	readQuantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	writeAddress := common.Address(binary.BigEndian.Uint16(req.GetPDU().Data[4:6]))
	writeQuantity := common.Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[6:8]))
	byteCount := int(req.GetPDU().Data[8])

	// Validate data length
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Request Validation)
	if len(req.GetPDU().Data) != 9+byteCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Check quantity limits
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Constraints)
	// "The quantity of registers to read must be in the range of 1 to 125 (0x7D) both inclusive."
	// "The quantity of registers to write must be in the range of 1 to 121 (0x79) both inclusive."
	if readQuantity == 0 || readQuantity > common.MaxReadWriteReadCount ||
		writeQuantity == 0 || writeQuantity > common.MaxReadWriteWriteCount {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Validate byte count matches write quantity
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Request)
	// "The Byte Count field specifies the number of bytes to follow in the Write Register Values field."
	// "This must be equal to twice the Quantity of Registers to Write value, as each register is 2 bytes."
	if byteCount != int(writeQuantity)*2 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Extract register values from request
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Request Data Encoding)
	// "Each register value is transmitted as 2 bytes, with the high order byte first."
	writeValues := make([]common.RegisterValue, writeQuantity)
	for i := uint16(0); i < uint16(writeQuantity); i++ {
		writeValues[i] = common.RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[9+i*2 : 9+i*2+2]))
	}

	// Write the register values to the data store
	err := store.WriteMultipleRegisters(ctx, writeAddress, writeValues)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Read the register values from the data store
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Query Processing)
	// "The write operation is performed before the read operation."
	readValues, err := store.ReadHoldingRegisters(ctx, readAddress, readQuantity)
	if err != nil {
		if err == common.ErrInvalidQuantity {
			return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
		}
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionServerDeviceFailure)
	}

	// Calculate response data size and create response data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17 (Response PDU)
	// Response format:
	// - Byte Count (1 byte) - N bytes to follow (N = 2 × Quantity of Registers to Read)
	// - Read Register Values (N bytes) - 2 bytes per register, big-endian format
	byteCount = len(readValues) * 2
	responseData := make([]byte, 1+byteCount)
	responseData[0] = byte(byteCount) // First byte is the byte count

	// Pack register values into bytes using big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding)
	for i, value := range readValues {
		binary.BigEndian.PutUint16(responseData[1+i*2:1+i*2+2], value)
	}

	// Create the response
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadDeviceIdentification processes a read device identification request
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Read Device Identification)
func (h *serverProtocolHandler) HandleReadDeviceIdentification(ctx context.Context, req common.Request, store common.DataStore) (common.Response, error) {
	// Parse request PDU data
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Request PDU)
	// Request format:
	// - MEI Type (1 byte): 0x0E for Read Device Identification
	// - ReadDeviceID code (1 byte): 0x01-0x04 (access level)
	// - Object ID (1 byte): ID of the first object to obtain
	if len(req.GetPDU().Data) < 3 {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	// Data format:
	// Byte 0: MEI Type (0x0E for ReadDeviceID)
	// Byte 1: ReadDeviceID code (0x01-0x04)
	// Byte 2: Object ID
	if common.MEIType(req.GetPDU().Data[0]) != common.MEIReadDeviceID {
		return nil, common.NewModbusError(req.GetPDU().FunctionCode, common.ExceptionInvalidDataValue)
	}

	readDeviceIDCode := common.ReadDeviceIDCode(req.GetPDU().Data[1])
	objectID := common.DeviceIDObjectCode(req.GetPDU().Data[2])

	// Create a response based on the request
	// Fixed values for this example server
	deviceID := &common.DeviceIdentification{
		ReadDeviceIDCode: readDeviceIDCode,
		ConformityLevel:  common.ConformityLevelBasic, // Basic identification
		MoreFollows:      common.MoreFollowsNo,
		NextObjectID:     0x00,
		NumberOfObjects:  0,
		Objects:          make([]common.DeviceIDObject, 0),
	}

	// Default objects to include (all basic identification objects)
	objectsToInclude := []common.DeviceIDObjectCode{
		common.DeviceIDVendorName,
		common.DeviceIDProductCode,
		common.DeviceIDMajorMinorRevision,
	}

	// Specific object handling
	if readDeviceIDCode == common.ReadDeviceIDSpecificObject {
		objectsToInclude = []common.DeviceIDObjectCode{objectID}
	} else if readDeviceIDCode == common.ReadDeviceIDRegularStream {
		// Include regular objects too
		objectsToInclude = append(objectsToInclude,
			common.DeviceIDVendorURL,
			common.DeviceIDProductName,
			common.DeviceIDModelName,
			common.DeviceIDUserAppName,
		)
	} else if readDeviceIDCode == common.ReadDeviceIDExtendedStream {
		// Include all objects (basic + regular + extended)
		objectsToInclude = append(objectsToInclude,
			common.DeviceIDVendorURL,
			common.DeviceIDProductName,
			common.DeviceIDModelName,
			common.DeviceIDUserAppName,
			// Add any extended objects here (0x80-0xFF)
			common.DeviceIDObjectCode(0x80), // Example extended object
		)
	}

	// Fixed object values for this server
	objectValues := map[common.DeviceIDObjectCode]string{
		// Basic identification objects (mandatory)
		common.DeviceIDVendorName:         "gomodbus",
		common.DeviceIDProductCode:        "GM-001",
		common.DeviceIDMajorMinorRevision: "1.0",

		// Regular identification objects (optional)
		common.DeviceIDVendorURL:   "https://github.com/Moonlight-Companies/gomodbus",
		common.DeviceIDProductName: "gomodbus Server",
		common.DeviceIDModelName:   "Modbus TCP Server",
		common.DeviceIDUserAppName: "Example Server",

		// Extended identification objects (vendor-specific)
		common.DeviceIDObjectCode(0x80): "Extended Object Example",
	}

	// Add objects to response
	for _, id := range objectsToInclude {
		value, exists := objectValues[id]
		if exists {
			deviceID.Objects = append(deviceID.Objects, common.DeviceIDObject{
				ID:     id,
				Length: byte(len(value)),
				Value:  value,
			})
		}
	}

	deviceID.NumberOfObjects = byte(len(deviceID.Objects))

	// Encode response
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU)
	// Response format:
	// Byte 0: MEI Type (0x0E)
	// Byte 1: ReadDeviceID code
	// Byte 2: Conformity level
	// Byte 3: More follows (0 or 1)
	// Byte 4: Next object ID
	// Byte 5: Number of objects
	// For each object:
	//   Byte n+0: Object ID
	//   Byte n+1: Object length
	//   Byte n+2..n+1+length: Object value

	// Calculate response size
	responseSize := 6 // Fixed header
	for _, obj := range deviceID.Objects {
		responseSize += 2 + int(obj.Length) // ID + length + value
	}

	responseData := make([]byte, responseSize)
	responseData[0] = byte(common.MEIReadDeviceID)
	responseData[1] = byte(deviceID.ReadDeviceIDCode)
	responseData[2] = byte(deviceID.ConformityLevel)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU)
	// MoreFollows: 0x00 = no more objects, 0xFF = more objects available (request again with NextObjectID)
	responseData[3] = byte(deviceID.MoreFollows)
	responseData[4] = byte(deviceID.NextObjectID)
	responseData[5] = deviceID.NumberOfObjects

	// Add objects
	offset := 6
	for _, obj := range deviceID.Objects {
		responseData[offset] = byte(obj.ID)
		responseData[offset+1] = obj.Length
		copy(responseData[offset+2:offset+2+int(obj.Length)], []byte(obj.Value))
		offset += 2 + int(obj.Length)
	}

	// Create the response
	response := transport.NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}
