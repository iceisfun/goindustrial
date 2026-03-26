package utils

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

func TestHexDump(t *testing.T) {
	data := []byte{0x00, 0x01, 0xFE, 0xFF}

	got := HexDump(data)
	want := hex.Dump(data)

	if got != want {
		t.Fatalf("HexDump(%v) = %q, want %q", data, got, want)
	}
}

func TestHexDumpEmpty(t *testing.T) {
	if got := HexDump(nil); got != "" {
		t.Fatalf("HexDump(nil) = %q, want empty string", got)
	}
	if got := HexDump([]byte{}); got != "" {
		t.Fatalf("HexDump(empty slice) = %q, want empty string", got)
	}
}

func TestHexDumpLines(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwx") // spans multiple lines

	got := HexDumpLines(data)
	want := trimLines(hex.Dump(data))

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HexDumpLines(%q) = %v, want %v", data, got, want)
	}
}

func TestHexDumpLinesEmpty(t *testing.T) {
	if got := HexDumpLines(nil); len(got) != 0 {
		t.Fatalf("HexDumpLines(nil) length = %d, want 0", len(got))
	}
}

func TestByteToHex(t *testing.T) {
	tests := []struct {
		name string
		b    byte
		want string
	}{
		{name: "zero", b: 0x00, want: "00"},
		{name: "single digit", b: 0x0A, want: "0A"},
		{name: "max", b: 0xFF, want: "FF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ByteToHex(tt.b); got != tt.want {
				t.Fatalf("ByteToHex(%#x) = %q, want %q", tt.b, got, tt.want)
			}
		})
	}
}

func trimLines(dump string) []string {
	lines := strings.Split(dump, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
