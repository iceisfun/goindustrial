package cip

import (
	"encoding/binary"
	"fmt"
)

// Path Segment Types
const (
	SegmentTypePort      byte = 0x00 // 000xxxxx
	SegmentTypeLogical   byte = 0x20 // 001xxxxx
	SegmentTypeNetwork   byte = 0x40 // 010xxxxx
	SegmentTypeSymbolic  byte = 0x60 // 011xxxxx
	SegmentTypeData      byte = 0x80 // 100xxxxx
	SegmentTypeDataType1 byte = 0xA0 // 101xxxxx
	SegmentTypeDataType2 byte = 0xC0 // 110xxxxx
	SegmentTypeReserved  byte = 0xE0 // 111xxxxx
)

// Logical Segment Types
const (
	LogicalTypeClass     byte = 0x00 // 000xxxxx
	LogicalTypeInstance  byte = 0x04 // 001xxxxx
	LogicalTypeMember    byte = 0x08 // 010xxxxx
	LogicalTypePoint     byte = 0x0C // 011xxxxx
	LogicalTypeAttribute byte = 0x10 // 100xxxxx
	LogicalTypeSpecial   byte = 0x14 // 101xxxxx
	LogicalTypeService   byte = 0x18 // 110xxxxx
	LogicalTypeExtended  byte = 0x1C // 111xxxxx
)

// Logical Segment Formats
const (
	LogicalFormat8Bit     byte = 0x00 // xx00xxxx
	LogicalFormat16Bit    byte = 0x01 // xx01xxxx
	LogicalFormat32Bit    byte = 0x02 // xx10xxxx
	LogicalFormatReserved byte = 0x03 // xx11xxxx
)

// Path represents a CIP EPATH
type Path []byte

// NewPath creates a new empty path
func NewPath() Path {
	return make(Path, 0)
}

// AddClass adds a Class segment to the path
func (p *Path) AddClass(classID UINT) {
	if classID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeClass|LogicalFormat8Bit)
		*p = append(*p, byte(classID))
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypeClass|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(classID))
		*p = append(*p, b...)
	}
}

// AddInstance adds an Instance segment to the path
func (p *Path) AddInstance(instanceID UINT) {
	if instanceID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeInstance|LogicalFormat8Bit)
		*p = append(*p, byte(instanceID))
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypeInstance|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(instanceID))
		*p = append(*p, b...)
	}
}

// AddInstance32 adds a 32-bit Instance segment to the path
func (p *Path) AddInstance32(instanceID uint32) {
	if instanceID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeInstance|LogicalFormat8Bit)
		*p = append(*p, byte(instanceID))
	} else if instanceID <= 0xFFFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeInstance|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(instanceID))
		*p = append(*p, b...)
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypeInstance|LogicalFormat32Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, instanceID)
		*p = append(*p, b...)
	}
}

// AddAttribute adds an Attribute segment to the path
func (p *Path) AddAttribute(attributeID UINT) {
	if attributeID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeAttribute|LogicalFormat8Bit)
		*p = append(*p, byte(attributeID))
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypeAttribute|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(attributeID))
		*p = append(*p, b...)
	}
}

// AddMember adds a Member segment to the path
func (p *Path) AddMember(memberID UINT) {
	if memberID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypeMember|LogicalFormat8Bit)
		*p = append(*p, byte(memberID))
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypeMember|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(memberID))
		*p = append(*p, b...)
	}
}

// AddSymbolicSegment adds a Symbolic segment (ANSI Extended Symbol)
func (p *Path) AddSymbolicSegment(symbol string) {
	*p = append(*p, 0x91) // Extended Symbol Segment (Data Segment 0x80 | 0x11)
	l := len(symbol)
	*p = append(*p, byte(l))
	*p = append(*p, []byte(symbol)...)
	if l%2 != 0 {
		*p = append(*p, 0x00) // Pad to even length
	}
}

// AddPortSegment adds a Port segment
func (p *Path) AddPortSegment(port UINT, linkAddress []byte) {
	segStart := len(*p)

	var b byte
	if port < 15 {
		b = SegmentTypePort | byte(port)
	} else {
		// Extended port: set port nibble to 0x0F
		b = SegmentTypePort | 0x0F
	}

	if len(linkAddress) > 1 {
		b |= 0x10 // Extended link address bit
	}

	*p = append(*p, b)

	if port >= 15 {
		// Write the actual port number as a uint16
		portBytes := make([]byte, 2)
		binary.LittleEndian.PutUint16(portBytes, uint16(port))
		*p = append(*p, portBytes...)
	}

	if len(linkAddress) > 1 {
		// Write link address length byte
		*p = append(*p, byte(len(linkAddress)))
	}

	*p = append(*p, linkAddress...)

	// Pad to 16-bit word boundary if the segment length is odd
	if (len(*p)-segStart)%2 != 0 {
		*p = append(*p, 0x00)
	}
}

// Bytes returns the byte slice of the path
func (p Path) Bytes() []byte {
	return []byte(p)
}

// Len returns the length in words (16-bit)
func (p Path) LenWords() byte {
	return byte((len(p) + 1) / 2)
}

// String returns a string representation of the path
func (p Path) String() string {
	return fmt.Sprintf("%X", []byte(p))
}

// BuildPath creates a standard Class/Instance/Attribute path
func BuildPath(classID, instanceID, attributeID UINT) Path {
	p := NewPath()
	p.AddClass(classID)
	p.AddInstance(instanceID)
	if attributeID != 0 {
		p.AddAttribute(attributeID)
	}
	return p
}
