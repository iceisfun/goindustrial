package cip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
)

// Unmarshaler is the interface implemented by types that can unmarshal
// a CIP binary description of themselves.
type Unmarshaler interface {
	UnmarshalCIP(data []byte) error
}

// Unmarshal parses the CIP-encoded data and stores the result
// in the value pointed to by v. If v is nil or not a pointer,
// Unmarshal returns an error.
func Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("cip: Unmarshal(non-pointer %T)", v)
	}

	// 1. Check if v implements Unmarshaler
	if u, ok := v.(Unmarshaler); ok {
		return u.UnmarshalCIP(data)
	}

	// 2. Handle basic types and structs using binary.Read
	// binary.Read handles:
	// - bool, int8, uint8, int16, uint16, int32, uint32, int64, uint64, float32, float64, complex64, complex128
	// - Arrays of the above
	// - Structs containing only the above (recursively)
	// It uses LittleEndian by default for CIP? Yes, CIP is Little Endian.

	// We need a reader
	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.LittleEndian, v); err != nil {
		return fmt.Errorf("cip: binary.Read failed: %w", err)
	}

	return nil
}
