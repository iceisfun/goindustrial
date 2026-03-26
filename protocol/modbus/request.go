package modbus

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

// Request represents a Modbus TCP request with MBAP header and PDU.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
type Request struct {
	TransactionID TransactionID
	ProtocolID    ProtocolID
	UnitID        UnitID
	PDU           *PDU
	Create        time.Time
}

// NewRequest creates a new Request with the given unit ID, function code, and data.
func NewRequest(unitID UnitID, functionCode FunctionCode, data []byte) *Request {
	return &Request{
		ProtocolID: TCPProtocolIdentifier,
		UnitID:     unitID,
		PDU: &PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
		Create: time.Now(),
	}
}

// GetTransactionID returns the transaction ID.
func (r *Request) GetTransactionID() TransactionID {
	return r.TransactionID
}

// SetTransactionID sets the transaction ID.
func (r *Request) SetTransactionID(id TransactionID) {
	r.TransactionID = id
}

// GetUnitID returns the unit ID.
func (r *Request) GetUnitID() UnitID {
	return r.UnitID
}

// GetPDU returns the PDU.
func (r *Request) GetPDU() *PDU {
	return r.PDU
}

// Encode encodes a Request into bytes (MBAP header + PDU, big-endian).
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
func (r *Request) Encode() ([]byte, error) {
	// Length field = Unit ID (1 byte) + Function Code (1 byte) + Data (N bytes)
	length := uint16(1 + 1 + len(r.PDU.Data))

	buffer := bytes.Buffer{}

	// Write MBAP header - all multi-byte values use big-endian byte order
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

// Decode decodes a Request from bytes.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header) and Section 6 (PDU format)
func (r *Request) Decode(data []byte) error {
	if len(data) < TCPHeaderLength {
		return ErrInvalidResponseLength
	}

	buffer := bytes.NewReader(data)

	// Read MBAP header
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
	// Length field includes Unit ID (1) and Function Code (1)
	pduData := make([]byte, length-2)
	if _, err := io.ReadFull(buffer, pduData); err != nil {
		return err
	}

	r.PDU = &PDU{
		FunctionCode: FunctionCode(functionCode),
		Data:         pduData,
	}

	return nil
}

// GetLifetime returns the lifetime of the request.
func (r *Request) GetLifetime() time.Duration {
	return time.Since(r.Create)
}

// Cancel is called when a transaction is cancelled.
func (r *Request) Cancel(err error) {
	// Cleanup hook; currently a no-op.
}
