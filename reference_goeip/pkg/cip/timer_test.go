package cip

import (
	"encoding/binary"
	"testing"
)

func TestDecodeTimer(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    *Timer
		wantErr bool
	}{
		{
			name: "Valid Timer - All Bits Set",
			data: func() []byte {
				buf := make([]byte, 14)
				// Status: EN(31) | TT(30) | DN(29) = 0xE0000000
				binary.LittleEndian.PutUint32(buf[2:6], 0xE0000000)
				// PRE: 1000
				binary.LittleEndian.PutUint32(buf[6:10], 1000)
				// ACC: 500
				binary.LittleEndian.PutUint32(buf[10:14], 500)
				return buf
			}(),
			want: &Timer{
				PRE: 1000,
				ACC: 500,
				EN:  true,
				TT:  true,
				DN:  true,
			},
			wantErr: false,
		},
		{
			name: "Valid Timer - No Bits Set",
			data: func() []byte {
				buf := make([]byte, 14)
				// Status: 0
				binary.LittleEndian.PutUint32(buf[2:6], 0)
				// PRE: 5000
				binary.LittleEndian.PutUint32(buf[6:10], 5000)
				// ACC: 0
				binary.LittleEndian.PutUint32(buf[10:14], 0)
				return buf
			}(),
			want: &Timer{
				PRE: 5000,
				ACC: 0,
				EN:  false,
				TT:  false,
				DN:  false,
			},
			wantErr: false,
		},
		{
			name: "Valid Timer - EN Only",
			data: func() []byte {
				buf := make([]byte, 14)
				// Status: EN(31) = 0x80000000
				binary.LittleEndian.PutUint32(buf[2:6], 0x80000000)
				// PRE: 2000
				binary.LittleEndian.PutUint32(buf[6:10], 2000)
				// ACC: 100
				binary.LittleEndian.PutUint32(buf[10:14], 100)
				return buf
			}(),
			want: &Timer{
				PRE: 2000,
				ACC: 100,
				EN:  true,
				TT:  false,
				DN:  false,
			},
			wantErr: false,
		},
		{
			name:    "Insufficient Data",
			data:    make([]byte, 13),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Empty Data",
			data:    []byte{},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeTimer(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeTimer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.PRE != tt.want.PRE {
					t.Errorf("DecodeTimer() PRE = %v, want %v", got.PRE, tt.want.PRE)
				}
				if got.ACC != tt.want.ACC {
					t.Errorf("DecodeTimer() ACC = %v, want %v", got.ACC, tt.want.ACC)
				}
				if got.EN != tt.want.EN {
					t.Errorf("DecodeTimer() EN = %v, want %v", got.EN, tt.want.EN)
				}
				if got.TT != tt.want.TT {
					t.Errorf("DecodeTimer() TT = %v, want %v", got.TT, tt.want.TT)
				}
				if got.DN != tt.want.DN {
					t.Errorf("DecodeTimer() DN = %v, want %v", got.DN, tt.want.DN)
				}
			}
		})
	}
}

func TestTimer_MarshalCIP(t *testing.T) {
	timer := &Timer{
		PRE: 1000,
		ACC: 500,
		EN:  true,
		TT:  true,
		DN:  true,
	}

	got, err := timer.MarshalCIP()
	if err != nil {
		t.Fatalf("MarshalCIP() error = %v", err)
	}

	if len(got) != 14 {
		t.Errorf("MarshalCIP() length = %d, want 14", len(got))
	}

	// Check Reserved (0-1)
	if got[0] != 0 || got[1] != 0 {
		t.Errorf("Reserved bytes = %X, want 0000", got[0:2])
	}

	// Check Status (2-5)
	status := binary.LittleEndian.Uint32(got[2:6])
	expectedStatus := uint32(0)
	expectedStatus |= 1 << TimerStatusEN
	expectedStatus |= 1 << TimerStatusTT
	expectedStatus |= 1 << TimerStatusDN
	if status != expectedStatus {
		t.Errorf("Status = %08X, want %08X", status, expectedStatus)
	}

	// Check PRE (6-9)
	pre := int32(binary.LittleEndian.Uint32(got[6:10]))
	if pre != 1000 {
		t.Errorf("PRE = %d, want 1000", pre)
	}

	// Check ACC (10-13)
	acc := int32(binary.LittleEndian.Uint32(got[10:14]))
	if acc != 500 {
		t.Errorf("ACC = %d, want 500", acc)
	}
}

func TestTimer_UnmarshalCIP(t *testing.T) {
	data := make([]byte, 14)
	// Status: EN | TT
	binary.LittleEndian.PutUint32(data[2:6], (1<<TimerStatusEN)|(1<<TimerStatusTT))
	// PRE: 2000
	binary.LittleEndian.PutUint32(data[6:10], 2000)
	// ACC: 1000
	binary.LittleEndian.PutUint32(data[10:14], 1000)

	var timer Timer
	if err := timer.UnmarshalCIP(data); err != nil {
		t.Fatalf("UnmarshalCIP() error = %v", err)
	}

	if !timer.EN || !timer.TT || timer.DN {
		t.Errorf("Flags mismatch: EN=%v, TT=%v, DN=%v", timer.EN, timer.TT, timer.DN)
	}
	if timer.PRE != 2000 {
		t.Errorf("PRE = %d, want 2000", timer.PRE)
	}
	if timer.ACC != 1000 {
		t.Errorf("ACC = %d, want 1000", timer.ACC)
	}
}

func TestTimer_RoundTrip(t *testing.T) {
	original := &Timer{
		PRE: 12345,
		ACC: 6789,
		EN:  true,
		TT:  false,
		DN:  true,
	}

	data, err := original.MarshalCIP()
	if err != nil {
		t.Fatalf("MarshalCIP() error = %v", err)
	}

	var decoded Timer
	if err := decoded.UnmarshalCIP(data); err != nil {
		t.Fatalf("UnmarshalCIP() error = %v", err)
	}

	if original.PRE != decoded.PRE {
		t.Errorf("PRE mismatch: got %d, want %d", decoded.PRE, original.PRE)
	}
	if original.ACC != decoded.ACC {
		t.Errorf("ACC mismatch: got %d, want %d", decoded.ACC, original.ACC)
	}
	if original.EN != decoded.EN {
		t.Errorf("EN mismatch: got %v, want %v", decoded.EN, original.EN)
	}
	if original.TT != decoded.TT {
		t.Errorf("TT mismatch: got %v, want %v", decoded.TT, original.TT)
	}
	if original.DN != decoded.DN {
		t.Errorf("DN mismatch: got %v, want %v", decoded.DN, original.DN)
	}
}
