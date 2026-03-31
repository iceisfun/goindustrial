package rockwell

import (
	"testing"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func TestTimerRoundTrip(t *testing.T) {
	// Register with a fake type code for testing.
	RegisterTimer(0x1001)

	codec := cip.LookupType(0x1001)
	if codec == nil {
		t.Fatal("LookupType(0x1001) returned nil")
	}

	tmr, ok := codec.(*Timer)
	if !ok {
		t.Fatalf("expected *Timer, got %T", codec)
	}

	// Build a 14-byte timer payload: reserved(2) + status(4) + PRE(4) + ACC(4)
	// EN=true (bit 31), DN=true (bit 29), PRE=5000, ACC=3000
	data := []byte{
		0x00, 0x00, // reserved
		0x00, 0x00, 0x00, 0xA0, // status: bits 31+29 set = 0xA0000000
		0x88, 0x13, 0x00, 0x00, // PRE = 5000
		0xB8, 0x0B, 0x00, 0x00, // ACC = 3000
	}

	if err := tmr.UnmarshalCIP(data); err != nil {
		t.Fatalf("UnmarshalCIP: %v", err)
	}

	if tmr.PRE != 5000 {
		t.Errorf("PRE = %d, want 5000", tmr.PRE)
	}
	if tmr.ACC != 3000 {
		t.Errorf("ACC = %d, want 3000", tmr.ACC)
	}
	if !tmr.EN {
		t.Error("EN should be true")
	}
	if tmr.TT {
		t.Error("TT should be false")
	}
	if !tmr.DN {
		t.Error("DN should be true")
	}

	// Marshal round-trip.
	out, err := tmr.MarshalCIP()
	if err != nil {
		t.Fatalf("MarshalCIP: %v", err)
	}
	if len(out) != 14 {
		t.Fatalf("MarshalCIP: got %d bytes, want 14", len(out))
	}

	// Verify CIPType and String.
	if tmr.CIPType() != 0x1001 {
		t.Errorf("CIPType = 0x%04X, want 0x1001", uint16(tmr.CIPType()))
	}
	if tmr.String() != "TIMER" {
		t.Errorf("String = %q, want %q", tmr.String(), "TIMER")
	}
}

func TestCounterRoundTrip(t *testing.T) {
	RegisterCounter(0x1002)

	codec := cip.LookupType(0x1002)
	if codec == nil {
		t.Fatal("LookupType(0x1002) returned nil")
	}

	ctr, ok := codec.(*Counter)
	if !ok {
		t.Fatalf("expected *Counter, got %T", codec)
	}

	// CU=true (bit 31), DN=true (bit 29), PRE=100, ACC=42
	data := []byte{
		0x00, 0x00, // reserved
		0x00, 0x00, 0x00, 0xA0, // status: bits 31+29 set
		0x64, 0x00, 0x00, 0x00, // PRE = 100
		0x2A, 0x00, 0x00, 0x00, // ACC = 42
	}

	if err := ctr.UnmarshalCIP(data); err != nil {
		t.Fatalf("UnmarshalCIP: %v", err)
	}

	if ctr.PRE != 100 {
		t.Errorf("PRE = %d, want 100", ctr.PRE)
	}
	if ctr.ACC != 42 {
		t.Errorf("ACC = %d, want 42", ctr.ACC)
	}
	if !ctr.CU {
		t.Error("CU should be true")
	}
	if !ctr.DN {
		t.Error("DN should be true")
	}
}

func TestControlRoundTrip(t *testing.T) {
	RegisterControl(0x1003)

	codec := cip.LookupType(0x1003)
	if codec == nil {
		t.Fatal("LookupType(0x1003) returned nil")
	}

	ctrl, ok := codec.(*Control)
	if !ok {
		t.Fatalf("expected *Control, got %T", codec)
	}

	// EN=true (bit 31), DN=true (bit 29), LEN=50, POS=25
	data := []byte{
		0x00, 0x00, // reserved
		0x00, 0x00, 0x00, 0xA0, // status: bits 31+29 set
		0x32, 0x00, 0x00, 0x00, // LEN = 50
		0x19, 0x00, 0x00, 0x00, // POS = 25
	}

	if err := ctrl.UnmarshalCIP(data); err != nil {
		t.Fatalf("UnmarshalCIP: %v", err)
	}

	if ctrl.LEN != 50 {
		t.Errorf("LEN = %d, want 50", ctrl.LEN)
	}
	if ctrl.POS != 25 {
		t.Errorf("POS = %d, want 25", ctrl.POS)
	}
	if !ctrl.EN {
		t.Error("EN should be true")
	}
	if !ctrl.DN {
		t.Error("DN should be true")
	}
	if ctrl.EM {
		t.Error("EM should be false")
	}

	// Marshal round-trip.
	out, err := ctrl.MarshalCIP()
	if err != nil {
		t.Fatalf("MarshalCIP: %v", err)
	}
	if len(out) != 14 {
		t.Fatalf("MarshalCIP: got %d bytes, want 14", len(out))
	}

	// Unmarshal the round-tripped bytes into a fresh Control.
	var ctrl2 Control
	ctrl2.dt = 0x1003
	if err := ctrl2.UnmarshalCIP(out); err != nil {
		t.Fatalf("round-trip UnmarshalCIP: %v", err)
	}
	if ctrl2.LEN != 50 || ctrl2.POS != 25 || !ctrl2.EN || !ctrl2.DN {
		t.Errorf("round-trip mismatch: %+v", ctrl2)
	}
}

func TestDataTypeStringResolvesRegistered(t *testing.T) {
	// 0x1001 was registered as Timer above.
	got := cip.DataType(0x1001).String()
	if got != "TIMER" {
		t.Errorf("DataType(0x1001).String() = %q, want %q", got, "TIMER")
	}

	// Array variant.
	got = cip.DataType(0x1001 | 0x8000).String()
	if got != "TIMER[]" {
		t.Errorf("DataType(0x9001).String() = %q, want %q", got, "TIMER[]")
	}
}

func TestGoTypeToCIPTypeWithCodec(t *testing.T) {
	tmr := &Timer{dt: 0x1001}
	dt, err := cip.GoTypeToCIPType(tmr)
	if err != nil {
		t.Fatalf("GoTypeToCIPType: %v", err)
	}
	if dt != 0x1001 {
		t.Errorf("GoTypeToCIPType = 0x%04X, want 0x1001", uint16(dt))
	}
}

func TestDuplicateRegisterPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate RegisterType")
		}
	}()
	// 0x1001 already registered.
	RegisterTimer(0x1001)
}
