package modbus

import "fmt"

// HoldingRegister identifies a range of Modbus holding registers.
// It implements plc.DataPoint.
type HoldingRegister struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable representation of the data point.
func (h HoldingRegister) String() string {
	return fmt.Sprintf("HoldingRegister(addr=%d, qty=%d)", h.Addr, h.Qty)
}

// InputRegister identifies a range of Modbus input registers.
// It implements plc.DataPoint.
type InputRegister struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable representation of the data point.
func (r InputRegister) String() string {
	return fmt.Sprintf("InputRegister(addr=%d, qty=%d)", r.Addr, r.Qty)
}

// Coil identifies a range of Modbus coils.
// It implements plc.DataPoint.
type Coil struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable representation of the data point.
func (c Coil) String() string {
	return fmt.Sprintf("Coil(addr=%d, qty=%d)", c.Addr, c.Qty)
}

// DiscreteInput identifies a range of Modbus discrete inputs.
// It implements plc.DataPoint.
type DiscreteInput struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable representation of the data point.
func (d DiscreteInput) String() string {
	return fmt.Sprintf("DiscreteInput(addr=%d, qty=%d)", d.Addr, d.Qty)
}
