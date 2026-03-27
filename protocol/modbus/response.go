package modbus

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Response represents a Modbus TCP response. It carries the MBAP header fields
// (transaction ID, protocol ID, unit ID) and a [PDU] containing the function
// code and response-specific data. If the server replied with an exception, the
// function code will have its high bit set.
type Response struct {
	TransactionID TransactionID
	ProtocolID    ProtocolID
	UnitID        UnitID
	PDU           *PDU
}

// NewResponse creates a new Response with the given MBAP fields and PDU data.
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

// Encode serialises the Response into a Modbus TCP frame (MBAP header + PDU)
// with big-endian byte order, ready to be written to a TCP connection.
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

// Decode deserialises a Modbus TCP frame (MBAP header + PDU) from the given
// byte slice and populates the Response fields.
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

// IsException reports whether the response is a Modbus exception (i.e. the high
// bit of the function code is set).
func (r *Response) IsException() bool {
	return IsFunctionException(r.PDU.FunctionCode)
}

// GetException returns the [ExceptionCode] if the response is an exception, or
// 0 if it is a normal response.
func (r *Response) GetException() ExceptionCode {
	if r.IsException() && len(r.PDU.Data) > 0 {
		return ExceptionCode(r.PDU.Data[0])
	}
	return 0
}

// ToError converts an exception response to a [ModbusError]. If the response is
// not an exception, nil is returned.
func (r *Response) ToError() error {
	if r.IsException() {
		return NewModbusError(r.PDU.FunctionCode, r.GetException())
	}
	return nil
}
