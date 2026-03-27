package eip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Command represents an EtherNet/IP encapsulation command code. Commands
// identify the type of message carried in an [EncapsulationHeader].
type Command uint16

// EtherNet/IP encapsulation command codes.
const (
	CommandNop               Command = 0x0000
	CommandListServices      Command = 0x0004
	CommandListIdentity      Command = 0x0063
	CommandListInterfaces    Command = 0x0064
	CommandRegisterSession   Command = 0x0065
	CommandUnregisterSession Command = 0x0066
	CommandSendRRData        Command = 0x006F
	CommandSendUnitData      Command = 0x0070
	CommandIndicateStatus    Command = 0x0072
	CommandCancel            Command = 0x0073
)

// String returns a human-readable name for the command.
func (c Command) String() string {
	switch c {
	case CommandNop:
		return "Nop"
	case CommandListServices:
		return "ListServices"
	case CommandListIdentity:
		return "ListIdentity"
	case CommandListInterfaces:
		return "ListInterfaces"
	case CommandRegisterSession:
		return "RegisterSession"
	case CommandUnregisterSession:
		return "UnregisterSession"
	case CommandSendRRData:
		return "SendRRData"
	case CommandSendUnitData:
		return "SendUnitData"
	case CommandIndicateStatus:
		return "IndicateStatus"
	case CommandCancel:
		return "Cancel"
	default:
		return fmt.Sprintf("UnknownCommand(0x%04X)", uint16(c))
	}
}

// EtherNet/IP encapsulation status codes returned in the
// [EncapsulationHeader.Status] field.
const (
	StatusSuccess              uint32 = 0x00000000
	StatusInvalidCommand       uint32 = 0x00000001
	StatusInsufficientMemory   uint32 = 0x00000002
	StatusIncorrectData        uint32 = 0x00000003
	StatusInvalidSessionHandle uint32 = 0x00000064
	StatusInvalidLength        uint32 = 0x00000065
	StatusUnsupportedProtocol  uint32 = 0x00000069
)

// RegisterSessionData represents the payload of a RegisterSession command,
// which initiates a session between a client and an EtherNet/IP device.
type RegisterSessionData struct {
	ProtocolVersion uint16
	OptionsFlags    uint16
}

// NewRegisterSessionData creates a RegisterSessionData with protocol version 1
// and no option flags.
func NewRegisterSessionData() *RegisterSessionData {
	return &RegisterSessionData{
		ProtocolVersion: 1,
		OptionsFlags:    0,
	}
}

// Encode serializes the RegisterSessionData into its wire format.
func (d *RegisterSessionData) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
