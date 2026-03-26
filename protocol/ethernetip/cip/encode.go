package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Marshaler is the interface implemented by types that can marshal
// themselves into a CIP binary description.
type Marshaler interface {
	MarshalCIP() ([]byte, error)
}

// Marshal returns the CIP encoding of v.
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

// GoTypeToCIPType maps a Go type to a CIP Data Type.
func GoTypeToCIPType(v any) (DataType, error) {
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
