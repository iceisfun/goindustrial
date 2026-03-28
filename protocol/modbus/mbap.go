package modbus

import (
	"bytes"
	"encoding/binary"
)

// MBAP (Modbus Application Protocol) header identifies and frames a Modbus TCP
// message. It prefixes the PDU to form the complete Application Data Unit (ADU).
// The header is 7 bytes on the wire: Transaction ID (2) + Protocol ID (2) +
// Length (2) + Unit ID (1).
//
// Ref: MODBUS Messaging on TCP/IP Implementation Guide, Section 3.1.3.
type MBAP struct {
	TransactionID TransactionID // 2 bytes, correlates requests with responses
	ProtocolID    ProtocolID    // 2 bytes, 0x0000 for Modbus TCP
	UnitID        UnitID        // 1 byte, remote unit address
}

// Encode serialises the MBAP header into the given buffer using big-endian byte
// order. The length field covers the Unit ID (1 byte) plus the pduLength bytes
// that follow.
func (m *MBAP) Encode(buf *bytes.Buffer, pduLength int) error {
	// Length field = Unit ID (1 byte) + PDU (function code + data)
	length := uint16(1 + pduLength)

	if err := binary.Write(buf, binary.BigEndian, m.TransactionID); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, m.ProtocolID); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, m.UnitID); err != nil {
		return err
	}
	return nil
}

// Decode deserialises the MBAP header from the given reader using big-endian
// byte order and returns the length field value. The length field indicates how
// many bytes follow the header (Unit ID + PDU).
func (m *MBAP) Decode(buf *bytes.Reader) (uint16, error) {
	if err := binary.Read(buf, binary.BigEndian, &m.TransactionID); err != nil {
		return 0, err
	}
	if err := binary.Read(buf, binary.BigEndian, &m.ProtocolID); err != nil {
		return 0, err
	}

	var length uint16
	if err := binary.Read(buf, binary.BigEndian, &length); err != nil {
		return 0, err
	}

	if err := binary.Read(buf, binary.BigEndian, &m.UnitID); err != nil {
		return 0, err
	}
	return length, nil
}
