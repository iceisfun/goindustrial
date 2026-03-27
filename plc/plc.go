// Package plc defines protocol-agnostic abstractions for communicating with
// industrial programmable logic controllers (PLCs). It provides the [PLC]
// interface for reading and writing data points, along with a [Value] type
// that carries raw bytes with type and byte-order metadata so callers can
// interpret responses without depending on a specific protocol.
//
// Protocol implementations (Modbus TCP, EtherNet/IP CIP) satisfy these
// interfaces while providing their own concrete [DataPoint] types that encode
// protocol-native addressing such as register numbers or tag names.
package plc

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

// DataType is a protocol-agnostic hint describing the kind of data in a Value.
// Protocol implementations set this when returning Values from Read, so
// consumers can interpret Raw bytes without type-switching on the DataPoint.
type DataType uint8

const (
	// TypeUnknown indicates no type information is available.
	TypeUnknown DataType = iota

	// TypeBool indicates a boolean value (coil, discrete input, BOOL tag).
	TypeBool

	// TypeInt16 indicates a signed 16-bit integer.
	TypeInt16

	// TypeUint16 indicates an unsigned 16-bit integer (Modbus register).
	TypeUint16

	// TypeInt32 indicates a signed 32-bit integer (e.g., CIP DINT).
	TypeInt32

	// TypeUint32 indicates an unsigned 32-bit integer (e.g., CIP UDINT).
	TypeUint32

	// TypeInt64 indicates a signed 64-bit integer (e.g., CIP LINT).
	TypeInt64

	// TypeUint64 indicates an unsigned 64-bit integer (e.g., CIP ULINT).
	TypeUint64

	// TypeFloat32 indicates a 32-bit float (e.g., CIP REAL).
	TypeFloat32

	// TypeFloat64 indicates a 64-bit float (e.g., CIP LREAL).
	TypeFloat64

	// TypeString indicates a string value.
	TypeString

	// TypeBytes indicates an opaque byte sequence (multi-register reads,
	// structs, or any type the protocol layer cannot further classify).
	TypeBytes
)

// String returns the name of the data type.
func (t DataType) String() string {
	switch t {
	case TypeUnknown:
		return "unknown"
	case TypeBool:
		return "bool"
	case TypeInt16:
		return "int16"
	case TypeUint16:
		return "uint16"
	case TypeInt32:
		return "int32"
	case TypeUint32:
		return "uint32"
	case TypeInt64:
		return "int64"
	case TypeUint64:
		return "uint64"
	case TypeFloat32:
		return "float32"
	case TypeFloat64:
		return "float64"
	case TypeString:
		return "string"
	case TypeBytes:
		return "bytes"
	default:
		return fmt.Sprintf("DataType(%d)", t)
	}
}

// ByteOrder describes the byte order of the raw data in a Value.
type ByteOrder uint8

const (
	// ByteOrderUnspecified means the byte order was not determined.
	ByteOrderUnspecified ByteOrder = iota

	// ByteOrderBigEndian indicates big-endian byte order (Modbus).
	ByteOrderBigEndian

	// ByteOrderLittleEndian indicates little-endian byte order (EtherNet/IP CIP).
	ByteOrderLittleEndian
)

// String returns the name of the byte order.
func (o ByteOrder) String() string {
	switch o {
	case ByteOrderBigEndian:
		return "big-endian"
	case ByteOrderLittleEndian:
		return "little-endian"
	default:
		return "unspecified"
	}
}

// DataPoint identifies a readable/writable location on a controller.
// Protocol-specific implementations encode their native addressing:
//   - Modbus: area (coil/register), address, quantity
//   - EtherNet/IP: tag name, element count
type DataPoint interface {
	// String returns a human-readable representation of the data point.
	String() string
}

// TypedDataPoint is an optional extension of DataPoint that carries
// type metadata. Protocol implementations may implement this interface
// on their DataPoint types to provide type hints without requiring
// the consumer to type-switch on the concrete type.
//
// Implementations that do not have type information at the data-point
// level (e.g., Modbus registers that can hold any 16-bit value) should
// not implement this interface; in that case the type information on the
// returned Value is the authoritative source.
type TypedDataPoint interface {
	DataPoint

	// DataType returns the expected data type for this data point.
	DataType() DataType
}

// Value holds the result of reading a data point.
type Value struct {
	// DataPoint is the data point that was read.
	DataPoint DataPoint

	// Raw is the raw bytes returned by the protocol layer.
	Raw []byte

	// Type is a protocol-agnostic hint for the data type of Raw.
	// Set by the protocol implementation during Read. May be TypeUnknown
	// if the protocol layer cannot determine the type.
	Type DataType

	// ByteOrder indicates the byte order of Raw. This tells consumers
	// how to decode multi-byte values without knowing the protocol.
	ByteOrder ByteOrder
}

// Bool interprets the value as a boolean. Returns false if Raw is empty.
// For multi-byte values, returns true if any byte is nonzero.
func (v Value) Bool() bool {
	for _, b := range v.Raw {
		if b != 0 {
			return true
		}
	}
	return false
}

// Int interprets the value as a signed 64-bit integer using the Value's
// ByteOrder. Supports 1, 2, 4, and 8 byte values. Returns 0 and an error
// for unsupported sizes or if ByteOrder is unspecified for multi-byte values.
func (v Value) Int() (int64, error) {
	order := v.byteOrder()
	switch len(v.Raw) {
	case 1:
		return int64(int8(v.Raw[0])), nil
	case 2:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return int64(int16(order.Uint16(v.Raw))), nil
	case 4:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return int64(int32(order.Uint32(v.Raw))), nil
	case 8:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return int64(order.Uint64(v.Raw)), nil
	default:
		return 0, fmt.Errorf("plc: cannot interpret %d bytes as integer", len(v.Raw))
	}
}

// Uint interprets the value as an unsigned 64-bit integer using the Value's
// ByteOrder. Supports 1, 2, 4, and 8 byte values.
func (v Value) Uint() (uint64, error) {
	order := v.byteOrder()
	switch len(v.Raw) {
	case 1:
		return uint64(v.Raw[0]), nil
	case 2:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return uint64(order.Uint16(v.Raw)), nil
	case 4:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return uint64(order.Uint32(v.Raw)), nil
	case 8:
		if order == nil {
			return 0, fmt.Errorf("plc: byte order required for %d-byte value", len(v.Raw))
		}
		return uint64(order.Uint64(v.Raw)), nil
	default:
		return 0, fmt.Errorf("plc: cannot interpret %d bytes as unsigned integer", len(v.Raw))
	}
}

// Float32 interprets the value as a 32-bit float using the Value's ByteOrder.
func (v Value) Float32() (float32, error) {
	if len(v.Raw) != 4 {
		return 0, fmt.Errorf("plc: expected 4 bytes for float32, got %d", len(v.Raw))
	}
	order := v.byteOrder()
	if order == nil {
		return 0, fmt.Errorf("plc: byte order required for float32")
	}
	return math.Float32frombits(order.Uint32(v.Raw)), nil
}

// Float64 interprets the value as a 64-bit float using the Value's ByteOrder.
func (v Value) Float64() (float64, error) {
	if len(v.Raw) != 8 {
		return 0, fmt.Errorf("plc: expected 8 bytes for float64, got %d", len(v.Raw))
	}
	order := v.byteOrder()
	if order == nil {
		return 0, fmt.Errorf("plc: byte order required for float64")
	}
	return math.Float64frombits(order.Uint64(v.Raw)), nil
}

func (v Value) byteOrder() binary.ByteOrder {
	switch v.ByteOrder {
	case ByteOrderBigEndian:
		return binary.BigEndian
	case ByteOrderLittleEndian:
		return binary.LittleEndian
	default:
		return nil
	}
}

// Reader reads data points from a controller. Implementations perform one or
// more protocol transactions and return a Value for each requested DataPoint
// in the same order.
type Reader interface {
	// Read retrieves the current values of the given data points from the
	// controller. The returned slice has one Value per input DataPoint, in
	// the same order. The context may be used for cancellation or deadlines.
	Read(ctx context.Context, points ...DataPoint) ([]Value, error)
}

// Writer writes data to a controller. Implementations encode the raw bytes
// according to the protocol and address specified by the DataPoint.
type Writer interface {
	// Write sends data to the specified data point on the controller.
	// The caller is responsible for encoding data in the byte order
	// expected by the target protocol.
	Write(ctx context.Context, point DataPoint, data []byte) error
}

// PLC represents a connection to an industrial controller that can be opened,
// closed, and used for reading and writing data points. It embeds [Reader]
// and [Writer] for data access.
//
// For protocol-specific features (e.g., Modbus ReadCoils, EtherNet/IP
// ListTags), use the concrete protocol client types directly.
type PLC interface {
	Reader
	Writer

	// Connect establishes the underlying protocol session with the controller.
	Connect(ctx context.Context) error

	// Disconnect gracefully tears down the protocol session.
	Disconnect(ctx context.Context) error

	// IsConnected reports whether the connection is currently active.
	IsConnected() bool
}
