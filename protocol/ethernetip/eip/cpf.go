package eip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// CPF Item IDs
const (
	ItemIDNullAddress        uint16 = 0x0000
	ItemIDListIdentity       uint16 = 0x000C
	ItemIDConnectionBased    uint16 = 0x00A1
	ItemIDConnectedAddress   uint16 = 0x00A1 // Alias for ConnectionBased
	ItemIDConnectedTransport uint16 = 0x00B1
	ItemIDConnectedData      uint16 = 0x00B1 // Alias for ConnectedTransport
	ItemIDUnconnectedMessage uint16 = 0x00B2
	ItemIDListServices       uint16 = 0x0100
	ItemIDSockaddrInfo       uint16 = 0x8000
	ItemIDSequencedAddress   uint16 = 0x8002
)

// CPFItem represents a single item in the Common Packet Format
type CPFItem struct {
	TypeID uint16
	Length uint16
	Data   []byte
}

// NewCPFItem creates a new CPF item
func NewCPFItem(typeID uint16, data []byte) CPFItem {
	return CPFItem{
		TypeID: typeID,
		Length: uint16(len(data)),
		Data:   data,
	}
}

// Encode writes the CPF item to the writer
func (item *CPFItem) Encode(w io.Writer) error {
	if err := binary.Write(w, binary.LittleEndian, item.TypeID); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, item.Length); err != nil {
		return err
	}
	if item.Length > 0 {
		if _, err := w.Write(item.Data); err != nil {
			return err
		}
	}
	return nil
}

// CommonPacketFormat represents a collection of CPF items
type CommonPacketFormat struct {
	ItemCount uint16
	Items     []CPFItem
}

// NewCommonPacketFormat creates a new CPF with given items
func NewCommonPacketFormat(items ...CPFItem) *CommonPacketFormat {
	return &CommonPacketFormat{
		ItemCount: uint16(len(items)),
		Items:     items,
	}
}

// Encode encodes the entire CPF structure
func (cpf *CommonPacketFormat) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, cpf.ItemCount); err != nil {
		return nil, err
	}
	for _, item := range cpf.Items {
		if err := item.Encode(buf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeCommonPacketFormat decodes a CPF from a byte slice
func DecodeCommonPacketFormat(data []byte) (*CommonPacketFormat, error) {
	r := bytes.NewReader(data)
	cpf := &CommonPacketFormat{}

	if err := binary.Read(r, binary.LittleEndian, &cpf.ItemCount); err != nil {
		return nil, err
	}

	remaining := r.Len()
	if remaining < 4*int(cpf.ItemCount) {
		return nil, fmt.Errorf("cpf: item count %d requires at least %d bytes but only %d remain", cpf.ItemCount, 4*int(cpf.ItemCount), remaining)
	}

	for i := 0; i < int(cpf.ItemCount); i++ {
		var typeID, length uint16
		if err := binary.Read(r, binary.LittleEndian, &typeID); err != nil {
			return nil, err
		}
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return nil, err
		}

		itemData := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(r, itemData); err != nil {
				return nil, err
			}
		}

		cpf.Items = append(cpf.Items, CPFItem{
			TypeID: typeID,
			Length: length,
			Data:   itemData,
		})
	}

	return cpf, nil
}

// FindItemByType returns the first item with the given TypeID
func (cpf *CommonPacketFormat) FindItemByType(typeID uint16) *CPFItem {
	for i := range cpf.Items {
		if cpf.Items[i].TypeID == typeID {
			return &cpf.Items[i]
		}
	}
	return nil
}
