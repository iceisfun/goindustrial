package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Timer represents a Rockwell Logix timer structure (TON, TOF, RTO).
// Rockwell timers are vendor-specific 14-byte structures stored in PLC memory.
//
// The wire layout is:
//
//	Offset 0-1:   Reserved (INT)
//	Offset 2-5:   Status bits (DINT) -- EN, TT, DN packed in the high bits
//	Offset 6-9:   PRE (DINT) -- preset value in milliseconds
//	Offset 10-13: ACC (DINT) -- accumulated time in milliseconds
//
// Use [DecodeTimer] or [Timer.UnmarshalCIP] to decode from raw bytes.
type Timer struct {
	PRE int32 // Preset (ms)
	ACC int32 // Accumulated (ms)
	EN  bool  // Enable
	TT  bool  // Timer Timing
	DN  bool  // Done
}

// Rockwell Logix timer status-bit positions within the 32-bit status DINT.
const (
	// TimerStatusEN is the bit position for Enable (timer is running).
	TimerStatusEN = 31
	// TimerStatusTT is the bit position for Timer Timing (ACC < PRE while enabled).
	TimerStatusTT = 30
	// TimerStatusDN is the bit position for Done (ACC >= PRE).
	TimerStatusDN = 29
)

// DecodeTimer decodes a byte slice into a Timer struct. It expects at least
// 14 bytes in the canonical Rockwell Logix timer memory layout.
func DecodeTimer(data []byte) (*Timer, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("insufficient data for Timer: expected at least 14 bytes, got %d", len(data))
	}

	// Offset 0-1: Reserved (skip)

	// Offset 2-5: Status Bits (DINT)
	status := int32(binary.LittleEndian.Uint32(data[2:6]))

	// Offset 6-9: PRE (DINT)
	pre := int32(binary.LittleEndian.Uint32(data[6:10]))

	// Offset 10-13: ACC (DINT)
	acc := int32(binary.LittleEndian.Uint32(data[10:14]))

	// Use uint32 for status bit checks to avoid overflow issues with bit 31
	statusU := uint32(status)

	t := &Timer{
		PRE: pre,
		ACC: acc,
		EN:  (statusU & (1 << TimerStatusEN)) != 0,
		TT:  (statusU & (1 << TimerStatusTT)) != 0,
		DN:  (statusU & (1 << TimerStatusDN)) != 0,
	}

	return t, nil
}

// UnmarshalCIP implements the [Unmarshaler] interface for Timer.
func (t *Timer) UnmarshalCIP(data []byte) error {
	decoded, err := DecodeTimer(data)
	if err != nil {
		return err
	}
	*t = *decoded
	return nil
}

// MarshalCIP implements the [Marshaler] interface for Timer, encoding the
// struct into the 14-byte Rockwell Logix timer memory layout.
func (t *Timer) MarshalCIP() ([]byte, error) {
	// Canonical Rockwell memory layout (14 bytes)
	// Offset 0-1: Reserved (INT) - 0x0000
	// Offset 2-5: Status Bits (DINT)
	// Offset 6-9: PRE (DINT)
	// Offset 10-13: ACC (DINT)

	buf := new(bytes.Buffer)

	// Reserved
	if err := binary.Write(buf, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}

	// Status Bits
	var status uint32
	if t.EN {
		status |= 1 << TimerStatusEN
	}
	if t.TT {
		status |= 1 << TimerStatusTT
	}
	if t.DN {
		status |= 1 << TimerStatusDN
	}
	if err := binary.Write(buf, binary.LittleEndian, status); err != nil {
		return nil, err
	}

	// PRE
	if err := binary.Write(buf, binary.LittleEndian, t.PRE); err != nil {
		return nil, err
	}

	// ACC
	if err := binary.Write(buf, binary.LittleEndian, t.ACC); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
