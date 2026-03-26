package client

import (
	"encoding/binary"
	"fmt"

	"github.com/iceisfun/goeip/pkg/cip"
)

// Read reads a single tag value from the PLC and returns it as type T.
// T must be a fixed-size type compatible with binary.Read (int32, float32, bool, etc.)
// or implement cip.Unmarshaler.
func Read[T any](c *Client, tagName string) (T, error) {
	var zero T
	data, err := c.ReadTag(tagName)
	if err != nil {
		return zero, err
	}
	if len(data) < 2 {
		return zero, fmt.Errorf("response too short to contain type code")
	}
	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return zero, fmt.Errorf("response too short for type header")
	}
	var result T
	if err := cip.Unmarshal(data[hdrLen:], &result); err != nil {
		return zero, err
	}
	return result, nil
}

// ReadSlice reads count elements of a tag and returns them as []T.
// T must be a fixed-size type compatible with binary.Read (int32, float32, bool, etc.).
func ReadSlice[T any](c *Client, tagName string, count uint16) ([]T, error) {
	data, err := c.ReadTagElements(tagName, count)
	if err != nil {
		return nil, err
	}
	if len(data) < 2 {
		return nil, fmt.Errorf("response too short to contain type code")
	}
	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return nil, fmt.Errorf("response too short for type header")
	}
	result := make([]T, count)
	if err := cip.Unmarshal(data[hdrLen:], &result); err != nil {
		return nil, err
	}
	return result, nil
}
