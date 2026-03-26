package modbus

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Response represents a Modbus TCP response with MBAP header and PDU.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
type Response struct {
	TransactionID TransactionID
	ProtocolID    ProtocolID
	UnitID        UnitID
	PDU           *PDU
}

// NewResponse creates a new Response.
func NewResponse(transactionID TransactionID, unitID UnitID, functionCode FunctionCode, data []byte) *Response {
	return &Response{
		TransactionID: transactionID,
		ProtocolID:    TCPProtocolIdentifier,
		UnitID:        unitID,
		PDU: &PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
	}
}

// GetTransactionID returns the transaction ID.
func (r *Response) GetTransactionID() TransactionID {
	return r.TransactionID
}

// GetUnitID returns the unit ID.
func (r *Response) GetUnitID() UnitID {
	return r.UnitID
}

// GetPDU returns the PDU.
func (r *Response) GetPDU() *PDU {
	return r.PDU
}

// Encode encodes a Response into bytes (MBAP header + PDU, big-endian).
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
func (r *Response) Encode() ([]byte, error) {
	// Length field = Unit ID (1 byte) + Function Code (1 byte) + Data (N bytes)
	length := uint16(1 + 1 + len(r.PDU.Data))

	buffer := bytes.Buffer{}

	// Write MBAP header
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
	if err := binary.Write(&buffer, binary.BigEndian, r.PDU.FunctionCode); err != nil {
		return nil, err
	}
	if _, err := buffer.Write(r.PDU.Data); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// Decode decodes a Response from bytes.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header) and Section 6 (PDU format)
func (r *Response) Decode(data []byte) error {
	if len(data) < TCPHeaderLength {
		return ErrInvalidResponseLength
	}

	buffer := bytes.NewReader(data)

	if err := binary.Read(buffer, binary.BigEndian, &r.TransactionID); err != nil {
		return err
	}
	if err := binary.Read(buffer, binary.BigEndian, &r.ProtocolID); err != nil {
		return err
	}

	var length uint16
	if err := binary.Read(buffer, binary.BigEndian, &length); err != nil {
		return err
	}

	if err := binary.Read(buffer, binary.BigEndian, &r.UnitID); err != nil {
		return err
	}

	// Read PDU - Function Code (1 byte)
	functionCode := byte(0)
	if err := binary.Read(buffer, binary.BigEndian, &functionCode); err != nil {
		return err
	}

	// Read PDU - Data (variable)
	pduDataLength := int(length) - 2 // -2 for UnitID and FunctionCode
	if pduDataLength < 0 {
		return ErrInvalidResponseLength
	}

	pduData := make([]byte, pduDataLength)
	if _, err := io.ReadFull(buffer, pduData); err != nil {
		return err
	}

	r.PDU = &PDU{
		FunctionCode: FunctionCode(functionCode),
		Data:         pduData,
	}

	return nil
}

// IsException checks if the response is an exception.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) IsException() bool {
	return IsFunctionException(r.PDU.FunctionCode)
}

// GetException returns the exception code if the response is an exception.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) GetException() ExceptionCode {
	if r.IsException() && len(r.PDU.Data) > 0 {
		return ExceptionCode(r.PDU.Data[0])
	}
	return 0
}

// ToError converts an exception response to an error.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 7 (Exception Responses)
func (r *Response) ToError() error {
	if r.IsException() {
		return NewModbusError(r.PDU.FunctionCode, r.GetException())
	}
	return nil
}
