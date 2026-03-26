package test

import (
	"github.com/Moonlight-Companies/gomodbus/common"
)

// MockRequest implements common.Request for testing
type MockRequest struct {
	TransactionID common.TransactionID
	UnitID        common.UnitID
	PDU           *common.PDU
}

func NewMockRequest(transactionID common.TransactionID, unitID common.UnitID, functionCode common.FunctionCode, data []byte) *MockRequest {
	return &MockRequest{
		TransactionID: transactionID,
		UnitID:        unitID,
		PDU: &common.PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
	}
}

func (r *MockRequest) GetTransactionID() common.TransactionID {
	return r.TransactionID
}

func (r *MockRequest) SetTransactionID(id common.TransactionID) {
	r.TransactionID = id
}

func (r *MockRequest) GetUnitID() common.UnitID {
	return r.UnitID
}

func (r *MockRequest) GetPDU() *common.PDU {
	return r.PDU
}

func (r *MockRequest) Encode() ([]byte, error) {
	// Simple encoding for testing - not used in actual tests
	result := make([]byte, 7+len(r.PDU.Data))

	// Transaction ID (2 bytes)
	result[0] = byte(r.TransactionID >> 8)
	result[1] = byte(r.TransactionID)

	// Protocol ID (2 bytes) - always 0 for Modbus TCP
	result[2] = 0
	result[3] = 0

	// Length (2 bytes) - length of remaining data
	length := 1 + len(r.PDU.Data) + 1 // Function code + data + unit ID
	result[4] = byte(length >> 8)
	result[5] = byte(length)

	// Unit ID (1 byte)
	result[6] = byte(r.UnitID)

	// Function code (1 byte)
	result[7] = byte(r.PDU.FunctionCode)

	// Data
	copy(result[8:], r.PDU.Data)

	return result, nil
}

// MockResponse implements common.Response for testing
type MockResponse struct {
	TransactionID common.TransactionID
	UnitID        common.UnitID
	PDU           *common.PDU
}

func NewMockResponse(transactionID common.TransactionID, unitID common.UnitID, functionCode common.FunctionCode, data []byte) *MockResponse {
	return &MockResponse{
		TransactionID: transactionID,
		UnitID:        unitID,
		PDU: &common.PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
	}
}

func (r *MockResponse) GetTransactionID() common.TransactionID {
	return r.TransactionID
}

func (r *MockResponse) GetUnitID() common.UnitID {
	return r.UnitID
}

func (r *MockResponse) GetPDU() *common.PDU {
	return r.PDU
}

func (r *MockResponse) IsException() bool {
	return common.IsFunctionException(r.PDU.FunctionCode)
}

func (r *MockResponse) GetException() common.ExceptionCode {
	if !r.IsException() {
		return 0
	}

	if len(r.PDU.Data) > 0 {
		return common.ExceptionCode(r.PDU.Data[0])
	}

	return 0
}

func (r *MockResponse) ToError() error {
	if !r.IsException() {
		return nil
	}

	return common.NewModbusError(r.PDU.FunctionCode, r.GetException())
}

func (r *MockResponse) Encode() ([]byte, error) {
	// Simple encoding for testing - not used in actual tests
	result := make([]byte, 7+len(r.PDU.Data))

	// Transaction ID (2 bytes)
	result[0] = byte(r.TransactionID >> 8)
	result[1] = byte(r.TransactionID)

	// Protocol ID (2 bytes) - always 0 for Modbus TCP
	result[2] = 0
	result[3] = 0

	// Length (2 bytes) - length of remaining data
	length := 1 + len(r.PDU.Data) + 1 // Function code + data + unit ID
	result[4] = byte(length >> 8)
	result[5] = byte(length)

	// Unit ID (1 byte)
	result[6] = byte(r.UnitID)

	// Function code (1 byte)
	result[7] = byte(r.PDU.FunctionCode)

	// Data
	copy(result[8:], r.PDU.Data)

	return result, nil
}