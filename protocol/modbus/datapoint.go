package modbus

import (
	"fmt"

	"github.com/iceisfun/goindustrial/plc"
)

// HoldingRegister identifies a range of Modbus holding registers.
// It implements plc.DataPoint and monitor.Clusterable.
type HoldingRegister struct {
	Addr Address
	Qty  Quantity
}

func (h HoldingRegister) String() string {
	return fmt.Sprintf("HoldingRegister(addr=%d, qty=%d)", h.Addr, h.Qty)
}

func (h HoldingRegister) ClusterKey() string        { return "holding_register" }
func (h HoldingRegister) ClusterAddr() uint16        { return uint16(h.Addr) }
func (h HoldingRegister) ClusterQty() uint16         { return uint16(h.Qty) }
func (h HoldingRegister) ClusterBitsPerUnit() uint16 { return 16 }

func (h HoldingRegister) ClusterMerge(start, count uint16) plc.DataPoint {
	return HoldingRegister{Addr: Address(start), Qty: Quantity(count)}
}

func (h HoldingRegister) ClusterExtract(val plc.Value, clusterStart uint16) plc.Value {
	offset := int(uint16(h.Addr)-clusterStart) * 2
	size := int(h.Qty) * 2
	raw := make([]byte, size)
	copy(raw, val.Raw[offset:offset+size])
	dt := plc.TypeUint16
	if h.Qty > 1 {
		dt = plc.TypeBytes
	}
	return plc.Value{
		Raw:       raw,
		Type:      dt,
		ByteOrder: val.ByteOrder,
	}
}

// InputRegister identifies a range of Modbus input registers.
// It implements plc.DataPoint and monitor.Clusterable.
type InputRegister struct {
	Addr Address
	Qty  Quantity
}

func (r InputRegister) String() string {
	return fmt.Sprintf("InputRegister(addr=%d, qty=%d)", r.Addr, r.Qty)
}

func (r InputRegister) ClusterKey() string        { return "input_register" }
func (r InputRegister) ClusterAddr() uint16        { return uint16(r.Addr) }
func (r InputRegister) ClusterQty() uint16         { return uint16(r.Qty) }
func (r InputRegister) ClusterBitsPerUnit() uint16 { return 16 }

func (r InputRegister) ClusterMerge(start, count uint16) plc.DataPoint {
	return InputRegister{Addr: Address(start), Qty: Quantity(count)}
}

func (r InputRegister) ClusterExtract(val plc.Value, clusterStart uint16) plc.Value {
	offset := int(uint16(r.Addr)-clusterStart) * 2
	size := int(r.Qty) * 2
	raw := make([]byte, size)
	copy(raw, val.Raw[offset:offset+size])
	dt := plc.TypeUint16
	if r.Qty > 1 {
		dt = plc.TypeBytes
	}
	return plc.Value{
		Raw:       raw,
		Type:      dt,
		ByteOrder: val.ByteOrder,
	}
}

// Coil identifies a range of Modbus coils.
// It implements plc.DataPoint and monitor.Clusterable.
type Coil struct {
	Addr Address
	Qty  Quantity
}

func (c Coil) String() string {
	return fmt.Sprintf("Coil(addr=%d, qty=%d)", c.Addr, c.Qty)
}

func (c Coil) ClusterKey() string        { return "coil" }
func (c Coil) ClusterAddr() uint16        { return uint16(c.Addr) }
func (c Coil) ClusterQty() uint16         { return uint16(c.Qty) }
func (c Coil) ClusterBitsPerUnit() uint16 { return 1 }

func (c Coil) ClusterMerge(start, count uint16) plc.DataPoint {
	return Coil{Addr: Address(start), Qty: Quantity(count)}
}

func (c Coil) ClusterExtract(val plc.Value, clusterStart uint16) plc.Value {
	return extractBits(val, uint16(c.Addr), uint16(c.Qty), clusterStart)
}

// DiscreteInput identifies a range of Modbus discrete inputs.
// It implements plc.DataPoint and monitor.Clusterable.
type DiscreteInput struct {
	Addr Address
	Qty  Quantity
}

func (d DiscreteInput) String() string {
	return fmt.Sprintf("DiscreteInput(addr=%d, qty=%d)", d.Addr, d.Qty)
}

func (d DiscreteInput) ClusterKey() string        { return "discrete_input" }
func (d DiscreteInput) ClusterAddr() uint16        { return uint16(d.Addr) }
func (d DiscreteInput) ClusterQty() uint16         { return uint16(d.Qty) }
func (d DiscreteInput) ClusterBitsPerUnit() uint16 { return 1 }

func (d DiscreteInput) ClusterMerge(start, count uint16) plc.DataPoint {
	return DiscreteInput{Addr: Address(start), Qty: Quantity(count)}
}

func (d DiscreteInput) ClusterExtract(val plc.Value, clusterStart uint16) plc.Value {
	return extractBits(val, uint16(d.Addr), uint16(d.Qty), clusterStart)
}

// extractBits extracts a sub-range of packed bits from a larger bit array.
// Used by Coil and DiscreteInput for cluster extraction.
func extractBits(val plc.Value, addr, qty, clusterStart uint16) plc.Value {
	bitOffset := int(addr - clusterStart)
	outBytes := (int(qty) + 7) / 8
	raw := make([]byte, outBytes)
	for i := 0; i < int(qty); i++ {
		srcBit := bitOffset + i
		if val.Raw[srcBit/8]&(1<<uint(srcBit%8)) != 0 {
			raw[i/8] |= 1 << uint(i%8)
		}
	}
	return plc.Value{
		Raw:       raw,
		Type:      plc.TypeBool,
		ByteOrder: val.ByteOrder,
	}
}
