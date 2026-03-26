package cip

import (
	"bytes"
	"encoding/binary"
	"io"
)

// Symbol Object Class ID
const ClassSymbol UINT = 0x6B

// Symbol Instance Structure (partial, for listing)
type SymbolInstance struct {
	InstanceID uint32
	Name       string
	Type       DataType
}

// GetInstanceAttributeListRequest creates a request to list instances of a class
// This uses the "Get Instance Attribute List" service (0x55) if supported,
// or we might have to iterate using "Find Next Object Instance" (0x11).
// Logix controllers typically support iterating via GetInstanceAttributeList on the Symbol Class.
const ServiceGetInstanceAttributeList USINT = 0x55

// NewGetSymbolClassAttributesRequest creates a request to get attributes of the Symbol Class (Instance 0)
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

// DecodeSymbolClassAttributesResponse decodes the response from GetAttributeList on Class 0x6B
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

// NewGetSymbolAttributesRequest creates a request to get attributes for a specific symbol instance
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

// DecodeSymbolAttributesResponse decodes the response from GetAttributeList (0x03)
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
