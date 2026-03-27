// Example: read_coils
//
// Demonstrates reading coils (FC 0x01) and discrete inputs (FC 0x02) from a
// Modbus TCP server.
//
// The Modbus data model defines four primary data tables. Two of them hold
// single-bit (boolean) values:
//
//   - Coils (FC 0x01) -- read/write single-bit values, typically representing
//     the state of output devices such as relays, solenoids, and indicator
//     lights. The term "coil" comes from the relay coils in early industrial
//     control systems.
//
//   - Discrete Inputs (FC 0x02) -- read-only single-bit values, typically
//     representing the state of input devices such as switches, proximity
//     sensors, and limit switches.
//
// On the wire, coil/discrete input values are packed as bits within bytes.
// The first coil requested corresponds to the least significant bit of the
// first data byte in the response. The Modbus specification allows reading
// up to 2000 coils in a single request.
//
// This example shows:
//   - Reading coils and displaying their boolean state
//   - Reading discrete inputs using the -discrete flag
//   - Displaying values in a bit-level format
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.1 and 6.2
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags
	// -----------------------------------------------------------------------

	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")
	port := flag.Int("port", modbus.DefaultTCPPort, "Modbus TCP port")
	unit := flag.Int("unit", 1, "Modbus unit ID (slave address, 0-247)")

	// -address: The starting coil/discrete input address. Modbus uses
	// zero-based addressing (0-65535).
	address := flag.Int("address", 0, "Starting coil/discrete input address (0-65535)")

	// -count: Number of coils or discrete inputs to read. The Modbus
	// specification limits this to 2000 per request (MaxCoilCount).
	count := flag.Int("count", 16, "Number of coils/discrete inputs to read (1-2000)")

	// -discrete: When set, reads discrete inputs (FC 0x02) instead of
	// coils (FC 0x01). Discrete inputs are read-only and represent
	// physical input states.
	discrete := flag.Bool("discrete", false, "Read discrete inputs (FC 0x02) instead of coils (FC 0x01)")

	flag.Parse()

	// Validate the count against the Modbus specification limit.
	if *count < 1 || *count > int(modbus.MaxCoilCount) {
		fmt.Fprintf(os.Stderr, "Error: count must be between 1 and %d\n", modbus.MaxCoilCount)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Connect to the Modbus TCP server
	// -----------------------------------------------------------------------
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Connecting to Modbus TCP server at %s:%d (unit ID %d)...\n", *addr, *port, *unit)

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),
		modbus.WithTimeout(5*time.Second),
		modbus.WithUnitID(modbus.UnitID(*unit)),
		modbus.WithRetries(2),
		modbus.WithRetryDelay(500*time.Millisecond),
		modbus.WithLogger(logger),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Connected successfully.")
	fmt.Println()

	// -----------------------------------------------------------------------
	// Read coils or discrete inputs
	// -----------------------------------------------------------------------
	if *discrete {
		readDiscreteInputs(client, modbus.Address(*address), modbus.Quantity(*count))
	} else {
		readCoils(client, modbus.Address(*address), modbus.Quantity(*count))
	}

	fmt.Println()
	fmt.Println("Done.")
}

// readCoils demonstrates FC 0x01 (Read Coils).
//
// The Modbus Read Coils request contains:
//   - Function Code: 0x01
//   - Starting Address: 2 bytes (big-endian)
//   - Quantity of Coils: 2 bytes (big-endian), 1-2000
//
// The response contains a byte count followed by the coil values packed as
// bits. The least significant bit of the first data byte corresponds to the
// coil at the starting address. If the number of coils is not a multiple of
// 8, the remaining bits in the last byte are padded with zeros.
func readCoils(client *modbus.Client, address modbus.Address, quantity modbus.Quantity) {
	fmt.Printf("--- Reading %d coils starting at address %d (FC 0x01) ---\n", quantity, address)
	fmt.Println()

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	coils, err := client.ReadCoils(readCtx, address, quantity)
	if err != nil {
		handleError("ReadCoils", err)
		return
	}

	displayBoolValues("Coil", address, coils)
}

// readDiscreteInputs demonstrates FC 0x02 (Read Discrete Inputs).
//
// The wire format is identical to Read Coils -- the only difference is the
// function code (0x02 instead of 0x01). Discrete inputs are read-only
// values that represent the state of physical input devices.
func readDiscreteInputs(client *modbus.Client, address modbus.Address, quantity modbus.Quantity) {
	fmt.Printf("--- Reading %d discrete inputs starting at address %d (FC 0x02) ---\n", quantity, address)
	fmt.Println()

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	inputs, err := client.ReadDiscreteInputs(readCtx, address, quantity)
	if err != nil {
		handleError("ReadDiscreteInputs", err)
		return
	}

	displayBoolValues("Input", address, inputs)
}

// displayBoolValues shows coil/discrete input values in multiple formats:
//   1. Individual address listing with ON/OFF labels
//   2. Bit-level view grouped by bytes (showing the wire representation)
//   3. Summary statistics
func displayBoolValues(label string, startAddress modbus.Address, values []bool) {
	// -----------------------------------------------------------------------
	// 1. Individual address listing
	// -----------------------------------------------------------------------
	fmt.Printf("  %-10s %-8s %-6s\n", "Address", "State", "Bit")
	fmt.Printf("  %-10s %-8s %-6s\n", "-------", "-----", "---")
	for i, v := range values {
		state := "OFF"
		bit := "0"
		if v {
			state = "ON"
			bit = "1"
		}
		fmt.Printf("  %-10d %-8s %-6s\n", int(startAddress)+i, state, bit)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// 2. Bit-level view
	// -----------------------------------------------------------------------
	// Show the coil values as they appear on the wire: packed into bytes
	// with the first coil in the LSB position. This matches the Modbus
	// protocol encoding and is useful for debugging wire captures.
	fmt.Println("  Bit-level view (wire format, LSB first within each byte):")
	fmt.Println()

	// Group into bytes of 8 bits.
	for byteIdx := 0; byteIdx*8 < len(values); byteIdx++ {
		startBit := byteIdx * 8
		endBit := startBit + 8
		if endBit > len(values) {
			endBit = len(values)
		}

		// Build the bit string for this byte (LSB = first coil in group).
		var bits strings.Builder
		var byteVal byte
		for bitIdx := startBit; bitIdx < endBit; bitIdx++ {
			if values[bitIdx] {
				byteVal |= 1 << uint(bitIdx-startBit)
			}
		}

		// Display bits from MSB to LSB for readability, but label them
		// with their actual coil addresses.
		for bitIdx := 7; bitIdx >= 0; bitIdx-- {
			if startBit+bitIdx < len(values) {
				if (byteVal>>uint(bitIdx))&1 == 1 {
					bits.WriteByte('1')
				} else {
					bits.WriteByte('0')
				}
			} else {
				// Padding bits (not part of the requested range).
				bits.WriteByte('.')
			}
		}

		addrEnd := int(startAddress) + endBit - 1
		fmt.Printf("  Byte %2d (addr %d-%d): %s = 0x%02X\n",
			byteIdx, int(startAddress)+startBit, addrEnd, bits.String(), byteVal)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// 3. Summary
	// -----------------------------------------------------------------------
	onCount := 0
	for _, v := range values {
		if v {
			onCount++
		}
	}
	fmt.Printf("  Summary: %d of %d %ss are ON\n", onCount, len(values), label)
}

// handleError prints a human-readable error message with Modbus-specific
// context when applicable.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)
		if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
			fmt.Fprintf(os.Stderr, "  Hint: The requested coil/input address range does not exist on this device.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionFunctionCodeNotSupported) {
			fmt.Fprintf(os.Stderr, "  Hint: This device does not support this function code.\n")
			fmt.Fprintf(os.Stderr, "  Some devices do not implement discrete inputs (FC 0x02).\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
