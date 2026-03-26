package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Counter represents a Rockwell Logix Counter structure (CTU, CTD).
//
// Memory Layout (similar to Timer):
// Offset 0-1: Reserved (INT) - often ignored or part of the previous word alignment
// Offset 2-5: Status Bits (DINT) - CU, CD, DN, OV, UN packed here
// Offset 6-9: PRE (DINT)
// Offset 10-13: ACC (DINT)
//
// Total size is typically 12-14 bytes depending on alignment/packing.
// We expect at least 12 bytes if just raw DINTs, but typically 14 bytes with the initial 2-byte pad/reserved.
type Counter struct {
	PRE int32 // Preset
	ACC int32 // Accumulated
	CU  bool  // Count Up
	CD  bool  // Count Down
	DN  bool  // Done
	OV  bool  // Overflow
	UN  bool  // Underflow
}

const (
	// CounterStatusCU is the bit position for Count Up
	CounterStatusCU = 31
	// CounterStatusCD is the bit position for Count Down
	CounterStatusCD = 30
	// CounterStatusDN is the bit position for Done
	CounterStatusDN = 29
	// CounterStatusOV is the bit position for Overflow
	CounterStatusOV = 28
	// CounterStatusUN is the bit position for Underflow
	CounterStatusUN = 27
)

// DecodeCounter decodes a byte slice into a Counter struct.
// It expects the canonical Rockwell memory layout (14 bytes).
func DecodeCounter(data []byte) (*Counter, error) {
	if len(data) < 14 {
		return nil, fmt.Errorf("insufficient data for Counter: expected at least 14 bytes, got %d", len(data))
	}

	// Offset 0-1: Reserved (skip)

	// Offset 2-5: Status Bits (DINT)
	status := int32(binary.LittleEndian.Uint32(data[2:6]))

	// Offset 6-9: PRE (DINT)
	pre := int32(binary.LittleEndian.Uint32(data[6:10]))

	// Offset 10-13: ACC (DINT)
	acc := int32(binary.LittleEndian.Uint32(data[10:14]))

	// Use uint32 for status bit checks
	statusU := uint32(status)

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

// UnmarshalCIP implements the Unmarshaler interface for Counter.
func (c *Counter) UnmarshalCIP(data []byte) error {
	decoded, err := DecodeCounter(data)
	if err != nil {
		return err
	}
	*c = *decoded
	return nil
}

// MarshalCIP implements the Marshaler interface for Counter.
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
