package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Marshaler is the interface implemented by types that can marshal themselves
// into CIP binary format. Types that implement Marshaler take priority over
// the default binary.Write encoding in [Marshal].
type Marshaler interface {
	MarshalCIP() ([]byte, error)
}

// Marshal returns the little-endian CIP encoding of v. If v implements
// [Marshaler] its MarshalCIP method is called; otherwise binary.Write is used.
func Marshal(v any) ([]byte, error) {
	// 1. Check if v implements Marshaler
	if m, ok := v.(Marshaler); ok {
		return m.MarshalCIP()
	}

	// 2. Handle basic types and structs using binary.Write
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
		return nil, fmt.Errorf("cip: binary.Write failed: %w", err)
	}

	return buf.Bytes(), nil
}

// GoTypeToCIPType maps a Go value's type to the corresponding CIP [DataType]
// code. It supports bool, all signed/unsigned integer sizes, float32, float64,
// string, and any type implementing [TypeCodec].
func GoTypeToCIPType(v any) (DataType, error) {
	// Check TypeCodec first so custom struct types are handled.
	if tc, ok := v.(TypeCodec); ok {
		return tc.CIPType(), nil
	}
	switch v.(type) {
	case bool:
		return TypeBOOL, nil
	case int8:
		return TypeSINT, nil
	case int16:
		return TypeINT, nil
	case int32:
		return TypeDINT, nil
	case int64:
		return TypeLINT, nil
	case uint8:
		return TypeUSINT, nil // or BYTE? USINT is usually preferred for numbers
	case uint16:
		return TypeUINT, nil // or WORD?
	case uint32:
		return TypeUDINT, nil
	case uint64:
		return TypeULINT, nil // or LWORD?
	case float32:
		return TypeREAL, nil
	case float64:
		return TypeLREAL, nil
	case string:
		return TypeSTRING, nil // Default to standard STRING for now
	default:
		return 0, fmt.Errorf("cip: unsupported Go type for automatic mapping: %T", v)
	}
}
