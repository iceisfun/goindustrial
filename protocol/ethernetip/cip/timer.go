package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Timer represents a Rockwell Logix timer structure (TON, TOF, RTO).
// Rockwell timers are vendor-specific structures stored in PLC memory.
//
// Two wire layouts are supported:
//
// 14-byte layout (older firmware / ControlLogix):
//
//	Offset 0-1:   Reserved (INT)
//	Offset 2-5:   Status bits (DINT) -- EN, TT, DN packed in the high bits
//	Offset 6-9:   PRE (DINT) -- preset value in milliseconds
//	Offset 10-13: ACC (DINT) -- accumulated time in milliseconds
//
// 12-byte layout (newer firmware / CompactLogix):
//
//	Offset 0-3:   Status bits (DINT) -- EN, TT, DN packed in the high bits
//	Offset 4-7:   PRE (DINT) -- preset value in milliseconds
//	Offset 8-11:  ACC (DINT) -- accumulated time in milliseconds
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

// DecodeTimer decodes a byte slice into a Timer struct. It accepts 12 bytes
// (status + PRE + ACC) or 14 bytes (reserved + status + PRE + ACC).
func DecodeTimer(data []byte) (*Timer, error) {
	var statusOff, preOff, accOff int
	switch {
	case len(data) >= 14:
		// 14-byte layout: reserved(2) + status(4) + PRE(4) + ACC(4)
		statusOff, preOff, accOff = 2, 6, 10
	case len(data) >= 12:
		// 12-byte layout: status(4) + PRE(4) + ACC(4)
		statusOff, preOff, accOff = 0, 4, 8
	default:
		return nil, fmt.Errorf("insufficient data for Timer: expected at least 12 bytes, got %d", len(data))
	}

	statusU := binary.LittleEndian.Uint32(data[statusOff : statusOff+4])
	pre := int32(binary.LittleEndian.Uint32(data[preOff : preOff+4]))
	acc := int32(binary.LittleEndian.Uint32(data[accOff : accOff+4]))

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
