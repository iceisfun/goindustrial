// Example: read_registers
//
// Demonstrates reading holding registers (FC 0x03) and input registers (FC 0x04)
// from a Modbus TCP server.
//
// Modbus defines four primary data tables. Two of them hold 16-bit register
// values:
//
//   - Holding Registers (FC 0x03) -- read/write, typically used for
//     configuration, setpoints, and control values. These are the most
//     commonly used registers in Modbus devices.
//
//   - Input Registers (FC 0x04) -- read-only, typically used for measured
//     values such as temperatures, pressures, and other sensor readings.
//
// Each register is a 16-bit (2-byte) unsigned integer, stored big-endian on
// the wire. The Modbus specification allows reading up to 125 registers in a
// single request (limited by the 253-byte PDU size).
//
// This example shows:
//   - Reading a single holding register
//   - Reading multiple holding registers in one request
//   - Reading input registers
//   - Displaying values in both decimal and hexadecimal
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.3 and 6.4
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags
	// -----------------------------------------------------------------------

	// -addr: IP address or hostname of the Modbus TCP server.
	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")

	// -port: TCP port. The default Modbus TCP port is 502, which typically
	// requires root/administrator privileges. Many simulators use 5020.
	port := flag.Int("port", modbus.DefaultTCPPort, "Modbus TCP port")

	// -unit: The Modbus Unit ID (also called slave address). In Modbus TCP
	// the unit ID identifies a specific device behind a gateway. For a
	// standalone device this is typically 0 or 1.
	unit := flag.Int("unit", 1, "Modbus unit ID (slave address, 0-247)")

	// -address: The starting register address. Modbus uses zero-based
	// addressing internally (0-65535), even though some device documentation
	// uses 1-based "Modbus addresses" like 40001 for the first holding
	// register.
	address := flag.Int("address", 0, "Starting register address (0-65535)")

	// -count: Number of registers to read. The Modbus specification limits
	// this to 125 registers per request (MaxRegisterCount).
	count := flag.Int("count", 10, "Number of registers to read (1-125)")

	flag.Parse()

	// Validate the register count against the Modbus specification limit.
	if *count < 1 || *count > int(modbus.MaxRegisterCount) {
		fmt.Fprintf(os.Stderr, "Error: count must be between 1 and %d\n", modbus.MaxRegisterCount)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Set up a logger
	// -----------------------------------------------------------------------
	// The logger is used by the Modbus client to report connection events,
	// retries, and protocol-level details. For a CLI tool we use Info level;
	// set to LevelDebug or LevelTrace for wire-level diagnostics.
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -----------------------------------------------------------------------
	// Connect to the Modbus TCP server
	// -----------------------------------------------------------------------
	// modbus.Connect is the convenience constructor. It:
	//   1. Creates a TCP connection with MBAP framing
	//   2. Wraps it in a ReconnectingTransport for automatic reconnection
	//   3. Verifies the initial connection is reachable
	//
	// Options are passed as variadic any values and are automatically sorted
	// into TCPConnOption, ClientOption, and transport.Option buckets.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Connecting to Modbus TCP server at %s:%d (unit ID %d)...\n", *addr, *port, *unit)

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),                  // TCPConnOption: set the TCP port
		modbus.WithTimeout(5*time.Second),        // TCPConnOption: connection timeout
		modbus.WithUnitID(modbus.UnitID(*unit)),  // ClientOption: set the unit/slave ID
		modbus.WithRetries(2),                    // ClientOption: retry up to 2 times on transport errors
		modbus.WithRetryDelay(500*time.Millisecond), // ClientOption: wait 500ms between retries
		modbus.WithLogger(logger),                // ClientOption: attach our logger
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		// Common causes:
		//   - "connection refused": no server listening on that host:port
		//   - "i/o timeout": server unreachable (wrong IP, firewall, etc.)
		//   - "context deadline exceeded": our 10s timeout was hit
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Connected successfully.")
	fmt.Println()

	// -----------------------------------------------------------------------
	// 1. Read a single holding register (FC 0x03)
	// -----------------------------------------------------------------------
	// Reading a single register is the simplest operation. We request
	// quantity=1 at the given address. The server responds with 2 bytes
	// (one 16-bit register value).
	fmt.Printf("--- Reading single holding register at address %d ---\n", *address)

	singleCtx, singleCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer singleCancel()

	registers, err := client.ReadHoldingRegisters(singleCtx, modbus.Address(*address), 1)
	if err != nil {
		handleError("ReadHoldingRegisters (single)", err)
	} else {
		fmt.Printf("  Address %d = %d (0x%04X)\n", *address, registers[0], registers[0])
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// 2. Read multiple holding registers (FC 0x03)
	// -----------------------------------------------------------------------
	// Reading multiple registers in a single request is more efficient than
	// issuing individual reads. The Modbus protocol packs them contiguously:
	// the response contains count*2 bytes of register data.
	//
	// This is the most common pattern in real-world SCADA/HMI applications
	// where you read a block of registers and parse them into meaningful
	// process values (temperatures, pressures, counts, etc.).
	fmt.Printf("--- Reading %d holding registers starting at address %d ---\n", *count, *address)

	multiCtx, multiCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer multiCancel()

	registers, err = client.ReadHoldingRegisters(multiCtx, modbus.Address(*address), modbus.Quantity(*count))
	if err != nil {
		handleError("ReadHoldingRegisters (multiple)", err)
	} else {
		// Display each register with its address, decimal value, and hex value.
		// In practice you would interpret these values according to the device's
		// register map (e.g., register 0 might be a temperature in tenths of
		// a degree, register 1 might be a status word, etc.).
		fmt.Printf("  %-10s %-10s %-10s\n", "Address", "Decimal", "Hex")
		fmt.Printf("  %-10s %-10s %-10s\n", "-------", "-------", "------")
		for i, val := range registers {
			regAddr := *address + i
			fmt.Printf("  %-10d %-10d 0x%04X\n", regAddr, val, val)
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// 3. Read input registers (FC 0x04)
	// -----------------------------------------------------------------------
	// Input registers are read-only and typically represent measured or
	// computed values from the device (sensor readings, counters, etc.).
	// The wire protocol is identical to holding registers -- the only
	// difference is the function code (0x04 vs 0x03).
	//
	// Note: Not all devices implement input registers. Some devices map
	// everything into the holding register space. If the device does not
	// support FC 0x04, you will get a Modbus exception response with
	// exception code 0x01 (Function Code Not Supported).
	fmt.Printf("--- Reading %d input registers starting at address %d ---\n", *count, *address)

	inputCtx, inputCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer inputCancel()

	inputRegisters, err := client.ReadInputRegisters(inputCtx, modbus.Address(*address), modbus.Quantity(*count))
	if err != nil {
		handleError("ReadInputRegisters", err)
	} else {
		fmt.Printf("  %-10s %-10s %-10s\n", "Address", "Decimal", "Hex")
		fmt.Printf("  %-10s %-10s %-10s\n", "-------", "-------", "------")
		for i, val := range inputRegisters {
			regAddr := *address + i
			fmt.Printf("  %-10d %-10d 0x%04X\n", regAddr, val, val)
		}
	}
	fmt.Println()

	fmt.Println("Done.")
}

// handleError prints a human-readable error message. If the error is a Modbus
// protocol exception (as opposed to a transport error), it provides additional
// context about what the exception code means.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		// The server responded with a Modbus exception. This is a protocol-
		// level error, not a connectivity issue. Common exceptions:
		//
		//   0x01 - Function Code Not Supported: the device does not implement
		//          the requested function code.
		//   0x02 - Data Address Not Available: the requested register address
		//          is out of the device's valid range.
		//   0x03 - Invalid Data Value: the request contained an invalid
		//          quantity or other parameter.
		//   0x04 - Server Device Failure: an unrecoverable error occurred
		//          while the server was processing the request.
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)

		if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
			fmt.Fprintf(os.Stderr, "  Hint: The requested address range is not available on this device.\n")
			fmt.Fprintf(os.Stderr, "  Check the device's register map documentation.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionFunctionCodeNotSupported) {
			fmt.Fprintf(os.Stderr, "  Hint: This device does not support this function code.\n")
		}
	} else {
		// Transport-level error (connection lost, timeout, etc.).
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
