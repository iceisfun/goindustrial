package eip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Encapsulation Header Size is always 24 bytes
const HeaderSize = 24

// SessionHandle is a handle for an EIP session
type SessionHandle uint32

// EncapsulationHeader represents the 24-byte EIP header
type EncapsulationHeader struct {
	Command       Command
	Length        uint16 // Length of the data following the header
	SessionHandle SessionHandle
	Status        uint32
	SenderContext [8]byte
	Options       uint32
}

// Encode writes the header to the writer
func (h *EncapsulationHeader) Encode(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, h)
}

// Decode reads the header from the reader
func (h *EncapsulationHeader) Decode(r io.Reader) error {
	return binary.Read(r, binary.LittleEndian, h)
}

// Bytes returns the byte slice of the header
func (h *EncapsulationHeader) Bytes() []byte {
	buf := new(bytes.Buffer)
	h.Encode(buf)
	return buf.Bytes()
}

// String returns a string representation of the header
func (h *EncapsulationHeader) String() string {
	return fmt.Sprintf("Cmd: %s (0x%04X), Len: %d, Session: 0x%08X, Status: 0x%08X",
		h.Command, uint16(h.Command), h.Length, h.SessionHandle, h.Status)
}
