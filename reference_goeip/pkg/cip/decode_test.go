package cip

import (
	"testing"
)

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		target  any
		want    any
		wantErr bool
	}{
		{
			name:   "int32",
			data:   []byte{0x01, 0x00, 0x00, 0x00}, // 1
			target: new(int32),
			want:   int32(1),
		},
		{
			name:   "float32",
			data:   []byte{0x00, 0x00, 0x80, 0x3F}, // 1.0
			target: new(float32),
			want:   float32(1.0),
		},
		{
			name: "Timer",
			data: []byte{
				0x00, 0x00, // Reserved
				0x00, 0x00, 0x00, 0x80, // Status (EN=1, TT=0, DN=0 -> bit 31 set -> 0x80000000)
				0x0A, 0x00, 0x00, 0x00, // PRE = 10
				0x05, 0x00, 0x00, 0x00, // ACC = 5
			},
			target: new(Timer),
			want: Timer{
				PRE: 10,
				ACC: 5,
				EN:  true,
				TT:  false,
				DN:  false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Unmarshal(tt.data, tt.target)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Compare values
				// For basic types
				if val, ok := tt.target.(*int32); ok {
					if *val != tt.want.(int32) {
						t.Errorf("Unmarshal() got = %v, want %v", *val, tt.want)
					}
				} else if val, ok := tt.target.(*float32); ok {
					if *val != tt.want.(float32) {
						t.Errorf("Unmarshal() got = %v, want %v", *val, tt.want)
					}
				} else if val, ok := tt.target.(*Timer); ok {
					wantTimer := tt.want.(Timer)
					if *val != wantTimer {
						t.Errorf("Unmarshal() got = %+v, want %+v", *val, wantTimer)
					}
				}
			}
		})
	}
}
