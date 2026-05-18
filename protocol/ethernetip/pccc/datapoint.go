package pccc

import (
	"fmt"
	"strconv"
)

// File is a [plc.DataPoint] that names a PCCC data-table location by its
// textual SLC address (e.g. "N7:0", "B3:0/2", "T4:0.ACC"). It is the
// pccc-package counterpart to [modbus.HoldingRegister] and [ethernetip.Tag].
//
// Count selects how many consecutive elements to read or write starting at
// the addressed element. A value of 0 is treated as 1. Count > 1 is
// meaningful only for word-sized files (N, B, F, S, I, O, D); bit suffixes
// and timer/counter sub-fields are scalar by definition and ignore Count.
type File struct {
	Address string
	Count   int
}

// String returns a human-readable description of the data point in the
// canonical form "File(address)" or "File(address, count=N)".
func (f File) String() string {
	if f.Count > 1 {
		return "File(" + f.Address + ", count=" + strconv.Itoa(f.Count) + ")"
	}
	if f.Address == "" {
		return "File()"
	}
	return "File(" + f.Address + ")"
}

// effectiveCount returns Count clamped to a minimum of 1.
func (f File) effectiveCount() int {
	if f.Count <= 0 {
		return 1
	}
	return f.Count
}

// parse parses File.Address with [ParseAddress] and reports a wrapped error
// on failure.
func (f File) parse() (Address, error) {
	a, err := ParseAddress(f.Address)
	if err != nil {
		return Address{}, fmt.Errorf("pccc: %s: %w", f.String(), err)
	}
	return a, nil
}
