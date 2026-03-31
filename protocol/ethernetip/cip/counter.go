package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Counter represents a Rockwell Logix counter structure (CTU, CTD, CTUD).
// Rockwell counters are vendor-specific structures stored in PLC memory.
//
// Two wire layouts are supported:
//
// 14-byte layout (older firmware / ControlLogix):
//
//	Offset 0-1:   Reserved (INT)
//	Offset 2-5:   Status bits (DINT) -- CU, CD, DN, OV, UN packed in the high bits
//	Offset 6-9:   PRE (DINT) -- preset value
//	Offset 10-13: ACC (DINT) -- accumulated count
//
// 12-byte layout (newer firmware / CompactLogix):
//
//	Offset 0-3:   Status bits (DINT) -- CU, CD, DN, OV, UN packed in the high bits
//	Offset 4-7:   PRE (DINT) -- preset value
//	Offset 8-11:  ACC (DINT) -- accumulated count
//
// Use [DecodeCounter] or [Counter.UnmarshalCIP] to decode from raw bytes.
type Counter struct {
	PRE int32 // Preset
	ACC int32 // Accumulated
	CU  bool  // Count Up
	CD  bool  // Count Down
	DN  bool  // Done
	OV  bool  // Overflow
	UN  bool  // Underflow
}

// Rockwell Logix counter status-bit positions within the 32-bit status DINT.
const (
	// CounterStatusCU is the bit position for Count Up enabled.
	CounterStatusCU = 31
	// CounterStatusCD is the bit position for Count Down enabled.
	CounterStatusCD = 30
	// CounterStatusDN is the bit position for Done (ACC >= PRE).
	CounterStatusDN = 29
	// CounterStatusOV is the bit position for Overflow.
	CounterStatusOV = 28
	// CounterStatusUN is the bit position for Underflow.
	CounterStatusUN = 27
)

// DecodeCounter decodes a byte slice into a Counter struct. It accepts 12 bytes
// (status + PRE + ACC) or 14 bytes (reserved + status + PRE + ACC).
func DecodeCounter(data []byte) (*Counter, error) {
	var statusOff, preOff, accOff int
	switch {
	case len(data) >= 14:
		// 14-byte layout: reserved(2) + status(4) + PRE(4) + ACC(4)
		statusOff, preOff, accOff = 2, 6, 10
	case len(data) >= 12:
		// 12-byte layout: status(4) + PRE(4) + ACC(4)
		statusOff, preOff, accOff = 0, 4, 8
	default:
		return nil, fmt.Errorf("insufficient data for Counter: expected at least 12 bytes, got %d", len(data))
	}

	statusU := binary.LittleEndian.Uint32(data[statusOff : statusOff+4])
	pre := int32(binary.LittleEndian.Uint32(data[preOff : preOff+4]))
	acc := int32(binary.LittleEndian.Uint32(data[accOff : accOff+4]))

	c := &Counter{
		PRE: pre,
		ACC: acc,
		CU:  (statusU & (1 << CounterStatusCU)) != 0,
		CD:  (statusU & (1 << CounterStatusCD)) != 0,
		DN:  (statusU & (1 << CounterStatusDN)) != 0,
		OV:  (statusU & (1 << CounterStatusOV)) != 0,
		UN:  (statusU & (1 << CounterStatusUN)) != 0,
	}

	return c, nil
}

// UnmarshalCIP implements the [Unmarshaler] interface for Counter.
func (c *Counter) UnmarshalCIP(data []byte) error {
	decoded, err := DecodeCounter(data)
	if err != nil {
		return err
	}
	*c = *decoded
	return nil
}

// MarshalCIP implements the [Marshaler] interface for Counter, encoding the
// struct into the 14-byte Rockwell Logix counter memory layout.
func (c *Counter) MarshalCIP() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Reserved
	if err := binary.Write(buf, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}

	// Status Bits
	var status uint32
	if c.CU {
		status |= 1 << CounterStatusCU
	}
	if c.CD {
		status |= 1 << CounterStatusCD
	}
	if c.DN {
		status |= 1 << CounterStatusDN
	}
	if c.OV {
		status |= 1 << CounterStatusOV
	}
	if c.UN {
		status |= 1 << CounterStatusUN
	}
	if err := binary.Write(buf, binary.LittleEndian, status); err != nil {
		return nil, err
	}

	// PRE
	if err := binary.Write(buf, binary.LittleEndian, c.PRE); err != nil {
		return nil, err
	}

	// ACC
	if err := binary.Write(buf, binary.LittleEndian, c.ACC); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
