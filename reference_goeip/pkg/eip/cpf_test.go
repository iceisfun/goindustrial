package eip

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestNewCPFItem(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	item := NewCPFItem(ItemIDUnconnectedMessage, data)

	if item.TypeID != ItemIDUnconnectedMessage {
		t.Errorf("TypeID = 0x%04X, want 0x%04X", item.TypeID, ItemIDUnconnectedMessage)
	}
	if item.Length != 4 {
		t.Errorf("Length = %d, want 4", item.Length)
	}
	if !bytes.Equal(item.Data, data) {
		t.Errorf("Data = %v, want %v", item.Data, data)
	}
}

func TestNewCPFItem_NilData(t *testing.T) {
	item := NewCPFItem(ItemIDNullAddress, nil)

	if item.TypeID != ItemIDNullAddress {
		t.Errorf("TypeID = 0x%04X, want 0x%04X", item.TypeID, ItemIDNullAddress)
	}
	if item.Length != 0 {
		t.Errorf("Length = %d, want 0", item.Length)
	}
}

func TestCPFItem_Encode(t *testing.T) {
	item := NewCPFItem(0x00B2, []byte{0xAA, 0xBB})
	buf := new(bytes.Buffer)

	err := item.Encode(buf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// TypeID (2) + Length (2) + Data (2) = 6 bytes
	if buf.Len() != 6 {
		t.Errorf("Encoded length = %d, want 6", buf.Len())
	}

	data := buf.Bytes()
	typeID := binary.LittleEndian.Uint16(data[0:2])
	length := binary.LittleEndian.Uint16(data[2:4])

	if typeID != 0x00B2 {
		t.Errorf("Encoded TypeID = 0x%04X, want 0x00B2", typeID)
	}
	if length != 2 {
		t.Errorf("Encoded Length = %d, want 2", length)
	}
	if data[4] != 0xAA || data[5] != 0xBB {
		t.Errorf("Encoded Data = %v, want [0xAA, 0xBB]", data[4:6])
	}
}

func TestCPFItem_Encode_EmptyData(t *testing.T) {
	item := NewCPFItem(ItemIDNullAddress, nil)
	buf := new(bytes.Buffer)

	err := item.Encode(buf)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// TypeID (2) + Length (2) = 4 bytes
	if buf.Len() != 4 {
		t.Errorf("Encoded length = %d, want 4", buf.Len())
	}
}

func TestNewCommonPacketFormat(t *testing.T) {
	item1 := NewCPFItem(ItemIDNullAddress, nil)
	item2 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x01, 0x02})

	cpf := NewCommonPacketFormat(item1, item2)

	if cpf.ItemCount != 2 {
		t.Errorf("ItemCount = %d, want 2", cpf.ItemCount)
	}
	if len(cpf.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(cpf.Items))
	}
}

func TestNewCommonPacketFormat_NoItems(t *testing.T) {
	cpf := NewCommonPacketFormat()

	if cpf.ItemCount != 0 {
		t.Errorf("ItemCount = %d, want 0", cpf.ItemCount)
	}
}

func TestCommonPacketFormat_Encode(t *testing.T) {
	item1 := NewCPFItem(ItemIDNullAddress, nil)
	item2 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x01, 0x02, 0x03})

	cpf := NewCommonPacketFormat(item1, item2)

	data, err := cpf.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// ItemCount (2) + Item1 (4) + Item2 (4 + 3) = 13 bytes
	expectedLen := 2 + 4 + 4 + 3
	if len(data) != expectedLen {
		t.Errorf("Encoded length = %d, want %d", len(data), expectedLen)
	}

	// Check item count
	itemCount := binary.LittleEndian.Uint16(data[0:2])
	if itemCount != 2 {
		t.Errorf("Encoded ItemCount = %d, want 2", itemCount)
	}
}

func TestDecodeCommonPacketFormat(t *testing.T) {
	// Build test data manually
	data := make([]byte, 0)

	// Item count = 2
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 2)
	data = append(data, itemCount...)

	// Item 1: Null Address
	item1Type := make([]byte, 2)
	binary.LittleEndian.PutUint16(item1Type, ItemIDNullAddress)
	data = append(data, item1Type...)
	item1Len := make([]byte, 2)
	binary.LittleEndian.PutUint16(item1Len, 0)
	data = append(data, item1Len...)

	// Item 2: Unconnected Message with data
	item2Type := make([]byte, 2)
	binary.LittleEndian.PutUint16(item2Type, ItemIDUnconnectedMessage)
	data = append(data, item2Type...)
	item2Data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	item2Len := make([]byte, 2)
	binary.LittleEndian.PutUint16(item2Len, uint16(len(item2Data)))
	data = append(data, item2Len...)
	data = append(data, item2Data...)

	cpf, err := DecodeCommonPacketFormat(data)
	if err != nil {
		t.Fatalf("DecodeCommonPacketFormat failed: %v", err)
	}

	if cpf.ItemCount != 2 {
		t.Errorf("ItemCount = %d, want 2", cpf.ItemCount)
	}
	if len(cpf.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(cpf.Items))
	}

	// Check first item
	if cpf.Items[0].TypeID != ItemIDNullAddress {
		t.Errorf("Items[0].TypeID = 0x%04X, want 0x%04X", cpf.Items[0].TypeID, ItemIDNullAddress)
	}
	if cpf.Items[0].Length != 0 {
		t.Errorf("Items[0].Length = %d, want 0", cpf.Items[0].Length)
	}

	// Check second item
	if cpf.Items[1].TypeID != ItemIDUnconnectedMessage {
		t.Errorf("Items[1].TypeID = 0x%04X, want 0x%04X", cpf.Items[1].TypeID, ItemIDUnconnectedMessage)
	}
	if cpf.Items[1].Length != 4 {
		t.Errorf("Items[1].Length = %d, want 4", cpf.Items[1].Length)
	}
	if !bytes.Equal(cpf.Items[1].Data, item2Data) {
		t.Errorf("Items[1].Data = %v, want %v", cpf.Items[1].Data, item2Data)
	}
}

func TestDecodeCommonPacketFormat_TooShort(t *testing.T) {
	// Only 1 byte (need at least 2 for item count)
	_, err := DecodeCommonPacketFormat([]byte{0x00})
	if err == nil {
		t.Error("Expected error for short data")
	}
}

func TestDecodeCommonPacketFormat_TruncatedItem(t *testing.T) {
	// Item count = 1, but no item data
	data := make([]byte, 2)
	binary.LittleEndian.PutUint16(data, 1)

	_, err := DecodeCommonPacketFormat(data)
	if err == nil {
		t.Error("Expected error for truncated item")
	}
}

func TestDecodeCommonPacketFormat_TruncatedItemData(t *testing.T) {
	data := make([]byte, 0)

	// Item count = 1
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 1)
	data = append(data, itemCount...)

	// Item with length but truncated data
	itemType := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemType, ItemIDUnconnectedMessage)
	data = append(data, itemType...)
	itemLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemLen, 100) // Claims 100 bytes
	data = append(data, itemLen...)
	data = append(data, []byte{0x01, 0x02}...) // Only 2 bytes

	_, err := DecodeCommonPacketFormat(data)
	if err == nil {
		t.Error("Expected error for truncated item data")
	}
}

func TestCommonPacketFormat_FindItemByType(t *testing.T) {
	item1 := NewCPFItem(ItemIDNullAddress, nil)
	item2 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x01, 0x02})
	item3 := NewCPFItem(ItemIDConnectedData, []byte{0x03, 0x04})

	cpf := NewCommonPacketFormat(item1, item2, item3)

	// Find existing item
	found := cpf.FindItemByType(ItemIDUnconnectedMessage)
	if found == nil {
		t.Fatal("Expected to find ItemIDUnconnectedMessage")
	}
	if !bytes.Equal(found.Data, []byte{0x01, 0x02}) {
		t.Errorf("Found wrong item data")
	}

	// Find another existing item
	found = cpf.FindItemByType(ItemIDConnectedData)
	if found == nil {
		t.Fatal("Expected to find ItemIDConnectedData")
	}
	if !bytes.Equal(found.Data, []byte{0x03, 0x04}) {
		t.Errorf("Found wrong item data")
	}

	// Find non-existing item
	found = cpf.FindItemByType(0xFFFF)
	if found != nil {
		t.Error("Should not find non-existing item type")
	}
}

func TestCommonPacketFormat_FindItemByType_FirstMatch(t *testing.T) {
	// Test that it returns the first matching item when duplicates exist
	item1 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x01})
	item2 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x02})

	cpf := NewCommonPacketFormat(item1, item2)

	found := cpf.FindItemByType(ItemIDUnconnectedMessage)
	if found == nil {
		t.Fatal("Expected to find item")
	}
	if !bytes.Equal(found.Data, []byte{0x01}) {
		t.Error("Should return first matching item")
	}
}

func TestCommonPacketFormat_RoundTrip(t *testing.T) {
	// Create original CPF
	item1 := NewCPFItem(ItemIDNullAddress, nil)
	item2 := NewCPFItem(ItemIDUnconnectedMessage, []byte{0x0E, 0x03, 0x20, 0x04, 0x24, 0x01, 0x30, 0x03})
	original := NewCommonPacketFormat(item1, item2)

	// Encode
	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded, err := DecodeCommonPacketFormat(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Compare
	if decoded.ItemCount != original.ItemCount {
		t.Errorf("ItemCount mismatch: got %d, want %d", decoded.ItemCount, original.ItemCount)
	}

	for i := range original.Items {
		if decoded.Items[i].TypeID != original.Items[i].TypeID {
			t.Errorf("Items[%d].TypeID mismatch", i)
		}
		if decoded.Items[i].Length != original.Items[i].Length {
			t.Errorf("Items[%d].Length mismatch", i)
		}
		if !bytes.Equal(decoded.Items[i].Data, original.Items[i].Data) {
			t.Errorf("Items[%d].Data mismatch", i)
		}
	}
}

func TestItemIDConstants(t *testing.T) {
	// Verify item ID constants match EtherNet/IP spec
	tests := []struct {
		name     string
		constant uint16
		expected uint16
	}{
		{"ItemIDNullAddress", ItemIDNullAddress, 0x0000},
		{"ItemIDListIdentity", ItemIDListIdentity, 0x000C},
		{"ItemIDConnectedAddress", ItemIDConnectedAddress, 0x00A1},
		{"ItemIDConnectedData", ItemIDConnectedData, 0x00B1},
		{"ItemIDUnconnectedMessage", ItemIDUnconnectedMessage, 0x00B2},
		{"ItemIDListServices", ItemIDListServices, 0x0100},
		{"ItemIDSockaddrInfo", ItemIDSockaddrInfo, 0x8000},
		{"ItemIDSequencedAddress", ItemIDSequencedAddress, 0x8002},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = 0x%04X, want 0x%04X", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
