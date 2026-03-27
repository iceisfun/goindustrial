package cip

import (
	"encoding/binary"
	"fmt"
)

// EPATH segment type identifiers. The high three bits of the first byte of
// each path segment encode the segment type.
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

// Logical segment sub-types. These occupy bits 4-2 of a logical segment byte
// and specify what the segment addresses (class, instance, attribute, etc.).
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

// Logical segment format selectors. These occupy bits 1-0 of a logical
// segment byte and indicate whether the value is 8, 16, or 32 bits wide.
const (
	LogicalFormat8Bit     byte = 0x00 // xx00xxxx
	LogicalFormat16Bit    byte = 0x01 // xx01xxxx
	LogicalFormat32Bit    byte = 0x02 // xx10xxxx
	LogicalFormatReserved byte = 0x03 // xx11xxxx
)

// Path represents a CIP EPATH (Encoded Path). An EPATH is a variable-length
// byte sequence of typed segments that addresses CIP objects by class,
// instance, attribute, connection point, or symbolic tag name.
type Path []byte

// NewPath creates a new empty EPATH. Use the Add* methods to append segments.
func NewPath() Path {
	return make(Path, 0)
}

// AddClass appends a logical Class segment to the path. Values up to 0xFF use
// the compact 8-bit format; larger values use the 16-bit format.
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

// AddInstance appends a logical Instance segment to the path.
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

// AddInstance32 appends an Instance segment to the path, automatically
// choosing 8-bit, 16-bit, or 32-bit encoding based on the value.
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

// AddAttribute appends a logical Attribute segment to the path.
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

// AddConnectionPoint appends a logical Connection Point segment (0x2C/0x2D) to
// the path. Connection points identify assembly instances in Forward_Open
// requests.
func (p *Path) AddConnectionPoint(pointID UINT) {
	if pointID <= 0xFF {
		*p = append(*p, SegmentTypeLogical|LogicalTypePoint|LogicalFormat8Bit)
		*p = append(*p, byte(pointID))
	} else {
		*p = append(*p, SegmentTypeLogical|LogicalTypePoint|LogicalFormat16Bit)
		*p = append(*p, 0x00) // Pad
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(pointID))
		*p = append(*p, b...)
	}
}

// AddMember appends a logical Member segment to the path.
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

// AddSymbolicSegment appends an ANSI Extended Symbol segment to the path. This
// is the primary mechanism for addressing PLC tags by name (e.g. "Motor_Speed").
// The segment is automatically padded to a 16-bit word boundary.
func (p *Path) AddSymbolicSegment(symbol string) {
	*p = append(*p, 0x91) // Extended Symbol Segment (Data Segment 0x80 | 0x11)
	l := len(symbol)
	*p = append(*p, byte(l))
	*p = append(*p, []byte(symbol)...)
	if l%2 != 0 {
		*p = append(*p, 0x00) // Pad to even length
	}
}

// AddPortSegment appends a Port segment to the path, used for routing CIP
// messages through a backplane or network port to a downstream device.
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

// Bytes returns the raw encoded path bytes.
func (p Path) Bytes() []byte {
	return []byte(p)
}

// LenWords returns the path length in 16-bit words, as required by the CIP
// message router request format.
func (p Path) LenWords() byte {
	return byte((len(p) + 1) / 2)
}

// String returns a hex-encoded representation of the path bytes.
func (p Path) String() string {
	return fmt.Sprintf("%X", []byte(p))
}

// BuildPath creates a standard Class/Instance/Attribute EPATH. If attributeID
// is 0 the attribute segment is omitted.
func BuildPath(classID, instanceID, attributeID UINT) Path {
	p := NewPath()
	p.AddClass(classID)
	p.AddInstance(instanceID)
	if attributeID != 0 {
		p.AddAttribute(attributeID)
	}
	return p
}
