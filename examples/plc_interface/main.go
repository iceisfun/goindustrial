// Example: plc_interface
//
// Demonstrates writing protocol-agnostic code using the plc.PLC interface.
// The plc.PLC interface is the common abstraction that both modbus.Client
// and ethernetip.Client implement:
//
//	type PLC interface {
//	    Reader   // Read(ctx, ...DataPoint) ([]Value, error)
//	    Writer   // Write(ctx, DataPoint, []byte) error
//	    Connect(ctx context.Context) error
//	    Disconnect(ctx context.Context) error
//	    IsConnected() bool
//	}
//
// This example accepts a -protocol flag ("modbus" or "ethernetip") to
// choose which client to create at startup. Once the client is created,
// all subsequent code uses the plc.PLC interface exclusively -- the same
// Read and Write calls work regardless of whether the underlying transport
// is Modbus TCP or EtherNet/IP CIP.
//
// The key insight is that while the plc.PLC interface (and plc.Reader /
// plc.Writer) is protocol-agnostic, the DataPoint values passed to Read()
// and Write() are protocol-specific:
//
//   - Modbus: modbus.HoldingRegister{Addr: 0, Qty: 1}
//   - EtherNet/IP: ethernetip.Tag{Name: "MyDINT", Elements: 1}
//
// This means your "business logic" code can be written once against plc.PLC,
// but the data point configuration comes from a protocol-aware layer (e.g.,
// configuration files, device profiles, or factory functions).
//
// Usage:
//
//	# Read/write using Modbus
//	go run . -protocol modbus -addr 127.0.0.1 -port 502 -register 0
//
//	# Read/write using EtherNet/IP
//	go run . -protocol ethernetip -addr 127.0.0.1:44818 -tag MyDINT
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// -------------------------------------------------------------------
	// Parse command-line flags
	// -------------------------------------------------------------------

	// -protocol: which industrial protocol to use. This is the only flag
	// that determines which concrete client type is instantiated. All other
	// code uses the plc.PLC interface.
	protocol := flag.String("protocol", "modbus", "Protocol to use: 'modbus' or 'ethernetip'")

	// -addr: server address. For Modbus this is the hostname/IP (port is
	// separate). For EtherNet/IP this includes the port (e.g., "host:44818").
	addr := flag.String("addr", "127.0.0.1", "Server address")

	// Modbus-specific flags
	port := flag.Int("port", modbus.DefaultTCPPort, "Modbus TCP port (only used with -protocol modbus)")
	register := flag.Int("register", 0, "Modbus holding register address (only used with -protocol modbus)")
	unit := flag.Int("unit", 1, "Modbus unit ID (only used with -protocol modbus)")

	// EtherNet/IP-specific flags
	tag := flag.String("tag", "MyDINT", "EtherNet/IP tag name (only used with -protocol ethernetip)")

	flag.Parse()

	// -------------------------------------------------------------------
	// Set up logging
	// -------------------------------------------------------------------
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Create the protocol-specific client and data point
	// -------------------------------------------------------------------
	// This is the ONLY place in the program where we use protocol-specific
	// types. After this block, everything operates through the plc.PLC
	// interface.
	//
	// In a real application, this factory logic might live in a
	// configuration loader that reads device profiles from a YAML/JSON file
	// and creates the appropriate client and data points.

	var (
		// device is the plc.PLC interface that we use for all operations.
		// The concrete type behind it is either *modbus.Client or
		// *ethernetip.Client, but the rest of the code doesn't know or care.
		device plc.PLC

		// readPoint is the data point we will read from.
		// This is protocol-specific (modbus.HoldingRegister or ethernetip.Tag)
		// but the plc.Read() call accepts any plc.DataPoint.
		readPoint plc.DataPoint

		// writePoint is the data point we will write to.
		writePoint plc.DataPoint

		// writeData is the raw bytes to write. The encoding depends on the
		// protocol: Modbus uses big-endian register values, while EtherNet/IP
		// uses little-endian CIP encoding with a type prefix.
		writeData []byte

		// protocolName is just for display purposes.
		protocolName string
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch *protocol {
	case "modbus":
		protocolName = "Modbus TCP"
		fmt.Printf("Creating %s client for %s:%d (unit ID %d)...\n",
			protocolName, *addr, *port, *unit)

		// Create a Modbus client using the convenience constructor.
		// This establishes the initial connection immediately.
		client, err := modbus.Connect(ctx, *addr,
			modbus.WithPort(*port),
			modbus.WithUnitID(modbus.UnitID(*unit)),
			modbus.WithLogger(logger),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect via Modbus: %v\n", err)
			os.Exit(1)
		}

		// Assign to the plc.PLC interface variable.
		// From this point on, we only use 'device' (the interface).
		device = client

		// Create Modbus-specific data points.
		// A HoldingRegister data point specifies the register address and
		// the number of 16-bit registers to read/write.
		readPoint = modbus.HoldingRegister{Addr: modbus.Address(*register), Qty: 1}
		writePoint = modbus.HoldingRegister{Addr: modbus.Address(*register), Qty: 1}

		// Modbus write data: a single 16-bit register value in big-endian.
		// We'll write the value 42 (0x002A).
		writeData = make([]byte, 2)
		binary.BigEndian.PutUint16(writeData, 42)

	case "ethernetip":
		protocolName = "EtherNet/IP"
		fmt.Printf("Creating %s client for %s...\n", protocolName, *addr)

		// Create an EtherNet/IP client using the convenience constructor.
		// This establishes the initial connection immediately.
		client, err := ethernetip.Connect(ctx, *addr,
			ethernetip.WithLogger(logger),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect via EtherNet/IP: %v\n", err)
			os.Exit(1)
		}

		// Assign to the plc.PLC interface variable.
		device = client

		// Create EtherNet/IP-specific data points.
		// A Tag data point specifies the tag name and element count.
		readPoint = ethernetip.Tag{Name: *tag, Elements: 1}
		writePoint = ethernetip.Tag{Name: *tag, Elements: 1}

		// EtherNet/IP write data: type code prefix (2 bytes) + value.
		// For a DINT (int32), we write: [type=0x00C4] [value=42 as int32]
		// The type code is required by the plc.Write() implementation for
		// EtherNet/IP (see ethernetip.Client.Write).
		writeData = make([]byte, 6)
		binary.LittleEndian.PutUint16(writeData[0:2], uint16(cip.TypeDINT))
		binary.LittleEndian.PutUint32(writeData[2:6], 42)

	default:
		fmt.Fprintf(os.Stderr, "Unknown protocol %q. Use 'modbus' or 'ethernetip'.\n", *protocol)
		os.Exit(1)
	}

	fmt.Printf("Connected via %s.\n", protocolName)
	fmt.Println()

	// -------------------------------------------------------------------
	// From here on, ALL code is protocol-agnostic.
	// We only use the plc.PLC interface.
	// -------------------------------------------------------------------

	// Ensure we disconnect cleanly when done.
	defer func() {
		fmt.Println("Disconnecting...")
		if err := device.Disconnect(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error disconnecting: %v\n", err)
		}
	}()

	// Step 1: Check connection status.
	// IsConnected() is a simple boolean check that does not perform I/O.
	fmt.Printf("Connection status: connected=%v\n", device.IsConnected())
	fmt.Println()

	// Step 2: Read the current value.
	// device.Read() takes one or more DataPoints and returns Values.
	// The DataPoint is protocol-specific (created above), but the Read()
	// call is identical regardless of protocol.
	fmt.Println("--- Step 1: Read current value ---")
	readFromDevice(ctx, device, readPoint)
	fmt.Println()

	// Step 3: Write a new value.
	// device.Write() takes a DataPoint and raw bytes. The byte encoding
	// depends on the protocol (big-endian for Modbus, CIP-encoded for EIP).
	fmt.Println("--- Step 2: Write new value (42) ---")
	writeToDevice(ctx, device, writePoint, writeData)
	fmt.Println()

	// Step 4: Read back the value to confirm the write.
	fmt.Println("--- Step 3: Read back to confirm write ---")
	readFromDevice(ctx, device, readPoint)
	fmt.Println()

	// Step 5: Demonstrate reading multiple data points in one call.
	// plc.Read() accepts variadic DataPoints, so you can batch reads.
	// Each protocol handles this differently internally:
	//   - Modbus: issues separate function code requests per data point
	//   - EtherNet/IP: issues separate ReadTag requests per data point
	// But from the caller's perspective, it is a single call.
	fmt.Println("--- Step 4: Batch read (same point twice for demo) ---")
	batchRead(ctx, device, readPoint)
	fmt.Println()

	fmt.Println("Done.")
}

// ---------------------------------------------------------------------------
// Protocol-agnostic functions that operate on plc.PLC / plc.Reader / plc.Writer
// ---------------------------------------------------------------------------

// readFromDevice reads a single data point from the PLC and prints the result.
// This function accepts a plc.PLC (or plc.Reader) and has zero knowledge of
// the underlying protocol.
func readFromDevice(ctx context.Context, device plc.PLC, point plc.DataPoint) {
	fmt.Printf("  Reading %s...\n", point.String())

	// Read returns a slice of Values, one per data point requested.
	values, err := device.Read(ctx, point)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Read error: %v\n", err)
		return
	}

	// Display each value.
	for _, val := range values {
		fmt.Printf("  %s = 0x%X (%d bytes)\n",
			val.DataPoint.String(), val.Raw, len(val.Raw))

		// Try to interpret the raw bytes as common types.
		interpretValue(val)
	}
}

// writeToDevice writes raw bytes to a data point on the PLC.
// This function accepts a plc.PLC (or plc.Writer) and has zero knowledge of
// the underlying protocol.
func writeToDevice(ctx context.Context, device plc.PLC, point plc.DataPoint, data []byte) {
	fmt.Printf("  Writing %d bytes to %s...\n", len(data), point.String())

	err := device.Write(ctx, point, data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Write error: %v\n", err)
		return
	}

	fmt.Println("  Write successful.")
}

// batchRead demonstrates reading multiple data points in a single call.
func batchRead(ctx context.Context, device plc.PLC, points ...plc.DataPoint) {
	fmt.Printf("  Batch reading %d point(s)...\n", len(points))

	values, err := device.Read(ctx, points...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Batch read error: %v\n", err)
		return
	}

	for i, val := range values {
		fmt.Printf("  [%d] %s = 0x%X (%d bytes)\n",
			i, val.DataPoint.String(), val.Raw, len(val.Raw))
	}
}

// interpretValue attempts to display the raw value in human-readable formats.
// This is a simple heuristic based on the byte length -- in a real application,
// you would know the expected type from your device configuration.
func interpretValue(val plc.Value) {
	switch len(val.Raw) {
	case 1:
		// Could be a BOOL, USINT, or SINT
		fmt.Printf("    -> as uint8:  %d\n", val.Raw[0])
		fmt.Printf("    -> as bool:   %v\n", val.Raw[0] != 0)

	case 2:
		// Could be a Modbus register (big-endian) or CIP INT (little-endian)
		beval := binary.BigEndian.Uint16(val.Raw)
		leval := binary.LittleEndian.Uint16(val.Raw)
		fmt.Printf("    -> as uint16 (big-endian):    %d\n", beval)
		fmt.Printf("    -> as uint16 (little-endian): %d\n", leval)

	case 4:
		// Could be a DINT (int32) or REAL (float32)
		beval := binary.BigEndian.Uint32(val.Raw)
		leval := binary.LittleEndian.Uint32(val.Raw)
		fmt.Printf("    -> as uint32 (big-endian):    %d\n", beval)
		fmt.Printf("    -> as uint32 (little-endian): %d\n", leval)
		fmt.Printf("    -> as int32  (little-endian): %d\n", int32(leval))

	default:
		// For EtherNet/IP, the response includes a 2-byte type prefix.
		// If we have 6+ bytes, the first 2 might be the type code.
		if len(val.Raw) >= 6 {
			typeCode := binary.LittleEndian.Uint16(val.Raw[0:2])
			fmt.Printf("    -> possible CIP type code: 0x%04X\n", typeCode)
			fmt.Printf("    -> value bytes: 0x%X\n", val.Raw[2:])
		}
	}
}
