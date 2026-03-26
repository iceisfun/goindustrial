package modbus

import (
	"context"
	"encoding/binary"
	"math"
)

// serverProtocolHandler processes Modbus requests and generates responses.
type serverProtocolHandler struct{}

// newServerProtocolHandler creates a new protocol handler for server.
func newServerProtocolHandler() *serverProtocolHandler {
	return &serverProtocolHandler{}
}

// handleReadBitValues is a helper function for handling bit value read requests (coils, discrete inputs).
// This handles both Read Coils (0x01) and Read Discrete Inputs (0x02) functions.
func (h *serverProtocolHandler) handleReadBitValues(
	ctx context.Context,
	req *Request,
	store DataStore,
	itemType string,
	maxQuantity Quantity,
	readFunc func(context.Context, Address, Quantity) ([]bool, error),
) (*Response, error) {

	// Parse request PDU data: Starting Address (2 bytes), Quantity (2 bytes)
	if len(req.GetPDU().Data) != 4 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	if quantity == 0 || quantity > maxQuantity {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	values, err := readFunc(ctx, address, quantity)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	// Pack bit values into bytes - LSB of first byte corresponds to lowest address
	byteCount := int(math.Ceil(float64(quantity) / 8.0))
	responseData := make([]byte, 1+byteCount)
	responseData[0] = byte(byteCount)

	for i, value := range values {
		if value {
			byteIndex := i / 8
			bitOffset := i % 8
			responseData[1+byteIndex] |= (1 << uint(bitOffset))
		}
	}

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// handleReadRegisterValues is a helper function for handling register read requests (holding/input registers).
// This handles both Read Holding Registers (0x03) and Read Input Registers (0x04) functions.
func (h *serverProtocolHandler) handleReadRegisterValues(
	ctx context.Context,
	req *Request,
	store DataStore,
	itemType string,
	maxQuantity Quantity,
	readFunc func(context.Context, Address, Quantity) ([]uint16, error),
) (*Response, error) {

	// Parse request PDU data: Starting Address (2 bytes), Quantity (2 bytes)
	if len(req.GetPDU().Data) != 4 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	if quantity == 0 || quantity > maxQuantity {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	values, err := readFunc(ctx, address, quantity)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	byteCount := len(values) * 2
	responseData := make([]byte, 1+byteCount)
	responseData[0] = byte(byteCount)

	for i, value := range values {
		binary.BigEndian.PutUint16(responseData[1+i*2:1+i*2+2], value)
	}

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadCoils processes a read coils request.
func (h *serverProtocolHandler) HandleReadCoils(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	return h.handleReadBitValues(
		ctx, req, store, "coils", MaxCoilCount, store.ReadCoils,
	)
}

// HandleReadDiscreteInputs processes a read discrete inputs request.
func (h *serverProtocolHandler) HandleReadDiscreteInputs(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	return h.handleReadBitValues(
		ctx, req, store, "discrete inputs", MaxCoilCount, store.ReadDiscreteInputs,
	)
}

// HandleReadHoldingRegisters processes a read holding registers request.
func (h *serverProtocolHandler) HandleReadHoldingRegisters(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	return h.handleReadRegisterValues(
		ctx, req, store, "holding registers", MaxRegisterCount, store.ReadHoldingRegisters,
	)
}

// HandleReadInputRegisters processes a read input registers request.
func (h *serverProtocolHandler) HandleReadInputRegisters(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	return h.handleReadRegisterValues(
		ctx, req, store, "input registers", MaxRegisterCount, store.ReadInputRegisters,
	)
}

// HandleWriteSingleCoil processes a write single coil request.
func (h *serverProtocolHandler) HandleWriteSingleCoil(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) != 4 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	value := binary.BigEndian.Uint16(req.GetPDU().Data[2:4])

	var coilValue CoilValue
	if value == CoilOnU16 {
		coilValue = true
	} else if value == CoilOffU16 {
		coilValue = false
	} else {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	err := store.WriteSingleCoil(ctx, address, coilValue)
	if err != nil {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	// Echo the request
	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		req.GetPDU().Data,
	)

	return response, nil
}

// HandleWriteSingleRegister processes a write single register request.
func (h *serverProtocolHandler) HandleWriteSingleRegister(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) != 4 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	value := RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))

	err := store.WriteSingleRegister(ctx, address, value)
	if err != nil {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	// Echo the request
	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		req.GetPDU().Data,
	)

	return response, nil
}

// HandleWriteMultipleCoils processes a write multiple coils request.
func (h *serverProtocolHandler) HandleWriteMultipleCoils(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) < 5 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	byteCount := int(req.GetPDU().Data[4])

	if len(req.GetPDU().Data) != 5+byteCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if quantity == 0 || quantity > MaxWriteCoilCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	expectedByteCount := int(math.Ceil(float64(quantity) / 8.0))
	if byteCount != expectedByteCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	values := make([]CoilValue, quantity)
	for i := uint16(0); i < uint16(quantity); i++ {
		byteIndex := i / 8
		bitOffset := i % 8
		values[i] = (req.GetPDU().Data[5+byteIndex]>>uint(bitOffset))&0x01 != 0
	}

	err := store.WriteMultipleCoils(ctx, address, values)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	responseData := make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], uint16(quantity))

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleWriteMultipleRegisters processes a write multiple registers request.
func (h *serverProtocolHandler) HandleWriteMultipleRegisters(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) < 5 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	address := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	quantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	byteCount := int(req.GetPDU().Data[4])

	if len(req.GetPDU().Data) != 5+byteCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if quantity == 0 || quantity > MaxWriteRegisterCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if byteCount != int(quantity)*2 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	values := make([]RegisterValue, quantity)
	for i := uint16(0); i < uint16(quantity); i++ {
		values[i] = RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[5+i*2 : 5+i*2+2]))
	}

	err := store.WriteMultipleRegisters(ctx, address, values)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	responseData := make([]byte, 4)
	binary.BigEndian.PutUint16(responseData[0:2], uint16(address))
	binary.BigEndian.PutUint16(responseData[2:4], uint16(quantity))

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadWriteMultipleRegisters processes a read/write multiple registers request.
func (h *serverProtocolHandler) HandleReadWriteMultipleRegisters(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) < 9 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	readAddress := Address(binary.BigEndian.Uint16(req.GetPDU().Data[0:2]))
	readQuantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[2:4]))
	writeAddress := Address(binary.BigEndian.Uint16(req.GetPDU().Data[4:6]))
	writeQuantity := Quantity(binary.BigEndian.Uint16(req.GetPDU().Data[6:8]))
	byteCount := int(req.GetPDU().Data[8])

	if len(req.GetPDU().Data) != 9+byteCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if readQuantity == 0 || readQuantity > MaxReadWriteReadCount ||
		writeQuantity == 0 || writeQuantity > MaxReadWriteWriteCount {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if byteCount != int(writeQuantity)*2 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	writeValues := make([]RegisterValue, writeQuantity)
	for i := uint16(0); i < uint16(writeQuantity); i++ {
		writeValues[i] = RegisterValue(binary.BigEndian.Uint16(req.GetPDU().Data[9+i*2 : 9+i*2+2]))
	}

	// Write operation is performed before the read operation
	err := store.WriteMultipleRegisters(ctx, writeAddress, writeValues)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	readValues, err := store.ReadHoldingRegisters(ctx, readAddress, readQuantity)
	if err != nil {
		if err == ErrInvalidQuantity {
			return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
		}
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionServerDeviceFailure)
	}

	respByteCount := len(readValues) * 2
	responseData := make([]byte, 1+respByteCount)
	responseData[0] = byte(respByteCount)

	for i, value := range readValues {
		binary.BigEndian.PutUint16(responseData[1+i*2:1+i*2+2], value)
	}

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}

// HandleReadDeviceIdentification processes a read device identification request.
func (h *serverProtocolHandler) HandleReadDeviceIdentification(ctx context.Context, req *Request, store DataStore) (*Response, error) {
	if len(req.GetPDU().Data) < 3 {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	if MEIType(req.GetPDU().Data[0]) != MEIReadDeviceID {
		return nil, NewModbusError(req.GetPDU().FunctionCode, ExceptionInvalidDataValue)
	}

	readDeviceIDCode := ReadDeviceIDCode(req.GetPDU().Data[1])
	objectID := DeviceIDObjectCode(req.GetPDU().Data[2])

	deviceID := &DeviceIdentification{
		ReadDeviceIDCode: readDeviceIDCode,
		ConformityLevel:  ConformityLevelBasic,
		MoreFollows:      MoreFollowsNo,
		NextObjectID:     0x00,
		NumberOfObjects:  0,
		Objects:          make([]DeviceIDObject, 0),
	}

	objectsToInclude := []DeviceIDObjectCode{
		DeviceIDVendorName,
		DeviceIDProductCode,
		DeviceIDMajorMinorRevision,
	}

	if readDeviceIDCode == ReadDeviceIDSpecificObject {
		objectsToInclude = []DeviceIDObjectCode{objectID}
	} else if readDeviceIDCode == ReadDeviceIDRegularStream {
		objectsToInclude = append(objectsToInclude,
			DeviceIDVendorURL,
			DeviceIDProductName,
			DeviceIDModelName,
			DeviceIDUserAppName,
		)
	} else if readDeviceIDCode == ReadDeviceIDExtendedStream {
		objectsToInclude = append(objectsToInclude,
			DeviceIDVendorURL,
			DeviceIDProductName,
			DeviceIDModelName,
			DeviceIDUserAppName,
			DeviceIDObjectCode(0x80),
		)
	}

	objectValues := map[DeviceIDObjectCode]string{
		DeviceIDVendorName:         "goindustrial",
		DeviceIDProductCode:        "GI-001",
		DeviceIDMajorMinorRevision: "1.0",
		DeviceIDVendorURL:          "https://github.com/iceisfun/goindustrial",
		DeviceIDProductName:        "goindustrial Modbus Server",
		DeviceIDModelName:          "Modbus TCP Server",
		DeviceIDUserAppName:        "Modbus Server",
		DeviceIDObjectCode(0x80):   "Extended Object",
	}

	for _, id := range objectsToInclude {
		value, exists := objectValues[id]
		if exists {
			deviceID.Objects = append(deviceID.Objects, DeviceIDObject{
				ID:     id,
				Length: byte(len(value)),
				Value:  value,
			})
		}
	}

	deviceID.NumberOfObjects = byte(len(deviceID.Objects))

	// Encode response
	responseSize := 6 // Fixed header
	for _, obj := range deviceID.Objects {
		responseSize += 2 + int(obj.Length)
	}

	responseData := make([]byte, responseSize)
	responseData[0] = byte(MEIReadDeviceID)
	responseData[1] = byte(deviceID.ReadDeviceIDCode)
	responseData[2] = byte(deviceID.ConformityLevel)
	responseData[3] = byte(deviceID.MoreFollows)
	responseData[4] = byte(deviceID.NextObjectID)
	responseData[5] = deviceID.NumberOfObjects

	offset := 6
	for _, obj := range deviceID.Objects {
		responseData[offset] = byte(obj.ID)
		responseData[offset+1] = obj.Length
		copy(responseData[offset+2:offset+2+int(obj.Length)], []byte(obj.Value))
		offset += 2 + int(obj.Length)
	}

	response := NewResponse(
		req.GetTransactionID(),
		req.GetUnitID(),
		req.GetPDU().FunctionCode,
		responseData,
	)

	return response, nil
}
