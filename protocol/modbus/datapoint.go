package modbus

import (
	"fmt"

	"github.com/iceisfun/goindustrial/plc"
)

// HoldingRegister identifies a contiguous range of Modbus holding registers
// (16-bit read/write data locations, function codes 3/6/16). It implements
// [plc.DataPoint] and the Clusterable interface for read optimization.
type HoldingRegister struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable description of the register range.
func (h HoldingRegister) String() string {
	return fmt.Sprintf("HoldingRegister(addr=%d, qty=%d)", h.Addr, h.Qty)
}

// ClusterKey returns a key that groups this data point with other holding registers for read merging.
func (h HoldingRegister) ClusterKey() string { return "holding_register" }

// ClusterAddr returns the starting register address for clustering.
func (h HoldingRegister) ClusterAddr() uint16 { return uint16(h.Addr) }

// ClusterQty returns the number of registers in this data point.
func (h HoldingRegister) ClusterQty() uint16 { return uint16(h.Qty) }

// ClusterBitsPerUnit returns 16, the bit width of a single holding register.
func (h HoldingRegister) ClusterBitsPerUnit() uint16 { return 16 }

// ClusterMerge creates a new HoldingRegister that spans a merged address range,
// allowing the monitor to combine adjacent register reads into a single request.
func (h HoldingRegister) ClusterMerge(start, count uint16) plc.DataPoint {
	return HoldingRegister{Addr: Address(start), Qty: Quantity(count)}
}

// ClusterExtract extracts this data point's value from a larger clustered read
// result by computing the byte offset within the merged response.
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

// InputRegister identifies a contiguous range of Modbus input registers
// (16-bit read-only data, function code 4). It implements [plc.DataPoint]
// and the Clusterable interface for read optimization.
type InputRegister struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable description of the register range.
func (r InputRegister) String() string {
	return fmt.Sprintf("InputRegister(addr=%d, qty=%d)", r.Addr, r.Qty)
}

// ClusterKey returns a key that groups this data point with other input registers for read merging.
func (r InputRegister) ClusterKey() string { return "input_register" }

// ClusterAddr returns the starting register address for clustering.
func (r InputRegister) ClusterAddr() uint16 { return uint16(r.Addr) }

// ClusterQty returns the number of registers in this data point.
func (r InputRegister) ClusterQty() uint16 { return uint16(r.Qty) }

// ClusterBitsPerUnit returns 16, the bit width of a single input register.
func (r InputRegister) ClusterBitsPerUnit() uint16 { return 16 }

// ClusterMerge creates a new InputRegister that spans a merged address range,
// allowing the monitor to combine adjacent register reads into a single request.
func (r InputRegister) ClusterMerge(start, count uint16) plc.DataPoint {
	return InputRegister{Addr: Address(start), Qty: Quantity(count)}
}

// ClusterExtract extracts this data point's value from a larger clustered read
// result by computing the byte offset within the merged response.
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

// Coil identifies a contiguous range of Modbus coils (single-bit read/write
// outputs, function codes 1/5/15). It implements [plc.DataPoint] and the
// Clusterable interface for read optimization.
type Coil struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable description of the coil range.
func (c Coil) String() string {
	return fmt.Sprintf("Coil(addr=%d, qty=%d)", c.Addr, c.Qty)
}

// ClusterKey returns a key that groups this data point with other coils for read merging.
func (c Coil) ClusterKey() string { return "coil" }

// ClusterAddr returns the starting coil address for clustering.
func (c Coil) ClusterAddr() uint16 { return uint16(c.Addr) }

// ClusterQty returns the number of coils in this data point.
func (c Coil) ClusterQty() uint16 { return uint16(c.Qty) }

// ClusterBitsPerUnit returns 1, the bit width of a single coil.
func (c Coil) ClusterBitsPerUnit() uint16 { return 1 }

// ClusterMerge creates a new Coil that spans a merged address range, allowing
// the monitor to combine adjacent coil reads into a single request.
func (c Coil) ClusterMerge(start, count uint16) plc.DataPoint {
	return Coil{Addr: Address(start), Qty: Quantity(count)}
}

// ClusterExtract extracts this data point's coil bits from a larger clustered
// read result by computing the bit offset within the packed response.
func (c Coil) ClusterExtract(val plc.Value, clusterStart uint16) plc.Value {
	return extractBits(val, uint16(c.Addr), uint16(c.Qty), clusterStart)
}

// DiscreteInput identifies a contiguous range of Modbus discrete inputs
// (single-bit read-only inputs, function code 2). It implements [plc.DataPoint]
// and the Clusterable interface for read optimization.
type DiscreteInput struct {
	Addr Address
	Qty  Quantity
}

// String returns a human-readable description of the discrete input range.
func (d DiscreteInput) String() string {
	return fmt.Sprintf("DiscreteInput(addr=%d, qty=%d)", d.Addr, d.Qty)
}

// ClusterKey returns a key that groups this data point with other discrete inputs for read merging.
func (d DiscreteInput) ClusterKey() string { return "discrete_input" }

// ClusterAddr returns the starting discrete input address for clustering.
func (d DiscreteInput) ClusterAddr() uint16 { return uint16(d.Addr) }

// ClusterQty returns the number of discrete inputs in this data point.
func (d DiscreteInput) ClusterQty() uint16 { return uint16(d.Qty) }

// ClusterBitsPerUnit returns 1, the bit width of a single discrete input.
func (d DiscreteInput) ClusterBitsPerUnit() uint16 { return 1 }

// ClusterMerge creates a new DiscreteInput that spans a merged address range,
// allowing the monitor to combine adjacent discrete input reads into a single request.
func (d DiscreteInput) ClusterMerge(start, count uint16) plc.DataPoint {
	return DiscreteInput{Addr: Address(start), Qty: Quantity(count)}
}

// ClusterExtract extracts this data point's bits from a larger clustered read
// result by computing the bit offset within the packed response.
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
