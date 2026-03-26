package ethernetip

// Tag identifies a readable/writable tag on an EtherNet/IP controller.
// It implements the plc.DataPoint interface.
type Tag struct {
	Name     string
	Elements uint16
}

// String returns the tag name.
func (t Tag) String() string { return t.Name }
