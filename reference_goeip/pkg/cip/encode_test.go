package cip

import (
	"bytes"
	"testing"
)

func TestMarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []byte
		wantErr bool
	}{
		{
			name:  "int32",
			input: int32(1),
			want:  []byte{0x01, 0x00, 0x00, 0x00},
		},
		{
			name:  "float32",
			input: float32(1.0),
			want:  []byte{0x00, 0x00, 0x80, 0x3F},
		},
		{
			name: "Timer",
			input: &Timer{
				PRE: 10,
				ACC: 5,
				EN:  true,
				TT:  false,
				DN:  false,
			},
			want: []byte{
				0x00, 0x00, // Reserved
				0x00, 0x00, 0x00, 0x80, // Status (EN=1 -> bit 31 set -> 0x80000000)
				0x0A, 0x00, 0x00, 0x00, // PRE = 10
				0x05, 0x00, 0x00, 0x00, // ACC = 5
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Marshal(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !bytes.Equal(got, tt.want) {
				t.Errorf("Marshal() = %v, want %v", got, tt.want)
			}
		})
	}
}
