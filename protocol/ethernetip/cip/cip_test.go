package cip

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// ===========================================================================
// EPATH Segment Type Identification
// Sourced from OpENer cipepathtest.cpp — verifies segment type byte decoding
// ===========================================================================

func TestSegmentTypeFromByte(t *testing.T) {
	tests := []struct {
		name     string
		raw      byte
		wantType byte
	}{
		{"PortSegment", 0x00, SegmentTypePort},
		{"PortSegment_0x0F", 0x0F, SegmentTypePort},
		{"LogicalSegment", 0x20, SegmentTypeLogical},
		{"LogicalSegment_0x3F", 0x3F, SegmentTypeLogical},
		{"NetworkSegment", 0x40, SegmentTypeNetwork},
		{"NetworkSegment_0x5F", 0x5F, SegmentTypeNetwork},
		{"SymbolicSegment", 0x60, SegmentTypeSymbolic},
		{"SymbolicSegment_0x7F", 0x7F, SegmentTypeSymbolic},
		{"DataSegment", 0x80, SegmentTypeData},
		{"DataSegment_ANSI", 0x91, SegmentTypeData},
		{"DataTypeConstructed", 0xA0, SegmentTypeDataType1},
		{"DataTypeElementary", 0xC0, SegmentTypeDataType2},
		{"Reserved", 0xE0, SegmentTypeReserved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw & 0xE0 // Top 3 bits determine segment type
			if got != tt.wantType {
				t.Errorf("segment type byte 0x%02X: got 0x%02X, want 0x%02X", tt.raw, got, tt.wantType)
			}
		})
	}
}

// ===========================================================================
// Logical Segment Type Identification
// Verifies the 3-bit logical type field within logical segments
// ===========================================================================

func TestLogicalSegmentType(t *testing.T) {
	tests := []struct {
		name    string
		raw     byte
		wantLT  byte
	}{
		{"ClassID", 0x20, LogicalTypeClass},
		{"InstanceID", 0x24, LogicalTypeInstance},
		{"MemberID", 0x28, LogicalTypeMember},
		{"ConnectionPoint", 0x2C, LogicalTypePoint},
		{"AttributeID", 0x30, LogicalTypeAttribute},
		{"Special", 0x34, LogicalTypeSpecial},
		{"ServiceID", 0x38, LogicalTypeService},
		{"ExtendedLogical", 0x3C, LogicalTypeExtended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (tt.raw >> 2) & 0x07 // Bits 4-2 of the segment byte
			want := (tt.wantLT >> 2) & 0x07
			if got != want {
				t.Errorf("logical type from 0x%02X: got 0x%02X, want 0x%02X", tt.raw, got, want)
			}
		})
	}
}

// ===========================================================================
// Logical Segment Format Identification
// Verifies the 2-bit format field (8-bit, 16-bit, 32-bit)
// ===========================================================================

func TestLogicalSegmentFormat(t *testing.T) {
	tests := []struct {
		name   string
		raw    byte
		format byte
	}{
		{"8Bit", 0x20, LogicalFormat8Bit},
		{"16Bit", 0x21, LogicalFormat16Bit},
		{"32Bit", 0x22, LogicalFormat32Bit},
		{"Reserved", 0x23, LogicalFormatReserved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw & 0x03 // Bottom 2 bits
			if got != tt.format {
				t.Errorf("format from 0x%02X: got 0x%02X, want 0x%02X", tt.raw, got, tt.format)
			}
		})
	}
}

// ===========================================================================
// EPATH Building — Comprehensive Path Construction Tests
// Validates byte-level output against known-good patterns from OpENer
// ===========================================================================

func TestPathBuild8BitClass(t *testing.T) {
	p := NewPath()
	p.AddClass(0x04)
	expect := []byte{0x20, 0x04}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("8-bit class: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild16BitClass(t *testing.T) {
	p := NewPath()
	p.AddClass(0x0100)
	expect := []byte{0x21, 0x00, 0x00, 0x01}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("16-bit class: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild8BitInstance(t *testing.T) {
	p := NewPath()
	p.AddInstance(0x01)
	expect := []byte{0x24, 0x01}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("8-bit instance: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild16BitInstance(t *testing.T) {
	p := NewPath()
	p.AddInstance(0x0200)
	expect := []byte{0x25, 0x00, 0x00, 0x02}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("16-bit instance: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild32BitInstance(t *testing.T) {
	p := NewPath()
	p.AddInstance32(0x00010000)
	expect := []byte{0x26, 0x00, 0x00, 0x00, 0x01, 0x00}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("32-bit instance: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild8BitAttribute(t *testing.T) {
	p := NewPath()
	p.AddAttribute(0x03)
	expect := []byte{0x30, 0x03}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("8-bit attribute: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild16BitAttribute(t *testing.T) {
	p := NewPath()
	p.AddAttribute(0x0300)
	expect := []byte{0x31, 0x00, 0x00, 0x03}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("16-bit attribute: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild8BitMember(t *testing.T) {
	p := NewPath()
	p.AddMember(0x05)
	expect := []byte{0x28, 0x05}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("8-bit member: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuild16BitMember(t *testing.T) {
	p := NewPath()
	p.AddMember(0x0400)
	expect := []byte{0x29, 0x00, 0x00, 0x04}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("16-bit member: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuildSymbolicEvenLength(t *testing.T) {
	p := NewPath()
	p.AddSymbolicSegment("Test")
	expect := []byte{0x91, 0x04, 'T', 'e', 's', 't'}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("symbolic even: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuildSymbolicOddPad(t *testing.T) {
	p := NewPath()
	p.AddSymbolicSegment("Tag")
	expect := []byte{0x91, 0x03, 'T', 'a', 'g', 0x00}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("symbolic odd pad: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuildPortSegmentSimple(t *testing.T) {
	p := NewPath()
	p.AddPortSegment(1, []byte{0x00}) // Port 1, link address 0
	expect := []byte{0x01, 0x00}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("port segment simple: got %X, want %X", p.Bytes(), expect)
	}
}

func TestPathBuildPortSegmentExtended(t *testing.T) {
	p := NewPath()
	p.AddPortSegment(15, []byte{0x01}) // Extended port number
	// Port >= 15: uses 0x0F with extended port bytes
	if len(p.Bytes()) < 4 {
		t.Fatalf("extended port segment too short: %d bytes", len(p.Bytes()))
	}
	// First byte should have port nibble = 0x0F
	if p.Bytes()[0]&0x0F != 0x0F {
		t.Fatalf("extended port nibble: got 0x%02X, want 0x0F", p.Bytes()[0]&0x0F)
	}
}

func TestPathBuildComplex(t *testing.T) {
	// Build Class 4, Instance 1, Attribute 3 — standard CIP path
	p := BuildPath(0x04, 0x01, 0x03)
	expect := []byte{
		0x20, 0x04, // Class 4
		0x24, 0x01, // Instance 1
		0x30, 0x03, // Attribute 3
	}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("complex path: got %X, want %X", p.Bytes(), expect)
	}
	if p.LenWords() != 3 {
		t.Fatalf("LenWords = %d, want 3", p.LenWords())
	}
}

func TestPathBuildNoAttribute(t *testing.T) {
	// Attribute 0 means "no attribute segment"
	p := BuildPath(0x04, 0x01, 0)
	expect := []byte{0x20, 0x04, 0x24, 0x01}
	if !bytes.Equal(p.Bytes(), expect) {
		t.Fatalf("no-attr path: got %X, want %X", p.Bytes(), expect)
	}
}

// ===========================================================================
// Data Segment Subtype Identification
// OpENer tests for 0x80 (Simple Data), 0x91 (ANSI Extended Symbol)
// ===========================================================================

func TestDataSegmentSubtype(t *testing.T) {
	tests := []struct {
		name    string
		raw     byte
		subtype byte
	}{
		{"SimpleData", 0x80, 0x00},
		{"ANSIExtendedSymbol", 0x91, 0x11},
		{"Reserved", 0x81, 0x01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw & 0x1F // Bottom 5 bits for data segment subtype
			if got != tt.subtype {
				t.Errorf("data segment subtype from 0x%02X: got 0x%02X, want 0x%02X", tt.raw, got, tt.subtype)
			}
		})
	}
}

// ===========================================================================
// Network Segment Subtype Identification
// From OpENer: Schedule=0x41, FixedTag=0x42, ProdInhibitMs=0x43, Safety=0x44
// ===========================================================================

func TestNetworkSegmentSubtype(t *testing.T) {
	const (
		netSubSchedule         byte = 0x01
		netSubFixedTag         byte = 0x02
		netSubProdInhibitMs    byte = 0x03
		netSubSafety           byte = 0x04
		netSubProdInhibitUs    byte = 0x10
		netSubExtendedNetwork  byte = 0x1F
	)
	tests := []struct {
		name    string
		raw     byte
		subtype byte
	}{
		{"Schedule", 0x41, netSubSchedule},
		{"FixedTag", 0x42, netSubFixedTag},
		{"ProdInhibitMs", 0x43, netSubProdInhibitMs},
		{"Safety", 0x44, netSubSafety},
		{"ProdInhibitUs", 0x50, netSubProdInhibitUs},
		{"ExtendedNetwork", 0x5F, netSubExtendedNetwork},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.raw & 0x1F
			if got != tt.subtype {
				t.Errorf("network segment subtype from 0x%02X: got 0x%02X, want 0x%02X", tt.raw, got, tt.subtype)
			}
		})
	}
}

// ===========================================================================
// CIP Data Type Encode/Decode Round-Trips
// Values from OpENer cipcommontests.cpp
// ===========================================================================

func TestEncodeDecodeBool(t *testing.T) {
	var v bool = false
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0] != 0 {
		t.Fatalf("bool false: got %X, want [00]", data)
	}

	v = true
	data, err = Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0] != 1 {
		t.Fatalf("bool true: got %X, want [01]", data)
	}

	var out bool
	if err := Unmarshal([]byte{1}, &out); err != nil {
		t.Fatal(err)
	}
	if !out {
		t.Fatal("expected true")
	}
}

func TestEncodeDecodeUSINT(t *testing.T) {
	var v uint8 = 212
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0] != 212 {
		t.Fatalf("USINT 212: got %X, want [D4]", data)
	}

	var out uint8
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 212 {
		t.Fatalf("USINT round-trip: got %d, want 212", out)
	}
}

func TestEncodeDecodeUINT(t *testing.T) {
	var v uint16 = 42568 // OpENer test value
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 {
		t.Fatalf("UINT length: got %d, want 2", len(data))
	}
	got := binary.LittleEndian.Uint16(data)
	if got != 42568 {
		t.Fatalf("UINT 42568: got %d", got)
	}

	var out uint16
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 42568 {
		t.Fatalf("UINT round-trip: got %d, want 42568", out)
	}
}

func TestEncodeDecodeUDINT(t *testing.T) {
	var v uint32 = 1653245 // OpENer test value
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 {
		t.Fatalf("UDINT length: got %d, want 4", len(data))
	}
	got := binary.LittleEndian.Uint32(data)
	if got != 1653245 {
		t.Fatalf("UDINT 1653245: got %d", got)
	}

	var out uint32
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 1653245 {
		t.Fatalf("UDINT round-trip: got %d, want 1653245", out)
	}
}

func TestEncodeDecodeDWORD(t *testing.T) {
	var v uint32 = 5357678 // OpENer test value for CipDword
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 {
		t.Fatalf("DWORD length: got %d, want 4", len(data))
	}
	got := binary.LittleEndian.Uint32(data)
	if got != 5357678 {
		t.Fatalf("DWORD 5357678: got %d", got)
	}
}

func TestEncodeDecodeULINT(t *testing.T) {
	var v uint64 = 8353457678 // OpENer test value for CipLword/Ulint
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8 {
		t.Fatalf("ULINT length: got %d, want 8", len(data))
	}
	got := binary.LittleEndian.Uint64(data)
	if got != 8353457678 {
		t.Fatalf("ULINT 8353457678: got %d", got)
	}

	var out uint64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 8353457678 {
		t.Fatalf("ULINT round-trip: got %d, want 8353457678", out)
	}
}

func TestEncodeDecodeSINT(t *testing.T) {
	var v int8 = -42
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 {
		t.Fatalf("SINT length: got %d, want 1", len(data))
	}

	var out int8
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != -42 {
		t.Fatalf("SINT round-trip: got %d, want -42", out)
	}
}

func TestEncodeDecodeINT(t *testing.T) {
	var v int16 = -12345
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 {
		t.Fatalf("INT length: got %d, want 2", len(data))
	}

	var out int16
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != -12345 {
		t.Fatalf("INT round-trip: got %d, want -12345", out)
	}
}

func TestEncodeDecodeDINT(t *testing.T) {
	var v int32 = -100000
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 {
		t.Fatalf("DINT length: got %d, want 4", len(data))
	}

	var out int32
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != -100000 {
		t.Fatalf("DINT round-trip: got %d, want -100000", out)
	}
}

func TestEncodeDecodeLINT(t *testing.T) {
	var v int64 = -9876543210
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8 {
		t.Fatalf("LINT length: got %d, want 8", len(data))
	}

	var out int64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != -9876543210 {
		t.Fatalf("LINT round-trip: got %d, want -9876543210", out)
	}
}

func TestEncodeDecodeREAL(t *testing.T) {
	var v float32 = 3.14
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4 {
		t.Fatalf("REAL length: got %d, want 4", len(data))
	}

	var out float32
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(out-3.14)) > 0.001 {
		t.Fatalf("REAL round-trip: got %f, want ~3.14", out)
	}
}

func TestEncodeDecodeLREAL(t *testing.T) {
	var v float64 = 2.718281828
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8 {
		t.Fatalf("LREAL length: got %d, want 8", len(data))
	}

	var out float64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if math.Abs(out-2.718281828) > 0.000001 {
		t.Fatalf("LREAL round-trip: got %f, want ~2.718281828", out)
	}
}

func TestEncodeDecodeWORD(t *testing.T) {
	var v uint16 = 53678 // OpENer test value for CipWord
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 2 {
		t.Fatalf("WORD length: got %d, want 2", len(data))
	}
	got := binary.LittleEndian.Uint16(data)
	if got != 53678 {
		t.Fatalf("WORD 53678: got %d", got)
	}
}

func TestEncodeDecodeBYTE(t *testing.T) {
	var v uint8 = 173 // OpENer test value for CipByte
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 || data[0] != 173 {
		t.Fatalf("BYTE 173: got %X, want [AD]", data)
	}
}

// ===========================================================================
// GoTypeToCIPType Mapping
// ===========================================================================

func TestGoTypeToCIPType(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantType DataType
	}{
		{"bool", true, TypeBOOL},
		{"int8", int8(0), TypeSINT},
		{"int16", int16(0), TypeINT},
		{"int32", int32(0), TypeDINT},
		{"int64", int64(0), TypeLINT},
		{"uint8", uint8(0), TypeUSINT},
		{"uint16", uint16(0), TypeUINT},
		{"uint32", uint32(0), TypeUDINT},
		{"uint64", uint64(0), TypeULINT},
		{"float32", float32(0), TypeREAL},
		{"float64", float64(0), TypeLREAL},
		{"string", "", TypeSTRING},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GoTypeToCIPType(tt.value)
			if err != nil {
				t.Fatalf("GoTypeToCIPType(%T): %v", tt.value, err)
			}
			if got != tt.wantType {
				t.Errorf("GoTypeToCIPType(%T) = 0x%04X, want 0x%04X", tt.value, got, tt.wantType)
			}
		})
	}
}

func TestGoTypeToCIPTypeUnsupported(t *testing.T) {
	_, err := GoTypeToCIPType(struct{}{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// ===========================================================================
// DataType String and Flag Helpers
// ===========================================================================

func TestDataTypeString(t *testing.T) {
	tests := []struct {
		dt   DataType
		want string
	}{
		{TypeBOOL, "BOOL"},
		{TypeDINT, "DINT"},
		{TypeREAL, "REAL"},
		{TypeSTRING, "STRING"},
		{TypeSTRUCT, "STRUCT"},
		{DataType(0x80C4), "DINT[]"}, // Array bit set
		{DataType(0x00FF), "UNKNOWN(0x00FF)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dt.String(); got != tt.want {
				t.Errorf("DataType(0x%04X).String() = %q, want %q", uint16(tt.dt), got, tt.want)
			}
		})
	}
}

func TestDataTypeIsArray(t *testing.T) {
	if TypeDINT.IsArray() {
		t.Error("TypeDINT should not be array")
	}
	arr := DataType(0x80C4)
	if !arr.IsArray() {
		t.Error("0x80C4 should be array")
	}
	if arr.Base() != TypeDINT {
		t.Errorf("Base() = 0x%04X, want TypeDINT (0x%04X)", arr.Base(), TypeDINT)
	}
}

// ===========================================================================
// CIP Status Codes Completeness
// Validates that all status codes from the CIP spec (sourced from OpENer
// ciperror.h) are defined
// ===========================================================================

func TestStatusCodesExist(t *testing.T) {
	codes := map[string]USINT{
		"Success":              StatusSuccess,
		"ConnectionFailure":    StatusConnectionFailure,
		"ResourceUnavailable":  StatusResourceUnavailable,
		"PathSegmentError":     StatusPathSegmentError,
		"PathDestUnknown":      StatusPathDestinationUnknown,
		"PartialTransfer":      StatusPartialTransfer,
		"ServiceNotSupported":  StatusServiceNotSupported,
		"InvalidAttributeVal":  StatusInvalidAttributeValue,
		"AttributeNotSettable": StatusAttributeNotSettable,
		"PrivilegeViolation":   StatusPrivilegeViolation,
		"DeviceStateConflict":  StatusDeviceStateConflict,
		"ReplyDataTooLarge":    StatusReplyDataTooLarge,
		"NotEnoughData":        StatusNotEnoughData,
		"AttrNotSupported":     StatusAttributeNotSupported,
		"TooMuchData":          StatusTooMuchData,
		"ObjectDoesNotExist":   StatusObjectDoesNotExist,
	}
	for name, code := range codes {
		if code == 0 && name != "Success" {
			t.Errorf("status code %s is zero (likely undefined)", name)
		}
	}
}

// ===========================================================================
// MessageRouterRequest Encode
// ===========================================================================

func TestMessageRouterRequestEncode(t *testing.T) {
	req := &MessageRouterRequest{
		Service:     ServiceReadTag,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
		RequestData: []byte{0x01, 0x00},
	}
	data, err := req.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// Service(1) + PathLenWords(1) + Path(4) + Data(2) = 8
	if len(data) != 8 {
		t.Fatalf("encoded length = %d, want 8", len(data))
	}
	if data[0] != byte(ServiceReadTag) {
		t.Errorf("service = 0x%02X, want 0x%02X", data[0], ServiceReadTag)
	}
	if data[1] != 2 { // 4 bytes / 2 = 2 words
		t.Errorf("path len words = %d, want 2", data[1])
	}
	if !bytes.Equal(data[2:6], []byte{0x20, 0x04, 0x24, 0x01}) {
		t.Errorf("path bytes = %X, want 20042401", data[2:6])
	}
}

func TestMessageRouterRequestEncodeEmpty(t *testing.T) {
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x01, 0x24, 0x01}),
	}
	data, err := req.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 6 {
		t.Fatalf("encoded length = %d, want 6", len(data))
	}
}

// ===========================================================================
// MessageRouterResponse Decode
// ===========================================================================

func TestMessageRouterResponseDecodeSuccess(t *testing.T) {
	// Build a known-good response: Service|0x80, Reserved, Status=0, ExtSize=0, Data
	respBytes := []byte{
		byte(ServiceReadTag) | 0x80, // Reply service
		0x00,                        // Reserved
		byte(StatusSuccess),         // General status
		0x00,                        // Ext status size
		0xC4, 0x00, 0x2A, 0x00, 0x00, 0x00, // Response data (TypeDINT + 42)
	}
	resp, err := DecodeMessageRouterResponse(respBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsSuccess() {
		t.Errorf("expected success, got status 0x%02X", resp.GeneralStatus)
	}
	if resp.Error() != nil {
		t.Errorf("expected nil error, got %v", resp.Error())
	}
	if len(resp.ResponseData) != 6 {
		t.Fatalf("response data len = %d, want 6", len(resp.ResponseData))
	}
	if resp.Service != USINT(ServiceReadTag)|0x80 {
		t.Errorf("reply service = 0x%02X, want 0x%02X", resp.Service, USINT(ServiceReadTag)|0x80)
	}
}

func TestMessageRouterResponseDecodeError(t *testing.T) {
	respBytes := []byte{
		byte(ServiceReadTag) | 0x80,
		0x00,
		byte(StatusObjectDoesNotExist),
		0x00,
	}
	resp, err := DecodeMessageRouterResponse(respBytes)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsSuccess() {
		t.Error("expected failure")
	}
	if resp.Error() == nil {
		t.Error("expected non-nil error")
	}
}

func TestMessageRouterResponseDecodeExtStatus(t *testing.T) {
	// Response with 1 extended status word
	respBytes := []byte{
		byte(ServiceReadTag) | 0x80,
		0x00,
		byte(StatusConnectionFailure),
		0x01,       // 1 word of ext status
		0x00, 0x01, // Extended status 0x0100
	}
	resp, err := DecodeMessageRouterResponse(respBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ExtStatus) != 1 {
		t.Fatalf("ext status len = %d, want 1", len(resp.ExtStatus))
	}
	if resp.ExtStatus[0] != 0x0100 {
		t.Errorf("ext status[0] = 0x%04X, want 0x0100", resp.ExtStatus[0])
	}
}

// ===========================================================================
// MessageRouter Dispatch
// ===========================================================================

func TestMessageRouterDispatch8BitClass(t *testing.T) {
	router := NewMessageRouter()

	called := false
	obj := &testObject{fn: func(service USINT, path Path, data []byte) ([]byte, error) {
		called = true
		return []byte{0x42}, nil
	}}
	router.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}
	resp, err := router.Dispatch(req)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("object not called")
	}
	if !resp.IsSuccess() {
		t.Fatalf("dispatch failed: status 0x%02X", resp.GeneralStatus)
	}
	if !bytes.Equal(resp.ResponseData, []byte{0x42}) {
		t.Fatalf("response data = %X, want [42]", resp.ResponseData)
	}
}

func TestMessageRouterDispatch16BitClass(t *testing.T) {
	router := NewMessageRouter()

	called := false
	obj := &testObject{fn: func(service USINT, path Path, data []byte) ([]byte, error) {
		called = true
		return nil, nil
	}}
	router.RegisterObject(0x0100, obj)

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x21, 0x00, 0x00, 0x01, 0x24, 0x01}),
	}
	resp, err := router.Dispatch(req)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("16-bit class object not called")
	}
	if !resp.IsSuccess() {
		t.Fatalf("dispatch failed: status 0x%02X", resp.GeneralStatus)
	}
}

func TestMessageRouterDispatchUnknownClass(t *testing.T) {
	router := NewMessageRouter()

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0xFF, 0x24, 0x01}),
	}
	resp, err := router.Dispatch(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IsSuccess() {
		t.Fatal("expected failure for unknown class")
	}
	if resp.GeneralStatus != StatusObjectDoesNotExist {
		t.Errorf("status = 0x%02X, want StatusObjectDoesNotExist (0x%02X)", resp.GeneralStatus, StatusObjectDoesNotExist)
	}
}

func TestMessageRouterDispatchEmptyPath(t *testing.T) {
	router := NewMessageRouter()

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path(nil),
	}
	_, err := router.Dispatch(req)
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestMessageRouterDispatchCIPError(t *testing.T) {
	router := NewMessageRouter()
	obj := &testObject{fn: func(service USINT, path Path, data []byte) ([]byte, error) {
		return nil, Error{Status: StatusServiceNotSupported}
	}}
	router.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     0xFF, // Unsupported
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}
	resp, err := router.Dispatch(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GeneralStatus != StatusServiceNotSupported {
		t.Errorf("status = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusServiceNotSupported)
	}
}

// ===========================================================================
// CIP Error Formatting
// ===========================================================================

func TestCIPErrorString(t *testing.T) {
	e := Error{Status: StatusObjectDoesNotExist}
	s := e.Error()
	if s == "" {
		t.Fatal("error string should not be empty")
	}

	e2 := Error{Status: StatusConnectionFailure, ExtStatus: []UINT{0x0100}}
	s2 := e2.Error()
	if s2 == "" {
		t.Fatal("error string with ext status should not be empty")
	}
}

// ===========================================================================
// Timer/Counter Additional Tests
// ===========================================================================

func TestTimerAllBitsSet(t *testing.T) {
	data := make([]byte, 14)
	binary.LittleEndian.PutUint16(data[0:], 0)
	statusBits := uint32(1<<TimerStatusEN | 1<<TimerStatusTT | 1<<TimerStatusDN)
	binary.LittleEndian.PutUint32(data[2:], statusBits)
	binary.LittleEndian.PutUint32(data[6:], 10000)
	binary.LittleEndian.PutUint32(data[10:], 9999)

	timer, err := DecodeTimer(data)
	if err != nil {
		t.Fatal(err)
	}
	if !timer.EN || !timer.TT || !timer.DN {
		t.Errorf("expected all status bits true: EN=%v TT=%v DN=%v", timer.EN, timer.TT, timer.DN)
	}
	if timer.PRE != 10000 || timer.ACC != 9999 {
		t.Errorf("PRE=%d ACC=%d, want 10000/9999", timer.PRE, timer.ACC)
	}
}

func TestTimerNoBitsSet(t *testing.T) {
	data := make([]byte, 14)
	binary.LittleEndian.PutUint32(data[6:], 5000)

	timer, err := DecodeTimer(data)
	if err != nil {
		t.Fatal(err)
	}
	if timer.EN || timer.TT || timer.DN {
		t.Error("expected all status bits false")
	}
}

func TestTimerDecodeShortData(t *testing.T) {
	_, err := DecodeTimer(make([]byte, 10)) // Too short
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestCounterAllBitsSet(t *testing.T) {
	data := make([]byte, 14)
	binary.LittleEndian.PutUint16(data[0:], 0)
	statusBits := uint32(
		1<<CounterStatusCU | 1<<CounterStatusCD |
			1<<CounterStatusDN | 1<<CounterStatusOV | 1<<CounterStatusUN)
	binary.LittleEndian.PutUint32(data[2:], statusBits)
	binary.LittleEndian.PutUint32(data[6:], 500)
	binary.LittleEndian.PutUint32(data[10:], 501)

	counter, err := DecodeCounter(data)
	if err != nil {
		t.Fatal(err)
	}
	if !counter.CU || !counter.CD || !counter.DN || !counter.OV || !counter.UN {
		t.Error("expected all counter bits true")
	}
	if counter.PRE != 500 || counter.ACC != 501 {
		t.Errorf("PRE=%d ACC=%d, want 500/501", counter.PRE, counter.ACC)
	}
}

func TestCounterDecodeShortData(t *testing.T) {
	_, err := DecodeCounter(make([]byte, 5))
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestCounterMarshalRoundTrip(t *testing.T) {
	original := &Counter{PRE: 200, ACC: 150, CU: true, CD: false, DN: true, OV: false, UN: true}
	data, err := original.MarshalCIP()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := DecodeCounter(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PRE != original.PRE || decoded.ACC != original.ACC {
		t.Fatalf("PRE/ACC mismatch: %v vs %v", decoded, original)
	}
	if decoded.CU != original.CU || decoded.CD != original.CD ||
		decoded.DN != original.DN || decoded.OV != original.OV || decoded.UN != original.UN {
		t.Fatalf("status mismatch: decoded=%+v original=%+v", decoded, original)
	}
}

// ===========================================================================
// Unmarshal Error Cases
// ===========================================================================

func TestUnmarshalNonPointer(t *testing.T) {
	err := Unmarshal([]byte{0x01}, 42)
	if err == nil {
		t.Fatal("expected error for non-pointer")
	}
}

func TestUnmarshalNilPointer(t *testing.T) {
	var p *int32
	err := Unmarshal([]byte{0x01, 0x00, 0x00, 0x00}, p)
	if err == nil {
		t.Fatal("expected error for nil pointer")
	}
}

// ===========================================================================
// Tests mined from cpppo (https://github.com/pjkundert/cpppo)
// Python EtherNet/IP CIP library — authoritative EPATH and CIP data tests
// ===========================================================================

// --- EPATH wire-format vectors ---

func TestPathMixedSegments(t *testing.T) {
	// extpath_3: A complex path with elements, classes, instances, attributes.
	// We verify our Path builder produces the same bytes cpppo expects.
	//
	// Wire format:
	//   0x20, 0x11  ->  8-bit class  == 0x11
	//   0x21, 0x00, 0x11, 0x02  -> 16-bit class == 0x0211
	//   0x24, 0x21  ->  8-bit instance == 0x21
	//   0x25, 0x00, 0x21, 0x02  -> 16-bit instance == 0x0221
	//   0x30, 0x31  ->  8-bit attribute == 0x31
	//   0x31, 0x00, 0x31, 0x02  -> 16-bit attribute == 0x0231

	// 8-bit class 0x11
	p := NewPath()
	p.AddClass(0x11)
	if !bytes.Equal(p.Bytes(), []byte{0x20, 0x11}) {
		t.Fatalf("8-bit class 0x11: got %X, want 2011", p.Bytes())
	}

	// 16-bit class 0x0211
	p = NewPath()
	p.AddClass(0x0211)
	if !bytes.Equal(p.Bytes(), []byte{0x21, 0x00, 0x11, 0x02}) {
		t.Fatalf("16-bit class 0x0211: got %X, want 21001102", p.Bytes())
	}

	// 8-bit instance 0x21
	p = NewPath()
	p.AddInstance(0x21)
	if !bytes.Equal(p.Bytes(), []byte{0x24, 0x21}) {
		t.Fatalf("8-bit instance 0x21: got %X, want 2421", p.Bytes())
	}

	// 16-bit instance 0x0221
	p = NewPath()
	p.AddInstance(0x0221)
	if !bytes.Equal(p.Bytes(), []byte{0x25, 0x00, 0x21, 0x02}) {
		t.Fatalf("16-bit instance 0x0221: got %X, want 25002102", p.Bytes())
	}

	// 8-bit attribute 0x31
	p = NewPath()
	p.AddAttribute(0x31)
	if !bytes.Equal(p.Bytes(), []byte{0x30, 0x31}) {
		t.Fatalf("8-bit attribute 0x31: got %X, want 3031", p.Bytes())
	}

	// 16-bit attribute 0x0231
	p = NewPath()
	p.AddAttribute(0x0231)
	if !bytes.Equal(p.Bytes(), []byte{0x31, 0x00, 0x31, 0x02}) {
		t.Fatalf("16-bit attribute 0x0231: got %X, want 31003102", p.Bytes())
	}
}

func TestPathSymbolicSegments(t *testing.T) {
	// extpath_4:
	//   0x91, 0x06, 'a','b','c','1','2','3'  -> symbolic "abc123" (even, no pad)
	//   0x91, 0x05, 'x','y','z','1','2', 0x00 -> symbolic "xyz12" (odd, +pad)

	p := NewPath()
	p.AddSymbolicSegment("abc123")
	expected := []byte{0x91, 0x06, 'a', 'b', 'c', '1', '2', '3'}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("symbolic abc123: got %X, want %X", p.Bytes(), expected)
	}

	p = NewPath()
	p.AddSymbolicSegment("xyz12")
	expected = []byte{0x91, 0x05, 'x', 'y', 'z', '1', '2', 0x00}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("symbolic xyz12: got %X, want %X", p.Bytes(), expected)
	}
}

func TestPathAssemblyClassInstance(t *testing.T) {
	// extpath_9 (from LEC-GEN1_v1 EDS):
	//   0x20, 0x04  -> Class 0x04 (Assembly Object)
	//   0x24, 0x05  -> Instance 5
	// Connection points (0x2C) are not implemented in goindustrial,
	// but we verify the class/instance portion matches.

	p := NewPath()
	p.AddClass(0x04)
	p.AddInstance(0x05)
	expected := []byte{0x20, 0x04, 0x24, 0x05}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("assembly class 0x04 instance 5: got %X, want %X", p.Bytes(), expected)
	}
}

func TestPathPortSegmentBasic(t *testing.T) {
	// extpath_5: port #1, link 0x00
	//   0x01, 0x00 -> port 1, link address 0x00
	p := NewPath()
	p.AddPortSegment(1, []byte{0x00})
	expected := []byte{0x01, 0x00}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("port 1 link 0: got %X, want %X", p.Bytes(), expected)
	}
}

func TestPathLenWords(t *testing.T) {
	// cpppo extpath_0: 0 words (empty path)
	p := NewPath()
	if p.LenWords() != 0 {
		t.Fatalf("empty path words: got %d, want 0", p.LenWords())
	}

	// cpppo extpath_1: 1 word (2 bytes: 0x28, 0x01)
	// Our Member segment is 0x28 for member ID 1
	p = NewPath()
	p.AddMember(0x01)
	if p.LenWords() != 1 {
		t.Fatalf("1-word path: got %d, want 1", p.LenWords())
	}

	// cpppo extpath_4: 8 words (16 bytes for two symbolic segments)
	p = NewPath()
	p.AddSymbolicSegment("abc123")
	p.AddSymbolicSegment("xyz12")
	if p.LenWords() != 8 {
		t.Fatalf("symbolic path words: got %d, want 8", p.LenWords())
	}
}

func TestPathBuildGetAttributeAllPath(t *testing.T) {
	// cpppo gaa_008_request CIP path: 0x20, 0x66, 0x24, 0x01
	// Class 0x66, Instance 0x01
	p := BuildPath(0x66, 0x01, 0)
	expected := []byte{0x20, 0x66, 0x24, 0x01}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("GAA path class=0x66 inst=1: got %X, want %X", p.Bytes(), expected)
	}
}

func TestPathBuildIdentityObjectPath(t *testing.T) {
	// cpppo gaa_011_request CIP path: 0x20, 0x01, 0x24, 0x01
	// Class 0x01 (Identity Object), Instance 0x01
	p := BuildPath(0x01, 0x01, 0)
	expected := []byte{0x20, 0x01, 0x24, 0x01}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("Identity path: got %X, want %X", p.Bytes(), expected)
	}
}

func TestPathBuildConnectionManagerPath(t *testing.T) {
	// cpppo Unconnected Send path: 0x20, 0x06, 0x24, 0x01
	// Class 0x06 (Connection Manager), Instance 0x01
	p := BuildPath(0x06, 0x01, 0)
	expected := []byte{0x20, 0x06, 0x24, 0x01}
	if !bytes.Equal(p.Bytes(), expected) {
		t.Fatalf("Connection Manager path: got %X, want %X", p.Bytes(), expected)
	}
}

// --- CIP data type boundary values ---

func TestEncodeDecodeLINTBoundary(t *testing.T) {
	// struct.pack('<q', -1234567890123456789)
	var v int64 = -1234567890123456789
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 8 {
		t.Fatalf("LINT length: got %d, want 8", len(data))
	}

	var out int64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != -1234567890123456789 {
		t.Fatalf("LINT boundary: got %d, want -1234567890123456789", out)
	}
}

func TestEncodeDecodeULINTMaxBit63(t *testing.T) {
	// b'\x00\x00\x00\x00\x00\x00\x00\x80' == 2^63
	var v uint64 = 1 << 63 // 2^63 = 9223372036854775808
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	// Verify wire format
	expected := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80}
	if !bytes.Equal(data, expected) {
		t.Fatalf("ULINT 2^63 wire bytes: got %X, want %X", data, expected)
	}

	var out uint64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 1<<63 {
		t.Fatalf("ULINT 2^63 round-trip: got %d, want %d", out, uint64(1<<63))
	}
}

func TestEncodeDecodeINTLittleEndian(t *testing.T) {
	// b'\x01\x00' -> INT value 1 (little-endian)
	data := []byte{0x01, 0x00}
	var out int16
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 1 {
		t.Fatalf("INT from bytes 0x0100: got %d, want 1", out)
	}
}

func TestEncodeDecodeREALZero(t *testing.T) {
	// 4 x REAL all zeros -> [0.0, 0.0, 0.0, 0.0]
	data := []byte{0x00, 0x00, 0x00, 0x00}
	var out float32
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 0.0 {
		t.Fatalf("REAL zero: got %f, want 0.0", out)
	}
}

func TestEncodeDecodeLREALRoundTrip(t *testing.T) {
	// struct.pack('<d', 1.23) -> LREAL 1.23
	var v float64 = 1.23
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}

	var out float64
	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out != 1.23 {
		t.Fatalf("LREAL 1.23: got %f, want 1.23", out)
	}
}

// --- BOOL truthiness ---

func TestBoolTruthiness(t *testing.T) {
	// b'\x00\x01\x02\x04\x08\x10\x20\x40\x80\xff\x00'
	// Expected truths: [False, True, True, True, True, True, True, True, True, True, False]
	// Any non-zero byte is true.
	testCases := []struct {
		raw  byte
		want bool
	}{
		{0x00, false},
		{0x01, true},
		{0x02, true},
		{0x04, true},
		{0x08, true},
		{0x10, true},
		{0x20, true},
		{0x40, true},
		{0x80, true},
		{0xFF, true},
	}
	for _, tc := range testCases {
		var out bool
		if err := Unmarshal([]byte{tc.raw}, &out); err != nil {
			t.Fatalf("BOOL 0x%02X: %v", tc.raw, err)
		}
		if out != tc.want {
			t.Fatalf("BOOL 0x%02X: got %v, want %v", tc.raw, out, tc.want)
		}
	}
}

// --- CIP service code and class ID constant verification ---

func TestCIPServiceCodes(t *testing.T) {
	// Verify service codes match expected values:
	// 0x01 = Get Attribute All
	// 0x52 = Unconnected Send (Read/Write Tag)
	// 0x54 = Forward Open
	// 0x4c = Read Tag
	// 0x4d = Write Tag
	// 0x52 = Read Tag Fragmented
	// 0x53 = Write Tag Fragmented

	if ServiceGetAttributeAll != 0x01 {
		t.Fatalf("ServiceGetAttributeAll: got 0x%02X, want 0x01", ServiceGetAttributeAll)
	}
	if ServiceReadTag != 0x4C {
		t.Fatalf("ServiceReadTag: got 0x%02X, want 0x4C", ServiceReadTag)
	}
	if ServiceWriteTag != 0x4D {
		t.Fatalf("ServiceWriteTag: got 0x%02X, want 0x4D", ServiceWriteTag)
	}
	if ServiceReadTagFragmented != 0x52 {
		t.Fatalf("ServiceReadTagFragmented: got 0x%02X, want 0x52", ServiceReadTagFragmented)
	}
	if ServiceWriteTagFragmented != 0x53 {
		t.Fatalf("ServiceWriteTagFragmented: got 0x%02X, want 0x53", ServiceWriteTagFragmented)
	}
	// Note: ServiceForwardOpen (0x54) is defined in the connmgr package, not cip
}

func TestCIPClassIDs(t *testing.T) {
	// Verify class IDs match expected values
	if ClassIdentity != 0x01 {
		t.Fatalf("ClassIdentity: got 0x%04X, want 0x01", ClassIdentity)
	}
	if ClassConnectionMgr != 0x06 {
		t.Fatalf("ClassConnectionMgr: got 0x%04X, want 0x06", ClassConnectionMgr)
	}
}

// --- CIP data type codes ---

func TestCIPDataTypeCodes(t *testing.T) {
	// Verify tag_type values used in typed_data parsing
	if TypeBOOL != 0x00C1 {
		t.Fatalf("TypeBOOL: got 0x%04X, want 0x00C1", TypeBOOL)
	}
	if TypeSINT != 0x00C2 {
		t.Fatalf("TypeSINT: got 0x%04X, want 0x00C2", TypeSINT)
	}
	if TypeINT != 0x00C3 {
		t.Fatalf("TypeINT: got 0x%04X, want 0x00C3", TypeINT)
	}
	if TypeDINT != 0x00C4 {
		t.Fatalf("TypeDINT: got 0x%04X, want 0x00C4", TypeDINT)
	}
	if TypeLINT != 0x00C5 {
		t.Fatalf("TypeLINT: got 0x%04X, want 0x00C5", TypeLINT)
	}
	if TypeUSINT != 0x00C6 {
		t.Fatalf("TypeUSINT: got 0x%04X, want 0x00C6", TypeUSINT)
	}
	if TypeUINT != 0x00C7 {
		t.Fatalf("TypeUINT: got 0x%04X, want 0x00C7", TypeUINT)
	}
	if TypeUDINT != 0x00C8 {
		t.Fatalf("TypeUDINT: got 0x%04X, want 0x00C8", TypeUDINT)
	}
	if TypeULINT != 0x00C9 {
		t.Fatalf("TypeULINT: got 0x%04X, want 0x00C9", TypeULINT)
	}
	if TypeREAL != 0x00CA {
		t.Fatalf("TypeREAL: got 0x%04X, want 0x00CA", TypeREAL)
	}
	if TypeLREAL != 0x00CB {
		t.Fatalf("TypeLREAL: got 0x%04X, want 0x00CB", TypeLREAL)
	}
	if TypeSTRING != 0x00D0 {
		t.Fatalf("TypeSTRING: got 0x%04X, want 0x00D0", TypeSTRING)
	}
}

// --- MessageRouter response decoding from wire captures ---

func TestMessageRouterResponseDecodeTagReply(t *testing.T) {
	// cpppo unk_014_reply CPF data inner CIP response:
	// 0xd2, 0x00, 0x00, 0x00 -> service 0xD2 (reply bit set for 0x52),
	//   reserved 0x00, status 0x00 (success), ext_status_size 0x00
	//
	// This is the CIP response portion from a real Logix PLC.
	data := []byte{
		0xD2,       // Service = 0x52 | 0x80 (reply)
		0x00,       // Reserved
		0x00,       // Status = success
		0x00,       // Extended status size = 0
		0xC3, 0x00, // Response data: type code 0x00C3 (INT)
		0x27, 0x80, // INT value = 0x8027 = -32729 (signed) or 32807 (unsigned)
	}

	resp, err := DecodeMessageRouterResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Service != 0xD2 {
		t.Fatalf("service: got 0x%02X, want 0xD2", resp.Service)
	}
	if resp.GeneralStatus != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", resp.GeneralStatus)
	}
	if len(resp.ExtStatus) != 0 {
		t.Fatalf("ext status: got %d entries, want 0", len(resp.ExtStatus))
	}
	// Response data should be the type code + value
	if len(resp.ResponseData) != 4 {
		t.Fatalf("data length: got %d, want 4", len(resp.ResponseData))
	}
}

func TestMessageRouterResponseDecodePathError(t *testing.T) {
	// cpppo rfg_002_reply — CIP error 0x04 with extended status:
	// 0xd2, 0x00, 0x04, 0x01, 0x00, 0x00
	// Service 0xD2, reserved 0x00, status 0x04 (path error),
	// ext_status_size=1, ext_status=0x0000
	data := []byte{
		0xD2,       // Service
		0x00,       // Reserved
		0x04,       // Status = 0x04 (path segment error)
		0x01,       // Extended status size = 1 word
		0x00, 0x00, // Extended status word = 0x0000
	}

	resp, err := DecodeMessageRouterResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GeneralStatus != 0x04 {
		t.Fatalf("status: got 0x%02X, want 0x04", resp.GeneralStatus)
	}
	if len(resp.ExtStatus) != 1 {
		t.Fatalf("ext status: got %d entries, want 1", len(resp.ExtStatus))
	}
	if resp.ExtStatus[0] != 0x0000 {
		t.Fatalf("ext status[0]: got 0x%04X, want 0x0000", resp.ExtStatus[0])
	}
}

func TestMessageRouterResponseDecodeGeneralError(t *testing.T) {
	// cpppo rfg_001_reply — CIP error 0x05:
	// 0xd2, 0x00, 0x05, 0x00
	// Service 0xD2, reserved 0x00, status 0x05, ext_status_size=0
	data := []byte{
		0xD2,
		0x00,
		0x05, // Status = 0x05 (access denied / privilege violation)
		0x00,
	}

	resp, err := DecodeMessageRouterResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GeneralStatus != 0x05 {
		t.Fatalf("status: got 0x%02X, want 0x05", resp.GeneralStatus)
	}
	if len(resp.ExtStatus) != 0 {
		t.Fatalf("ext status: got %d entries, want 0", len(resp.ExtStatus))
	}
}

// --- CIP string encoding verification ---

func TestGoTypeToCIPTypeString(t *testing.T) {
	// STRING type code is 0x00D0.
	// Verify Go string maps to TypeSTRING.
	dt, err := GoTypeToCIPType("Hello")
	if err != nil {
		t.Fatal(err)
	}
	if dt != TypeSTRING {
		t.Fatalf("string type: got 0x%04X, want 0x%04X (TypeSTRING)", dt, TypeSTRING)
	}
}

// ===========================================================================
// test helpers
// ===========================================================================

type testObject struct {
	fn func(service USINT, path Path, data []byte) ([]byte, error)
}

func (o *testObject) HandleRequest(service USINT, path Path, data []byte) ([]byte, error) {
	return o.fn(service, path, data)
}
