package eip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// HeaderSize is the fixed size in bytes of an EtherNet/IP encapsulation header.
const HeaderSize = 24

// SessionHandle is a 32-bit handle assigned by the target device during
// session registration. It must be included in all subsequent encapsulation
// commands for the lifetime of the session.
type SessionHandle uint32

// EncapsulationHeader represents the 24-byte EtherNet/IP encapsulation header
// that prefixes every EIP message on the wire.
type EncapsulationHeader struct {
	Command       Command
	Length        uint16 // Length of the data following the header
	SessionHandle SessionHandle
	Status        uint32
	SenderContext [8]byte
	Options       uint32
}

// Encode writes the 24-byte header to w in little-endian byte order.
func (h *EncapsulationHeader) Encode(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// Decode reads and parses a 24-byte header from r.
func (h *EncapsulationHeader) Decode(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, h)
}

// Bytes returns the header serialized as a 24-byte slice.
func (h *EncapsulationHeader) Bytes() []byte {
	buf := new(bytes.Buffer)
	h.Encode(buf)
	return buf.Bytes()
}

// String returns a human-readable representation of the header fields.
func (h *EncapsulationHeader) String() string {
	return fmt.Sprintf("Cmd: %s (0x%04X), Len: %d, Session: 0x%08X, Status: 0x%08X",
		h.Command, uint16(h.Command), h.Length, h.SessionHandle, h.Status)
}
