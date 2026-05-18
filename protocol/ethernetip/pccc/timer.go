package pccc

import (
	"encoding/binary"
	"fmt"
)

// SLC timer/counter/control elements occupy three 16-bit words:
//   word 0: control bits
//   word 1: PRE (preset) or LEN (length)
//   word 2: ACC (accumulated) or POS (position)
//
// This file decodes those 6-byte elements into Go structs.

// Timer is the decoded form of a 3-word SLC timer element (file type T,
// code 0x86). PRE and ACC are signed 16-bit values.
type Timer struct {
	Control uint16 // raw control word; use the bit accessors below
	PRE     int16  // preset
	ACC     int16  // accumulated
}

// EN reports the timer Enable bit (bit 15 of the control word).
func (t Timer) EN() bool { return t.Control&(1<<15) != 0 }

// TT reports the timer Timing bit (bit 14 of the control word).
func (t Timer) TT() bool { return t.Control&(1<<14) != 0 }

// DN reports the timer Done bit (bit 13 of the control word).
func (t Timer) DN() bool { return t.Control&(1<<13) != 0 }

// String returns a compact diagnostic representation of the timer.
func (t Timer) String() string {
	return fmt.Sprintf("Timer{EN=%t TT=%t DN=%t PRE=%d ACC=%d}", t.EN(), t.TT(), t.DN(), t.PRE, t.ACC)
}

// DecodeTimer parses the 6 raw little-endian bytes of an SLC timer element.
func DecodeTimer(b []byte) (Timer, error) {
	if len(b) < 6 {
		return Timer{}, fmt.Errorf("pccc: timer requires 6 bytes, got %d", len(b))
	}
	return Timer{
		Control: binary.LittleEndian.Uint16(b[0:2]),
		PRE:     int16(binary.LittleEndian.Uint16(b[2:4])),
		ACC:     int16(binary.LittleEndian.Uint16(b[4:6])),
	}, nil
}

// Counter is the decoded form of a 3-word SLC counter element (file type
// C, code 0x87).
type Counter struct {
	Control uint16
	PRE     int16
	ACC     int16
}

// CU reports the Count Up bit (bit 15 of the control word).
func (c Counter) CU() bool { return c.Control&(1<<15) != 0 }

// CD reports the Count Down bit (bit 14 of the control word).
func (c Counter) CD() bool { return c.Control&(1<<14) != 0 }

// DN reports the Done bit (bit 13 of the control word).
func (c Counter) DN() bool { return c.Control&(1<<13) != 0 }

// OV reports the Overflow bit (bit 12 of the control word).
func (c Counter) OV() bool { return c.Control&(1<<12) != 0 }

// UN reports the Underflow bit (bit 11 of the control word).
func (c Counter) UN() bool { return c.Control&(1<<11) != 0 }

// String returns a compact diagnostic representation of the counter.
func (c Counter) String() string {
	return fmt.Sprintf("Counter{CU=%t CD=%t DN=%t OV=%t UN=%t PRE=%d ACC=%d}",
		c.CU(), c.CD(), c.DN(), c.OV(), c.UN(), c.PRE, c.ACC)
}

// DecodeCounter parses the 6 raw little-endian bytes of an SLC counter element.
func DecodeCounter(b []byte) (Counter, error) {
	if len(b) < 6 {
		return Counter{}, fmt.Errorf("pccc: counter requires 6 bytes, got %d", len(b))
	}
	return Counter{
		Control: binary.LittleEndian.Uint16(b[0:2]),
		PRE:     int16(binary.LittleEndian.Uint16(b[2:4])),
		ACC:     int16(binary.LittleEndian.Uint16(b[4:6])),
	}, nil
}
