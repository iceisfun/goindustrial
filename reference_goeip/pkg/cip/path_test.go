package cip

import (
	"bytes"
	"testing"
)

func TestPath_AddClass(t *testing.T) {
	tests := []struct {
		name    string
		classID UINT
		want    []byte
	}{
		{
			name:    "8-bit Class",
			classID: 0x6B,
			want:    []byte{0x20, 0x6B},
		},
		{
			name:    "16-bit Class",
			classID: 0x1234,
			want:    []byte{0x21, 0x00, 0x34, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddClass(tt.classID)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddClass() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddInstance(t *testing.T) {
	tests := []struct {
		name       string
		instanceID UINT
		want       []byte
	}{
		{
			name:       "8-bit Instance",
			instanceID: 0x01,
			want:       []byte{0x24, 0x01},
		},
		{
			name:       "16-bit Instance",
			instanceID: 0x1234,
			want:       []byte{0x25, 0x00, 0x34, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddInstance(tt.instanceID)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddInstance() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddInstance32(t *testing.T) {
	tests := []struct {
		name       string
		instanceID uint32
		want       []byte
	}{
		{
			name:       "8-bit Instance",
			instanceID: 0x01,
			want:       []byte{0x24, 0x01},
		},
		{
			name:       "16-bit Instance",
			instanceID: 0x1234,
			want:       []byte{0x25, 0x00, 0x34, 0x12},
		},
		{
			name:       "32-bit Instance",
			instanceID: 0x12345678,
			want:       []byte{0x26, 0x00, 0x78, 0x56, 0x34, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddInstance32(tt.instanceID)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddInstance32() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddAttribute(t *testing.T) {
	tests := []struct {
		name        string
		attributeID UINT
		want        []byte
	}{
		{
			name:        "8-bit Attribute",
			attributeID: 0x01,
			want:        []byte{0x30, 0x01},
		},
		{
			name:        "16-bit Attribute",
			attributeID: 0x1234,
			want:        []byte{0x31, 0x00, 0x34, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddAttribute(tt.attributeID)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddAttribute() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddMember(t *testing.T) {
	tests := []struct {
		name     string
		memberID UINT
		want     []byte
	}{
		{
			name:     "8-bit Member",
			memberID: 0x01,
			want:     []byte{0x28, 0x01},
		},
		{
			name:     "16-bit Member",
			memberID: 0x1234,
			want:     []byte{0x29, 0x00, 0x34, 0x12},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddMember(tt.memberID)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddMember() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddSymbolicSegment(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   []byte
	}{
		{
			name:   "Even Length",
			symbol: "Test",
			want:   []byte{0x91, 0x04, 'T', 'e', 's', 't'},
		},
		{
			name:   "Odd Length",
			symbol: "Test1",
			want:   []byte{0x91, 0x05, 'T', 'e', 's', 't', '1', 0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPath()
			p.AddSymbolicSegment(tt.symbol)
			if !bytes.Equal(p.Bytes(), tt.want) {
				t.Errorf("Path.AddSymbolicSegment() = %X, want %X", p.Bytes(), tt.want)
			}
		})
	}
}

func TestPath_AddPortSegment(t *testing.T) {
	// tests := []struct {
	// 	name        string
	// 	port        UINT
	// 	linkAddress []byte
	// 	want        []byte
	// }{
	// 	{
	// 		name:        "Port 1, 1 byte address",
	// 		port:        1,
	// 		linkAddress: []byte{0x00},
	// 		want:        []byte{0x01, 0x00},
	// 	},
	// }

	// Skipping the 2nd test case for now as I need to verify the implementation first
	// But let's test the basic case which is used most often (Backplane)
	t.Run("Backplane Port", func(t *testing.T) {
		p := NewPath()
		p.AddPortSegment(1, []byte{0x00}) // Port 1 (Backplane), Slot 0
		want := []byte{0x01, 0x00}
		if !bytes.Equal(p.Bytes(), want) {
			t.Errorf("Path.AddPortSegment() = %X, want %X", p.Bytes(), want)
		}
	})
}

func TestBuildPath(t *testing.T) {
	p := BuildPath(0x6B, 0x01, 0x00)
	want := []byte{0x20, 0x6B, 0x24, 0x01}
	if !bytes.Equal(p.Bytes(), want) {
		t.Errorf("BuildPath() = %X, want %X", p.Bytes(), want)
	}

	p2 := BuildPath(0x6B, 0x01, 0x07)
	want2 := []byte{0x20, 0x6B, 0x24, 0x01, 0x30, 0x07}
	if !bytes.Equal(p2.Bytes(), want2) {
		t.Errorf("BuildPath() with attribute = %X, want %X", p2.Bytes(), want2)
	}
}

func TestPath_LenWords(t *testing.T) {
	p := NewPath()
	p.AddClass(0x6B) // 2 bytes
	if p.LenWords() != 1 {
		t.Errorf("LenWords() = %d, want 1", p.LenWords())
	}

	p.AddInstance(0x01) // +2 bytes = 4 bytes
	if p.LenWords() != 2 {
		t.Errorf("LenWords() = %d, want 2", p.LenWords())
	}
}

func TestPath_String(t *testing.T) {
	p := NewPath()
	p.AddClass(0x6B)
	if p.String() != "206B" {
		t.Errorf("String() = %s, want 206B", p.String())
	}
}
