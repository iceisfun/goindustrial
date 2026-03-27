package plc

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestDataTypeString(t *testing.T) {
	tests := []struct {
		dt   DataType
		want string
	}{
		{TypeUnknown, "unknown"},
		{TypeBool, "bool"},
		{TypeUint16, "uint16"},
		{TypeInt32, "int32"},
		{TypeFloat32, "float32"},
		{TypeBytes, "bytes"},
		{DataType(255), "DataType(255)"},
	}
	for _, tt := range tests {
		if got := tt.dt.String(); got != tt.want {
			t.Errorf("DataType(%d).String() = %q, want %q", tt.dt, got, tt.want)
		}
	}
}

func TestByteOrderString(t *testing.T) {
	tests := []struct {
		bo   ByteOrder
		want string
	}{
		{ByteOrderUnspecified, "unspecified"},
		{ByteOrderBigEndian, "big-endian"},
		{ByteOrderLittleEndian, "little-endian"},
	}
	for _, tt := range tests {
		if got := tt.bo.String(); got != tt.want {
			t.Errorf("ByteOrder(%d).String() = %q, want %q", tt.bo, got, tt.want)
		}
	}
}

func TestValueBool(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"nil", nil, false},
		{"empty", []byte{}, false},
		{"zero byte", []byte{0}, false},
		{"nonzero byte", []byte{1}, true},
		{"multi zero", []byte{0, 0, 0}, false},
		{"multi with nonzero", []byte{0, 0, 1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Value{Raw: tt.raw}
			if got := v.Bool(); got != tt.want {
				t.Errorf("Bool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValueIntBigEndian(t *testing.T) {
	// 1-byte signed
	v := Value{Raw: []byte{0xFF}, ByteOrder: ByteOrderBigEndian}
	got, err := v.Int()
	if err != nil {
		t.Fatal(err)
	}
	if got != -1 {
		t.Errorf("Int() = %d, want -1", got)
	}

	// 2-byte big-endian
	raw2 := make([]byte, 2)
	binary.BigEndian.PutUint16(raw2, uint16(0x8000)) // -32768 as int16
	v = Value{Raw: raw2, ByteOrder: ByteOrderBigEndian}
	got, err = v.Int()
	if err != nil {
		t.Fatal(err)
	}
	if got != -32768 {
		t.Errorf("Int() = %d, want -32768", got)
	}

	// 4-byte big-endian
	raw4 := make([]byte, 4)
	binary.BigEndian.PutUint32(raw4, uint32(42))
	v = Value{Raw: raw4, ByteOrder: ByteOrderBigEndian}
	got, err = v.Int()
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("Int() = %d, want 42", got)
	}
}

func TestValueIntLittleEndian(t *testing.T) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, uint32(99))
	v := Value{Raw: raw, ByteOrder: ByteOrderLittleEndian}
	got, err := v.Int()
	if err != nil {
		t.Fatal(err)
	}
	if got != 99 {
		t.Errorf("Int() = %d, want 99", got)
	}
}

func TestValueIntNoByteOrder(t *testing.T) {
	v := Value{Raw: []byte{0x00, 0x01}}
	_, err := v.Int()
	if err == nil {
		t.Error("expected error for unspecified byte order on 2-byte value")
	}
}

func TestValueUint(t *testing.T) {
	raw := make([]byte, 2)
	binary.BigEndian.PutUint16(raw, 0x1234)
	v := Value{Raw: raw, ByteOrder: ByteOrderBigEndian}
	got, err := v.Uint()
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x1234 {
		t.Errorf("Uint() = %d, want %d", got, 0x1234)
	}
}

func TestValueFloat32(t *testing.T) {
	raw := make([]byte, 4)
	binary.LittleEndian.PutUint32(raw, math.Float32bits(3.14))
	v := Value{Raw: raw, ByteOrder: ByteOrderLittleEndian}
	got, err := v.Float32()
	if err != nil {
		t.Fatal(err)
	}
	if got != 3.14 {
		t.Errorf("Float32() = %v, want 3.14", got)
	}
}

func TestValueFloat64(t *testing.T) {
	raw := make([]byte, 8)
	binary.BigEndian.PutUint64(raw, math.Float64bits(2.718))
	v := Value{Raw: raw, ByteOrder: ByteOrderBigEndian}
	got, err := v.Float64()
	if err != nil {
		t.Fatal(err)
	}
	if got != 2.718 {
		t.Errorf("Float64() = %v, want 2.718", got)
	}
}

func TestValueFloat32WrongSize(t *testing.T) {
	v := Value{Raw: []byte{1, 2, 3}, ByteOrder: ByteOrderLittleEndian}
	_, err := v.Float32()
	if err == nil {
		t.Error("expected error for 3-byte float32")
	}
}

func TestValueIntUnsupportedSize(t *testing.T) {
	v := Value{Raw: []byte{1, 2, 3}, ByteOrder: ByteOrderBigEndian}
	_, err := v.Int()
	if err == nil {
		t.Error("expected error for 3-byte int")
	}
}
