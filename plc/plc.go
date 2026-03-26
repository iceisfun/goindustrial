package plc

import "context"

// DataPoint identifies a readable/writable location on a controller.
// Protocol-specific implementations encode their native addressing:
//   - Modbus: area (coil/register), address, quantity
//   - EtherNet/IP: tag name, element count
type DataPoint interface {
	// String returns a human-readable representation of the data point.
	String() string
}

// Value holds the result of reading a data point.
type Value struct {
	DataPoint DataPoint
	Raw       []byte
}

// Reader can read data points from a controller.
type Reader interface {
	Read(ctx context.Context, points ...DataPoint) ([]Value, error)
}

// Writer can write data points to a controller.
type Writer interface {
	Write(ctx context.Context, point DataPoint, data []byte) error
}

// PLC represents a connection to an industrial controller.
// For protocol-specific features (e.g., Modbus ReadCoils, EtherNet/IP ListTags),
// use the concrete protocol client types directly.
type PLC interface {
	Reader
	Writer
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	IsConnected() bool
}
