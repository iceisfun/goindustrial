package pccc

import "testing"

func TestDecodeTimer(t *testing.T) {
	// Control = 0xE000 (EN=1, TT=1, DN=1), PRE=1000 (0x03E8), ACC=250 (0x00FA)
	raw := []byte{0x00, 0xE0, 0xE8, 0x03, 0xFA, 0x00}
	tm, err := DecodeTimer(raw)
	if err != nil {
		t.Fatalf("DecodeTimer: %v", err)
	}
	if !tm.EN() || !tm.TT() || !tm.DN() {
		t.Errorf("bits: EN=%t TT=%t DN=%t", tm.EN(), tm.TT(), tm.DN())
	}
	if tm.PRE != 1000 {
		t.Errorf("PRE: got %d want 1000", tm.PRE)
	}
	if tm.ACC != 250 {
		t.Errorf("ACC: got %d want 250", tm.ACC)
	}
}

func TestDecodeTimerShort(t *testing.T) {
	if _, err := DecodeTimer([]byte{0x00, 0x00}); err == nil {
		t.Fatal("expected error for short input")
	}
}

func TestDecodeCounter(t *testing.T) {
	// Control = 0xF800 (CU=1, CD=1, DN=1, OV=1, UN=1), PRE=100, ACC=50
	raw := []byte{0x00, 0xF8, 0x64, 0x00, 0x32, 0x00}
	c, err := DecodeCounter(raw)
	if err != nil {
		t.Fatalf("DecodeCounter: %v", err)
	}
	if !c.CU() || !c.CD() || !c.DN() || !c.OV() || !c.UN() {
		t.Errorf("bits: %s", c)
	}
	if c.PRE != 100 || c.ACC != 50 {
		t.Errorf("PRE/ACC: got %d/%d want 100/50", c.PRE, c.ACC)
	}
}

func TestDecodeCounterShort(t *testing.T) {
	if _, err := DecodeCounter([]byte{}); err == nil {
		t.Fatal("expected error for short input")
	}
}
