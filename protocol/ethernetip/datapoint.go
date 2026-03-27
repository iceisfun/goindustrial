package ethernetip

// Tag identifies a readable/writable tag on an EtherNet/IP controller. Tags
// are named data points in a PLC (e.g. "Motor_Speed") accessed via symbolic
// segment paths. Tag implements the plc.DataPoint interface.
type Tag struct {
	// Name is the symbolic tag name as configured in the PLC program
	// (e.g. "Motor_Speed", "Line1_Count").
	Name string

	// Elements is the number of array elements to read or write.
	// For scalar (non-array) tags, use 1. A zero value is treated as 1.
	Elements uint16
}

// String returns the tag name.
func (t Tag) String() string { return t.Name }
