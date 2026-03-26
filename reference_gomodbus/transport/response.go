package transport

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// Response implements the common.Response interface
type Response struct {
	TransactionID common.TransactionID
	ProtocolID    common.ProtocolID
	UnitID        common.UnitID
	PDU           *common.PDU
}

// NewResponse creates a new Response
func NewResponse(transactionID common.TransactionID, unitID common.UnitID, functionCode common.FunctionCode, data []byte) *Response {
	return &Response{
		TransactionID: transactionID,
		ProtocolID:    common.TCPProtocolIdentifier,
		UnitID:        unitID,
		PDU: &common.PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
	}
}

// GetTransactionID returns the transaction ID
func (r *Response) GetTransactionID() common.TransactionID {
	return r.TransactionID
}

// GetUnitID returns the unit ID
func (r *Response) GetUnitID() common.UnitID {
	return r.UnitID
}

// GetPDU returns the PDU
func (r *Response) GetPDU() *common.PDU {
	return r.PDU
}

// Encode encodes a Response into bytes
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
func (r *Response) Encode() ([]byte, error) {
	// Calculate the length of the remaining data (Unit ID + PDU)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1
	// Length field = Unit ID (1 byte) + Function Code (1 byte) + Data (N bytes)
	length := uint16(1 + 1 + len(r.PDU.Data)) // Unit ID + Function Code + Data

	// Create a buffer to hold the encoded bytes
	buffer := bytes.Buffer{}

	// Write MBAP header
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1, Table 3 (MBAP Header)
	if err := binary.Write(&buffer, binary.BigEndian, r.TransactionID); err != nil {
		return nil, err
	}
	if err := binary.Write(&buffer, binary.BigEndian, r.ProtocolID); err != nil {
		return nil, err
	}
	if err := binary.Write(&buffer, binary.BigEndian, length); err != nil {
		return nil, err
	}
	if err := binary.Write(&buffer, binary.BigEndian, r.UnitID); err != nil {
		return nil, err
	}

	// Write PDU
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (PDU)
	if err := binary.Write(&buffer, binary.BigEndian, r.PDU.FunctionCode); err != nil {
		return nil, err
	}
	if _, err := buffer.Write(r.PDU.Data); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// Decode decodes a Response from bytes
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header) and Section 6 (PDU format)
func (r *Response) Decode(data []byte) error {
	if len(data) < common.TCPHeaderLength {
		return common.ErrInvalidResponseLength
	}

	buffer := bytes.NewReader(data)

	// Read MBAP header
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1, Table 3
	// Field 1: Transaction Identifier (2 bytes)
	if err := binary.Read(buffer, binary.BigEndian, &r.TransactionID); err != nil {
		return err
	}
	// Field 2: Protocol Identifier (2 bytes)
	if err := binary.Read(buffer, binary.BigEndian, &r.ProtocolID); err != nil {
		return err
	}

	// Field 3: Length (2 bytes)
	var length uint16
	if err := binary.Read(buffer, binary.BigEndian, &length); err != nil {
		return err
	}

	// Field 4: Unit Identifier (1 byte)
	if err := binary.Read(buffer, binary.BigEndian, &r.UnitID); err != nil {
		return err
	}

	// Read PDU - Function Code (1 byte)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6
	functionCode := byte(0)
	if err := binary.Read(buffer, binary.BigEndian, &functionCode); err != nil {
		return err
	}

	// Read PDU - Data (variable)
	// Length field includes Unit ID (1) and Function Code (1)
	pduDataLength := int(length) - 2 // -2 for UnitID and FunctionCode
	if pduDataLength < 0 {
		return common.ErrInvalidResponseLength
	}

	pduData := make([]byte, pduDataLength)
	if _, err := io.ReadFull(buffer, pduData); err != nil {
		return err
	}

	r.PDU = &common.PDU{
		FunctionCode: common.FunctionCode(functionCode),
		Data:         pduData,
	}

	return nil
}

// IsException checks if the response is an exception
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) IsException() bool {
	return common.IsFunctionException(r.PDU.FunctionCode)
}

// GetException returns the exception code if the response is an exception
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) GetException() common.ExceptionCode {
	if r.IsException() && len(r.PDU.Data) > 0 {
		// For an exception response, the data field contains the exception code
		// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7
		return common.ExceptionCode(r.PDU.Data[0])
	}
	return 0
}

// ToError converts an exception response to an error
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) ToError() error {
	if r.IsException() {
		return common.NewModbusError(r.PDU.FunctionCode, r.GetException())
	}
	return nil
}
