package eip

import (
	"bytes"
	"encoding/binary"
	"io"
)

// ListIdentityItem represents an item in the ListIdentity response
type ListIdentityItem struct {
	TypeID        uint16
	Length        uint16
	EncapsVersion uint16
	SocketAddr    [16]byte // struct sockaddr_in
	VendorID      uint16
	DeviceType    uint16
	ProductCode   uint16
	Revision      [2]byte // Major, Minor
	Status        uint16
	SerialNumber  uint32
	ProductName   string // Max 32 chars
	State         uint8
}

// ListServicesItem represents an item in the ListServices response
type ListServicesItem struct {
	TypeID          uint16
	Length          uint16
	Version         uint16
	CapabilityFlags uint16
	Name            string // 16 bytes fixed
}

// DecodeListServicesItem decodes a single service item
func DecodeListServicesItem(r io.Reader) (*ListServicesItem, error) {
	item := &ListServicesItem{}
	if err := binary.Read(r, binary.LittleEndian, &item.TypeID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &item.Length); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &item.Version); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &item.CapabilityFlags); err != nil {
		return nil, err
	}

	nameBytes := make([]byte, 16)
	if _, err := io.ReadFull(r, nameBytes); err != nil {
		return nil, err
	}
	// Trim null bytes
	item.Name = string(bytes.TrimRight(nameBytes, "\x00"))

	return item, nil
}

// DecodeListIdentityResponse decodes the full response data from ListIdentity
func DecodeListIdentityResponse(data []byte) ([]ListIdentityItem, error) {
	r := bytes.NewReader(data)
	var count uint16
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, err
	}

	items := make([]ListIdentityItem, 0, count)
	for i := 0; i < int(count); i++ {
		// Read Type and Length first
		var typeID uint16
		if err := binary.Read(r, binary.LittleEndian, &typeID); err != nil {
			return nil, err
		}
		var length uint16
		if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
			return nil, err
		}

		if typeID == ItemIDListIdentity {
			// CIP Identity Item
			item := ListIdentityItem{
				TypeID: typeID,
				Length: length,
			}
			// Decode remaining fields
			if err := binary.Read(r, binary.LittleEndian, &item.EncapsVersion); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.SocketAddr); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.VendorID); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.DeviceType); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.ProductCode); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.Revision); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.Status); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &item.SerialNumber); err != nil {
				return nil, err
			}

			// ProductName is a length-prefixed string (1 byte length)
			var nameLen uint8
			if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
				return nil, err
			}
			nameBytes := make([]byte, nameLen)
			if _, err := io.ReadFull(r, nameBytes); err != nil {
				return nil, err
			}
			item.ProductName = string(nameBytes)

			if err := binary.Read(r, binary.LittleEndian, &item.State); err != nil {
				return nil, err
			}
			items = append(items, item)
		} else {
			// Unknown Item Type, skip data
			skip := make([]byte, length)
			if _, err := io.ReadFull(r, skip); err != nil {
				return nil, err
			}
		}
	}
	return items, nil
}

// DecodeListServicesResponse decodes the full response data from ListServices
func DecodeListServicesResponse(data []byte) ([]ListServicesItem, error) {
	r := bytes.NewReader(data)
	var count uint16
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return nil, err
	}

	items := make([]ListServicesItem, 0, count)
	for i := 0; i < int(count); i++ {
		item, err := DecodeListServicesItem(r)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}
