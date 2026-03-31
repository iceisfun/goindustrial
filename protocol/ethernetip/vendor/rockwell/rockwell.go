// Package rockwell provides [cip.TypeCodec] implementations for well-known
// Rockwell Automation (Allen-Bradley) Logix structured types such as Timer
// (TON/TOF/RTO), Counter (CTU/CTD/CTUD), PID, and CONTROL.
//
// Rockwell struct types are identified by vendor-specific CIP DataType codes.
// Because these codes can vary between controller models and firmware versions,
// the package does not auto-register types. Instead, call the Register
// functions with the DataType codes discovered from your controller (e.g. via
// [ethernetip.Client.ListTags]):
//
//	// Use type codes discovered from your controller via ListTags:
//	rockwell.RegisterTimer(0x0F83)
//	rockwell.RegisterCounter(0x0F84)
//
// This registers the types in the global [cip.RegisterType] registry, enabling
// automatic name resolution in [cip.DataType.String] and codec lookup via
// [cip.LookupType].
package rockwell

import "github.com/iceisfun/goindustrial/protocol/ethernetip/cip"

// ---------- Timer ----------

// Timer wraps [cip.Timer] as a [cip.TypeCodec] with a controller-specific CIP
// DataType code. Use [RegisterTimer] to register it in the type registry.
type Timer struct {
	cip.Timer
	dt cip.DataType
}

func (t *Timer) CIPType() cip.DataType { return t.dt }
func (t *Timer) String() string         { return "TIMER" }

// RegisterTimer registers [cip.Timer] as a [cip.TypeCodec] for the given
// DataType code. The code is controller-specific — use
// [ethernetip.Client.ListTags] to discover it.
func RegisterTimer(dt cip.DataType) {
	cip.RegisterType(dt, func() cip.TypeCodec {
		return &Timer{dt: dt}
	})
}

// ---------- Counter ----------

// Counter wraps [cip.Counter] as a [cip.TypeCodec] with a controller-specific
// CIP DataType code. Use [RegisterCounter] to register it in the type registry.
type Counter struct {
	cip.Counter
	dt cip.DataType
}

func (c *Counter) CIPType() cip.DataType { return c.dt }
func (c *Counter) String() string         { return "COUNTER" }

// RegisterCounter registers [cip.Counter] as a [cip.TypeCodec] for the given
// DataType code. The code is controller-specific — use
// [ethernetip.Client.ListTags] to discover it.
func RegisterCounter(dt cip.DataType) {
	cip.RegisterType(dt, func() cip.TypeCodec {
		return &Counter{dt: dt}
	})
}

// ---------- PID ----------

// PID represents a Rockwell Logix PID instruction structure. The wire layout
// is a 116-byte vendor-specific struct.
//
// This is a minimal representation exposing the most commonly accessed fields.
// For complete PID configuration, use ReadTagInto with a custom struct.
type PID struct {
	SP  float32 // Setpoint
	PV  float32 // Process Variable
	OUT float32 // Output (%)
	ERR float32 // Error (SP - PV)
	KP  float32 // Proportional gain
	KI  float32 // Integral gain
	KD  float32 // Derivative gain

	dt cip.DataType
}

func (p *PID) CIPType() cip.DataType { return p.dt }
func (p *PID) String() string         { return "PID" }

// UnmarshalCIP decodes the first 28 bytes of a Rockwell PID struct into the
// commonly-used fields. The full 116-byte layout contains tuning limits,
// scaling, and mode bits not captured here.
func (p *PID) UnmarshalCIP(data []byte) error {
	if len(data) < 28 {
		return cip.Error{Status: cip.StatusNotEnoughData}
	}
	var w pidWire
	if err := cip.Unmarshal(data[:28], &w); err != nil {
		return err
	}
	p.SP = w.SP
	p.PV = w.PV
	p.OUT = w.OUT
	p.ERR = w.ERR
	p.KP = w.KP
	p.KI = w.KI
	p.KD = w.KD
	return nil
}

// MarshalCIP encodes the PID fields. Note: writing individual PID members
// (e.g. "MyPID.SP") is usually preferred over writing the entire struct.
func (p *PID) MarshalCIP() ([]byte, error) {
	return cip.Marshal(pidWire{
		SP: p.SP, PV: p.PV, OUT: p.OUT, ERR: p.ERR,
		KP: p.KP, KI: p.KI, KD: p.KD,
	})
}

// pidWire is the binary-compatible layout for the first 28 bytes of the PID.
type pidWire struct {
	SP, PV, OUT, ERR, KP, KI, KD float32
}

// RegisterPID registers the [PID] type for the given DataType code.
func RegisterPID(dt cip.DataType) {
	cip.RegisterType(dt, func() cip.TypeCodec {
		return &PID{dt: dt}
	})
}

// ---------- CONTROL ----------

// Control represents a Rockwell Logix CONTROL structure used by instructions
// like FAL, FSC, COP, etc. The wire layout is 14 bytes — same size as Timer
// and Counter.
//
//	Offset 0-1:   Reserved (INT)
//	Offset 2-5:   Status bits (DINT) — EN, EU, DN, EM, ER, UL, IN, FD
//	Offset 6-9:   LEN (DINT) — array length
//	Offset 10-13: POS (DINT) — current position
type Control struct {
	LEN int32 // Array length
	POS int32 // Current position
	EN  bool  // Enable
	EU  bool  // Enable Unload
	DN  bool  // Done
	EM  bool  // Empty
	ER  bool  // Error
	UL  bool  // Unload
	IN  bool  // Inhibit
	FD  bool  // Found

	dt cip.DataType
}

func (c *Control) CIPType() cip.DataType { return c.dt }
func (c *Control) String() string         { return "CONTROL" }

// Control status-bit positions within the 32-bit status DINT.
const (
	ControlStatusEN = 31
	ControlStatusEU = 30
	ControlStatusDN = 29
	ControlStatusEM = 28
	ControlStatusER = 27
	ControlStatusUL = 26
	ControlStatusIN = 25
	ControlStatusFD = 24
)

// UnmarshalCIP decodes the 14-byte Rockwell CONTROL layout.
func (c *Control) UnmarshalCIP(data []byte) error {
	if len(data) < 14 {
		return cip.Error{Status: cip.StatusNotEnoughData}
	}
	return cip.Unmarshal(data, &controlWire{ctrl: c})
}

// MarshalCIP encodes the Control into the 14-byte Rockwell layout.
func (c *Control) MarshalCIP() ([]byte, error) {
	return cip.Marshal(&controlWire{ctrl: c})
}

// RegisterControl registers the [Control] type for the given DataType code.
func RegisterControl(dt cip.DataType) {
	cip.RegisterType(dt, func() cip.TypeCodec {
		return &Control{dt: dt}
	})
}

// controlWire handles the 14-byte binary encoding for Control, mirroring the
// approach used by cip.Timer and cip.Counter.
type controlWire struct{ ctrl *Control }

func (w *controlWire) UnmarshalCIP(data []byte) error {
	c := w.ctrl
	_ = data[13] // bounds check

	status := uint32(data[2]) | uint32(data[3])<<8 | uint32(data[4])<<16 | uint32(data[5])<<24
	c.LEN = int32(data[6]) | int32(data[7])<<8 | int32(data[8])<<16 | int32(data[9])<<24
	c.POS = int32(data[10]) | int32(data[11])<<8 | int32(data[12])<<16 | int32(data[13])<<24

	c.EN = (status & (1 << ControlStatusEN)) != 0
	c.EU = (status & (1 << ControlStatusEU)) != 0
	c.DN = (status & (1 << ControlStatusDN)) != 0
	c.EM = (status & (1 << ControlStatusEM)) != 0
	c.ER = (status & (1 << ControlStatusER)) != 0
	c.UL = (status & (1 << ControlStatusUL)) != 0
	c.IN = (status & (1 << ControlStatusIN)) != 0
	c.FD = (status & (1 << ControlStatusFD)) != 0
	return nil
}

func (w *controlWire) MarshalCIP() ([]byte, error) {
	c := w.ctrl
	var buf [14]byte
	// Offset 0-1: Reserved

	var status uint32
	if c.EN {
		status |= 1 << ControlStatusEN
	}
	if c.EU {
		status |= 1 << ControlStatusEU
	}
	if c.DN {
		status |= 1 << ControlStatusDN
	}
	if c.EM {
		status |= 1 << ControlStatusEM
	}
	if c.ER {
		status |= 1 << ControlStatusER
	}
	if c.UL {
		status |= 1 << ControlStatusUL
	}
	if c.IN {
		status |= 1 << ControlStatusIN
	}
	if c.FD {
		status |= 1 << ControlStatusFD
	}
	buf[2] = byte(status)
	buf[3] = byte(status >> 8)
	buf[4] = byte(status >> 16)
	buf[5] = byte(status >> 24)

	buf[6] = byte(c.LEN)
	buf[7] = byte(c.LEN >> 8)
	buf[8] = byte(c.LEN >> 16)
	buf[9] = byte(c.LEN >> 24)

	buf[10] = byte(c.POS)
	buf[11] = byte(c.POS >> 8)
	buf[12] = byte(c.POS >> 16)
	buf[13] = byte(c.POS >> 24)

	return buf[:], nil
}
