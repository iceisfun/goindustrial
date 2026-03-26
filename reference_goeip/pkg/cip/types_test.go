package cip

import (
	"testing"
)

func TestDataTypeString(t *testing.T) {
	tests := []struct {
		code     DataType
		expected string
	}{
		{TypeDINT, "DINT"},
		{TypeBOOL, "BOOL"},
		{TypeDINT | 0x8000, "DINT[]"},
		{0x1234, "UNKNOWN(0x1234)"},
		{0x1234 | 0x8000, "UNKNOWN(0x1234)[]"},
		{TypeSTRUCT, "STRUCT"},
	}

	for _, tt := range tests {
		got := tt.code.String()
		if got != tt.expected {
			t.Errorf("DataType(0x%04X).String() = %q, want %q", uint16(tt.code), got, tt.expected)
		}
	}
}

func TestDataTypeIsArray(t *testing.T) {
	if (TypeDINT).IsArray() {
		t.Error("TypeDINT should not be array")
	}
	if !(TypeDINT | 0x8000).IsArray() {
		t.Error("TypeDINT|0x8000 should be array")
	}
}

func TestDataTypeBase(t *testing.T) {
	if (TypeDINT | 0x8000).Base() != TypeDINT {
		t.Error("Base() should mask out array bit")
	}
	if (TypeDINT).Base() != TypeDINT {
		t.Error("Base() should return same type if not array")
	}
}
