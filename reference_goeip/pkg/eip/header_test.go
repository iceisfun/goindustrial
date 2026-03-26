package eip

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncapsulationHeader_Encode(t *testing.T) {
	header := &EncapsulationHeader{
		Command:       CommandRegisterSession,
		Length:        4,
		SessionHandle: 0x12345678,
		Status:        StatusSuccess,
		SenderContext: [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
		Options:       0,
	}

	buf := new(bytes.Buffer)
	err := header.Encode(buf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Header is always 24 bytes
	if buf.Len() != HeaderSize {
		t.Errorf("Encoded length = %d, want %d", buf.Len(), HeaderSize)
	}

	data := buf.Bytes()

	// Check command
	cmd := binary.LittleEndian.Uint16(data[0:2])
	if Command(cmd) != CommandRegisterSession {
		t.Errorf("Command = 0x%04X, want 0x%04X", cmd, CommandRegisterSession)
	}

	// Check length
	length := binary.LittleEndian.Uint16(data[2:4])
	if length != 4 {
		t.Errorf("Length = %d, want 4", length)
	}

	// Check session handle
	session := binary.LittleEndian.Uint32(data[4:8])
	if session != 0x12345678 {
		t.Errorf("SessionHandle = 0x%08X, want 0x12345678", session)
	}

	// Check status
	status := binary.LittleEndian.Uint32(data[8:12])
	if status != 0 {
		t.Errorf("Status = 0x%08X, want 0", status)
	}

	// Check sender context
	for i := 0; i < 8; i++ {
		if data[12+i] != header.SenderContext[i] {
			t.Errorf("SenderContext[%d] = 0x%02X, want 0x%02X", i, data[12+i], header.SenderContext[i])
		}
	}

	// Check options
	options := binary.LittleEndian.Uint32(data[20:24])
	if options != 0 {
		t.Errorf("Options = 0x%08X, want 0", options)
	}
}

func TestEncapsulationHeader_Decode(t *testing.T) {
	// Build raw header bytes
	data := make([]byte, HeaderSize)
	binary.LittleEndian.PutUint16(data[0:2], uint16(CommandSendRRData))
	binary.LittleEndian.PutUint16(data[2:4], 100)
	binary.LittleEndian.PutUint32(data[4:8], 0xDEADBEEF)
	binary.LittleEndian.PutUint32(data[8:12], StatusInvalidSessionHandle)
	copy(data[12:20], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22})
	binary.LittleEndian.PutUint32(data[20:24], 0x12340000)

	header := &EncapsulationHeader{}
	err := header.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if header.Command != CommandSendRRData {
		t.Errorf("Command = 0x%04X, want 0x%04X", header.Command, CommandSendRRData)
	}
	if header.Length != 100 {
		t.Errorf("Length = %d, want 100", header.Length)
	}
	if header.SessionHandle != 0xDEADBEEF {
		t.Errorf("SessionHandle = 0x%08X, want 0xDEADBEEF", header.SessionHandle)
	}
	if header.Status != StatusInvalidSessionHandle {
		t.Errorf("Status = 0x%08X, want 0x%08X", header.Status, StatusInvalidSessionHandle)
	}
	expectedContext := [8]byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22}
	if header.SenderContext != expectedContext {
		t.Errorf("SenderContext = %v, want %v", header.SenderContext, expectedContext)
	}
	if header.Options != 0x12340000 {
		t.Errorf("Options = 0x%08X, want 0x12340000", header.Options)
	}
}

func TestEncapsulationHeader_Decode_TooShort(t *testing.T) {
	data := make([]byte, 10) // Too short

	header := &EncapsulationHeader{}
	err := header.Decode(bytes.NewReader(data))
	if err == nil {
		t.Error("Expected error for short data")
	}
}

func TestEncapsulationHeader_Bytes(t *testing.T) {
	header := &EncapsulationHeader{
		Command:       CommandListIdentity,
		Length:        0,
		SessionHandle: 0,
		Status:        0,
	}

	data := header.Bytes()

	if len(data) != HeaderSize {
		t.Errorf("Bytes length = %d, want %d", len(data), HeaderSize)
	}

	cmd := binary.LittleEndian.Uint16(data[0:2])
	if Command(cmd) != CommandListIdentity {
		t.Errorf("Command = 0x%04X, want 0x%04X", cmd, CommandListIdentity)
	}
}

func TestEncapsulationHeader_String(t *testing.T) {
	header := &EncapsulationHeader{
		Command:       CommandRegisterSession,
		Length:        4,
		SessionHandle: 0x12345678,
		Status:        StatusSuccess,
	}

	str := header.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	// Should contain command, length, session, status
	if !bytes.Contains([]byte(str), []byte("0x0065")) {
		t.Error("String should contain command")
	}
}

func TestEncapsulationHeader_RoundTrip(t *testing.T) {
	original := &EncapsulationHeader{
		Command:       CommandSendUnitData,
		Length:        256,
		SessionHandle: 0xCAFEBABE,
		Status:        StatusIncorrectData,
		SenderContext: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
		Options:       0x11223344,
	}

	// Encode
	buf := new(bytes.Buffer)
	err := original.Encode(buf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded := &EncapsulationHeader{}
	err = decoded.Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Compare
	if decoded.Command != original.Command {
		t.Errorf("Command mismatch")
	}
	if decoded.Length != original.Length {
		t.Errorf("Length mismatch")
	}
	if decoded.SessionHandle != original.SessionHandle {
		t.Errorf("SessionHandle mismatch")
	}
	if decoded.Status != original.Status {
		t.Errorf("Status mismatch")
	}
	if decoded.SenderContext != original.SenderContext {
		t.Errorf("SenderContext mismatch")
	}
	if decoded.Options != original.Options {
		t.Errorf("Options mismatch")
	}
}

func TestHeaderSizeConstant(t *testing.T) {
	if HeaderSize != 24 {
		t.Errorf("HeaderSize = %d, want 24", HeaderSize)
	}
}

func TestCommandConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant Command
		expected uint16
	}{
		{"CommandNop", CommandNop, 0x0000},
		{"CommandListServices", CommandListServices, 0x0004},
		{"CommandListIdentity", CommandListIdentity, 0x0063},
		{"CommandListInterfaces", CommandListInterfaces, 0x0064},
		{"CommandRegisterSession", CommandRegisterSession, 0x0065},
		{"CommandUnregisterSession", CommandUnregisterSession, 0x0066},
		{"CommandSendRRData", CommandSendRRData, 0x006F},
		{"CommandSendUnitData", CommandSendUnitData, 0x0070},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint16(tt.constant) != tt.expected {
				t.Errorf("%s = 0x%04X, want 0x%04X", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant uint32
		expected uint32
	}{
		{"StatusSuccess", StatusSuccess, 0x00000000},
		{"StatusInvalidCommand", StatusInvalidCommand, 0x00000001},
		{"StatusInsufficientMemory", StatusInsufficientMemory, 0x00000002},
		{"StatusIncorrectData", StatusIncorrectData, 0x00000003},
		{"StatusInvalidSessionHandle", StatusInvalidSessionHandle, 0x00000064},
		{"StatusInvalidLength", StatusInvalidLength, 0x00000065},
		{"StatusUnsupportedProtocol", StatusUnsupportedProtocol, 0x00000069},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = 0x%08X, want 0x%08X", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
