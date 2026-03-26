package transport

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// Request implements the common.Request interface
type Request struct {
	TransactionID common.TransactionID
	ProtocolID    common.ProtocolID
	UnitID        common.UnitID
	PDU           *common.PDU
	Create        time.Time
}

// NewRequest creates a new Request
func NewRequest(unitID common.UnitID, functionCode common.FunctionCode, data []byte) *Request {
	return &Request{
		ProtocolID: common.TCPProtocolIdentifier,
		UnitID:     unitID,
		PDU: &common.PDU{
			FunctionCode: functionCode,
			Data:         data,
		},
		Create: time.Now(),
	}
}

// GetTransactionID returns the transaction ID
func (r *Request) GetTransactionID() common.TransactionID {
	return r.TransactionID
}

// SetTransactionID sets the transaction ID
func (r *Request) SetTransactionID(id common.TransactionID) {
	r.TransactionID = id
}

// GetUnitID returns the unit ID
func (r *Request) GetUnitID() common.UnitID {
	return r.UnitID
}

// GetPDU returns the PDU
func (r *Request) GetPDU() *common.PDU {
	return r.PDU
}

// Encode encodes a Request into bytes
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header format)
func (r *Request) Encode() ([]byte, error) {
	// Calculate the length of the remaining data (Unit ID + PDU)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1
	// Length field = Unit ID (1 byte) + Function Code (1 byte) + Data (N bytes)
	length := uint16(1 + 1 + len(r.PDU.Data)) // Unit ID + Function Code + Data

	// Create a buffer to hold the encoded bytes
	buffer := bytes.Buffer{}

	// Write MBAP header - all multi-byte values use big-endian byte order
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1, Table 3 (MBAP Header)
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.3 (Data Encoding):
	// "Each MODBUS data type is packed into a 2 byte field in big-endian format:
	// the most significant byte is transmitted first."
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

// Decode decodes a Request from bytes
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header) and Section 6 (PDU format)
func (r *Request) Decode(data []byte) error {
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
	pduData := make([]byte, length-2) // -2 for UnitID and FunctionCode
	if _, err := io.ReadFull(buffer, pduData); err != nil {
		return err
	}

	r.PDU = &common.PDU{
		FunctionCode: common.FunctionCode(functionCode),
		Data:         pduData,
	}

	return nil
}

// GetLifetime returns the lifetime of the request
func (r *Request) GetLifetime() time.Duration {
	return time.Since(r.Create)
}

// Cancel is called when a transaction is cancelled
func (r *Request) Cancel(err error) {
	// Our transaction has timed out or some other error occurred
	// This method can be used for cleanup if needed
}