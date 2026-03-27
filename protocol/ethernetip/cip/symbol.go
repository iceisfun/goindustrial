package cip

import (
	"bytes"
	"encoding/binary"
	"io"
)

// ClassSymbol is the CIP class ID for the Symbol Object (0x6B), used to
// enumerate tags on Rockwell Logix controllers.
const ClassSymbol UINT = 0x6B

// SymbolInstance represents a single tag entry discovered by enumerating the
// Symbol Object class. It contains the instance ID, the tag name, and the CIP
// data type code.
type SymbolInstance struct {
	InstanceID uint32
	Name       string
	Type       DataType
}

// ServiceGetInstanceAttributeList is the CIP Get_Instance_Attribute_List
// service code (0x55).
const ServiceGetInstanceAttributeList USINT = 0x55

// NewGetSymbolClassAttributesRequest creates a Get_Attribute_List request
// targeting the Symbol Class object (instance 0) to retrieve the class
// revision and maximum instance ID.
func NewGetSymbolClassAttributesRequest() *MessageRouterRequest {
	p := NewPath()
	p.AddClass(ClassSymbol)
	p.AddInstance(0)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(2)) // 2 Attributes
	binary.Write(buf, binary.LittleEndian, uint16(1)) // Attr 1: Revision
	binary.Write(buf, binary.LittleEndian, uint16(2)) // Attr 2: Max Instance

	return &MessageRouterRequest{
		Service:     0x03, // GetAttributeList
		RequestPath: p,
		RequestData: buf.Bytes(),
	}
}

// DecodeSymbolClassAttributesResponse decodes the Get_Attribute_List response
// from the Symbol Class (0x6B, instance 0) and returns the class revision and
// the maximum instance ID.
func DecodeSymbolClassAttributesResponse(data []byte) (uint16, uint16, error) {
	r := bytes.NewReader(data)
	var count uint16
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return 0, 0, err
	}

	var revision uint16
	var maxInstance uint16

	for i := 0; i < int(count); i++ {
		var attrID uint16
		if err := binary.Read(r, binary.LittleEndian, &attrID); err != nil {
			return 0, 0, err
		}
		var status uint16
		if err := binary.Read(r, binary.LittleEndian, &status); err != nil {
			return 0, 0, err
		}

		if status == 0 {
			switch attrID {
			case 1: // Revision (UINT)
				if err := binary.Read(r, binary.LittleEndian, &revision); err != nil {
					return 0, 0, err
				}
			case 2: // Max Instance (UINT)
				if err := binary.Read(r, binary.LittleEndian, &maxInstance); err != nil {
					return 0, 0, err
				}
			}
		}
	}
	return revision, maxInstance, nil
}

// NewGetSymbolAttributesRequest creates a Get_Attribute_List request targeting
// a specific Symbol Object instance to retrieve its name and data type.
func NewGetSymbolAttributesRequest(instanceID uint32) *MessageRouterRequest {
	p := NewPath()
	p.AddClass(ClassSymbol)
	p.AddInstance32(instanceID)

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(2)) // 2 Attributes
	binary.Write(buf, binary.LittleEndian, uint16(1)) // Attr 1: Name
	binary.Write(buf, binary.LittleEndian, uint16(2)) // Attr 2: Type

	return &MessageRouterRequest{
		Service:     0x03, // GetAttributeList
		RequestPath: p,
		RequestData: buf.Bytes(),
	}
}

// DecodeSymbolAttributesResponse decodes the Get_Attribute_List response for a
// Symbol Object instance and returns the tag name and CIP data type code.
func DecodeSymbolAttributesResponse(data []byte) (string, DataType, error) {
	r := bytes.NewReader(data)
	var count uint16
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return "", 0, err
	}

	var name string
	var typeCode uint16

	for i := 0; i < int(count); i++ {
		var attrID uint16
		if err := binary.Read(r, binary.LittleEndian, &attrID); err != nil {
			return "", 0, err
		}
		var status uint16
		if err := binary.Read(r, binary.LittleEndian, &status); err != nil {
			return "", 0, err
		}

		if status == 0 {
			switch attrID {
			case 1: // Name (STRING)
				var nameLen uint16
				if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
					return "", 0, err
				}
				nameBytes := make([]byte, nameLen)
				if _, err := io.ReadFull(r, nameBytes); err != nil {
					return "", 0, err
				}
				name = string(nameBytes)
			case 2: // Type (UINT)
				if err := binary.Read(r, binary.LittleEndian, &typeCode); err != nil {
					return "", 0, err
				}
			}
		}
	}
	return name, DataType(typeCode), nil
}
