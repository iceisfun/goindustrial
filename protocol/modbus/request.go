package modbus

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"
)

// Request represents a Modbus TCP request. It carries the MBAP header fields
// (transaction ID, protocol ID, unit ID) and a [PDU] containing the function
// code and request-specific data.
type Request struct {
	TransactionID TransactionID
	ProtocolID    ProtocolID
	UnitID        UnitID
	PDU           *PDU
	Create        time.Time
}

// NewRequest creates a new Request with the given unit ID, function code, and
// PDU data. The transaction ID is left at zero and is assigned later by the
// [TransactionPool].
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

// Encode serialises the Request into a Modbus TCP frame (MBAP header + PDU)
// with big-endian byte order, ready to be written to a TCP connection.
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

// Decode deserialises a Modbus TCP frame (MBAP header + PDU) from the given
// byte slice and populates the Request fields.
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
	if length < 2 {
		return ErrInvalidRequestLength
	}
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

// GetLifetime returns the elapsed time since the request was created.
func (r *Request) GetLifetime() time.Duration {
	return time.Since(r.Create)
}

// Cancel is called when the owning transaction is cancelled. It is a cleanup
// hook; the default implementation is a no-op.
func (r *Request) Cancel(err error) {
	// Cleanup hook; currently a no-op.
}
