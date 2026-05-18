package cip

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestParseTagPath(t *testing.T) {
	// Each `want` value is the expected EPATH byte sequence as a hex string.
	// Whitespace inside `want` is stripped before comparison so the byte
	// groupings stay legible.
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{
			name: "simple symbol",
			tag:  "MyTag",
			// symbol("MyTag", 5 chars, pad)
			want: "9105 4D79546167 00",
		},
		{
			name: "struct member",
			tag:  "MyStruct.Field",
			// symbol("MyStruct", 8 chars, no pad) + symbol("Field", 5 chars, pad)
			want: "9108 4D79537472756374 9105 4669656C64 00",
		},
		{
			name: "program scope + member + array + member",
			tag:  "Program:MainProgram.MyArray[5].Field",
			want: "9113 50726F6772616D3A4D61696E50726F6772616D 00" +
				"9107 4D7941727261 79 00" +
				"28 05" +
				"9105 4669656C64 00",
		},
		{
			name: "2D array index",
			tag:  "Matrix[2,3]",
			// symbol("Matrix", 6 chars, no pad) + member(2) + member(3)
			want: "9106 4D6174726978 2802 2803",
		},
		{
			name: "bit access on integer",
			tag:  "MyDINT.5",
			// symbol("MyDINT", 6 chars, no pad) + member(5)
			want: "9106 4D7944494E54 2805",
		},
		{
			name: "16-bit array index",
			tag:  "BigArray[300]",
			// symbol("BigArray", 8 chars, no pad) + member16(300=0x012C)
			want: "9108 4269674172726179 29 00 2C01",
		},
		{
			name: "module I/O tag with colons and array",
			tag:  "Local:2:I.Data[0]",
			// symbol("Local:2:I", 9 chars, pad) + symbol("Data", 4 chars, no pad) + member(0)
			want: "9109 4C6F63616C3A323A49 00 9104 44617461 2800",
		},
		{
			name: "user case",
			tag:  "Program:MainProgram.Tote_Count_CNTR.ACC",
			want: "9113 50726F6772616D3A4D61696E50726F6772616D 00" +
				"910F 546F74655F436F756E745F434E5452 00" +
				"9103 414343 00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseTagPath(tc.tag)
			if err != nil {
				t.Fatalf("ParseTagPath(%q): %v", tc.tag, err)
			}
			got := strings.ToUpper(hex.EncodeToString(p.Bytes()))
			wantHex := strings.ToUpper(strings.ReplaceAll(tc.want, " ", ""))
			if got != wantHex {
				t.Errorf("ParseTagPath(%q)\n got:  %s\n want: %s", tc.tag, got, wantHex)
			}
		})
	}
}

func TestParseTagPathErrors(t *testing.T) {
	cases := []string{
		"",         // empty
		"Foo[",     // unmatched [
		"Foo]",     // unmatched ]
		"Foo[]",    // empty brackets
		"Foo[abc]", // non-numeric index
		"Foo[1,]",  // empty index value
		"Foo..Bar", // empty middle segment
		"Foo.",     // empty trailing segment
		".Foo",     // empty leading segment
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := ParseTagPath(c); err == nil {
				t.Errorf("ParseTagPath(%q): expected error, got nil", c)
			}
		})
	}
}
