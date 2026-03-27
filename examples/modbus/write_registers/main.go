// Example: write_registers
//
// Demonstrates writing holding registers to a Modbus TCP server using both
// single-register writes (FC 0x06) and multiple-register writes (FC 0x10).
//
// Modbus provides two function codes for writing registers:
//
//   - FC 0x06 (Write Single Register) -- Writes a single 16-bit value to one
//     register. The response echoes back the address and value, confirming
//     the write. This is the simplest write operation.
//
//   - FC 0x10 (Write Multiple Registers) -- Writes a contiguous block of
//     16-bit values to multiple registers in a single request. The response
//     confirms the starting address and quantity written. This is more
//     efficient when updating several registers at once.
//
// This example also demonstrates the write-then-read-back pattern, which is
// common in industrial applications to verify that written values were
// accepted by the device.
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.6 and 6.12
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
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

	// -address: The target register address for the write operation.
	address := flag.Int("address", 0, "Target register address (0-65535)")

	// -value: A single 16-bit value for FC 0x06 (Write Single Register).
	// If this is set (and -values is not), we perform a single-register write.
	value := flag.Int("value", -1, "Single register value to write (0-65535); use -values for multiple")

	// -values: Comma-separated list of 16-bit values for FC 0x10 (Write
	// Multiple Registers). Example: "100,200,300"
	values := flag.String("values", "", "Comma-separated register values for multi-write (e.g., \"100,200,300\")")

	flag.Parse()

	// Validate that at least one write mode was specified.
	if *value < 0 && *values == "" {
		fmt.Fprintln(os.Stderr, "Error: specify -value for single register write or -values for multiple register write")
		fmt.Fprintln(os.Stderr, "  Example single:   -address 0 -value 1234")
		fmt.Fprintln(os.Stderr, "  Example multiple: -address 0 -values \"100,200,300\"")
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Parse the -values flag if provided
	// -----------------------------------------------------------------------
	var multiValues []modbus.RegisterValue
	if *values != "" {
		parts := strings.Split(*values, ",")
		multiValues = make([]modbus.RegisterValue, 0, len(parts))
		for i, part := range parts {
			part = strings.TrimSpace(part)
			v, err := strconv.ParseUint(part, 10, 16)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid value at position %d (%q): %v\n", i, part, err)
				fmt.Fprintln(os.Stderr, "  Values must be integers in the range 0-65535")
				os.Exit(1)
			}
			multiValues = append(multiValues, modbus.RegisterValue(v))
		}
		if len(multiValues) == 0 {
			fmt.Fprintln(os.Stderr, "Error: -values must contain at least one value")
			os.Exit(1)
		}
		if len(multiValues) > int(modbus.MaxWriteRegisterCount) {
			fmt.Fprintf(os.Stderr, "Error: cannot write more than %d registers in a single request\n", modbus.MaxWriteRegisterCount)
			os.Exit(1)
		}
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
	// Perform the write operation
	// -----------------------------------------------------------------------
	if *values != "" {
		// Write Multiple Registers (FC 0x10)
		writeMultipleRegisters(client, modbus.Address(*address), multiValues)
	} else {
		// Write Single Register (FC 0x06)
		writeSingleRegister(client, modbus.Address(*address), modbus.RegisterValue(*value))
	}

	fmt.Println()
	fmt.Println("Done.")
}

// writeSingleRegister demonstrates FC 0x06 (Write Single Register).
//
// The Modbus Write Single Register request contains:
//   - Function Code: 0x06
//   - Register Address: 2 bytes (big-endian)
//   - Register Value: 2 bytes (big-endian)
//
// The normal response is an echo of the request, confirming the write.
// This is useful for quick verification without a separate read-back.
func writeSingleRegister(client *modbus.Client, address modbus.Address, value modbus.RegisterValue) {
	fmt.Printf("--- Writing single register (FC 0x06) ---\n")
	fmt.Printf("  Address: %d\n", address)
	fmt.Printf("  Value:   %d (0x%04X)\n", value, value)
	fmt.Println()

	// Step 1: Read the current value so we can show what is changing.
	// This is optional but helpful for diagnostics.
	fmt.Println("  Reading current value before write...")
	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	before, err := client.ReadHoldingRegisters(readCtx, address, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current value: %v\n", err)
	} else {
		fmt.Printf("  Current value: %d (0x%04X)\n", before[0], before[0])
	}
	fmt.Println()

	// Step 2: Write the new value using FC 0x06.
	fmt.Printf("  Writing value %d to register %d...\n", value, address)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()

	err = client.WriteSingleRegister(writeCtx, address, value)
	if err != nil {
		handleError("WriteSingleRegister", err)
		return
	}
	fmt.Println("  Write successful.")
	fmt.Println()

	// Step 3: Read back the value to confirm it was written correctly.
	// This is the "write-then-read-back" pattern, commonly used in safety-
	// critical applications to ensure the device accepted the value.
	// Some devices may clamp or reject values outside their valid range,
	// so the read-back might differ from what was written.
	fmt.Println("  Reading back to verify...")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer verifyCancel()

	after, err := client.ReadHoldingRegisters(verifyCtx, address, 1)
	if err != nil {
		handleError("ReadHoldingRegisters (verify)", err)
		return
	}

	fmt.Printf("  Read-back value: %d (0x%04X)\n", after[0], after[0])
	if after[0] == value {
		fmt.Println("  Verification: PASS -- read-back matches written value.")
	} else {
		fmt.Printf("  Verification: MISMATCH -- wrote %d but read %d.\n", value, after[0])
		fmt.Println("  (The device may have clamped or rejected the value.)")
	}
}

// writeMultipleRegisters demonstrates FC 0x10 (Write Multiple Registers).
//
// The Modbus Write Multiple Registers request contains:
//   - Function Code: 0x10
//   - Starting Address: 2 bytes (big-endian)
//   - Quantity of Registers: 2 bytes (big-endian), 1-123
//   - Byte Count: 1 byte (= quantity * 2)
//   - Register Values: quantity * 2 bytes (big-endian, packed contiguously)
//
// The normal response contains only the starting address and quantity,
// confirming how many registers were written.
func writeMultipleRegisters(client *modbus.Client, address modbus.Address, values []modbus.RegisterValue) {
	qty := len(values)
	fmt.Printf("--- Writing %d registers (FC 0x10) ---\n", qty)
	fmt.Printf("  Starting address: %d\n", address)
	fmt.Printf("  Values: ")
	for i, v := range values {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%d", v)
	}
	fmt.Println()
	fmt.Println()

	// Step 1: Read the current values before writing.
	fmt.Println("  Reading current values before write...")
	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	before, err := client.ReadHoldingRegisters(readCtx, address, modbus.Quantity(qty))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current values: %v\n", err)
	} else {
		fmt.Printf("  %-10s %-15s\n", "Address", "Current Value")
		fmt.Printf("  %-10s %-15s\n", "-------", "-------------")
		for i, v := range before {
			fmt.Printf("  %-10d %-10d (0x%04X)\n", int(address)+i, v, v)
		}
	}
	fmt.Println()

	// Step 2: Write all values in a single FC 0x10 request.
	// This is atomic from the protocol perspective -- either all registers
	// are written or an exception is returned.
	fmt.Printf("  Writing %d registers starting at address %d...\n", qty, address)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()

	err = client.WriteMultipleRegisters(writeCtx, address, values)
	if err != nil {
		handleError("WriteMultipleRegisters", err)
		return
	}
	fmt.Println("  Write successful.")
	fmt.Println()

	// Step 3: Read back all registers to verify.
	fmt.Println("  Reading back to verify...")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer verifyCancel()

	after, err := client.ReadHoldingRegisters(verifyCtx, address, modbus.Quantity(qty))
	if err != nil {
		handleError("ReadHoldingRegisters (verify)", err)
		return
	}

	fmt.Printf("  %-10s %-15s %-15s %-10s\n", "Address", "Written", "Read Back", "Match")
	fmt.Printf("  %-10s %-15s %-15s %-10s\n", "-------", "-------", "---------", "-----")
	allMatch := true
	for i, v := range values {
		match := "OK"
		if after[i] != v {
			match = "MISMATCH"
			allMatch = false
		}
		fmt.Printf("  %-10d %-15d %-15d %-10s\n", int(address)+i, v, after[i], match)
	}
	fmt.Println()
	if allMatch {
		fmt.Println("  Verification: PASS -- all values match.")
	} else {
		fmt.Println("  Verification: FAIL -- some values do not match.")
	}
}

// handleError prints a human-readable error message with Modbus-specific
// context when applicable.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)
		if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
			fmt.Fprintf(os.Stderr, "  Hint: The target register address is not writable or does not exist.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionInvalidDataValue) {
			fmt.Fprintf(os.Stderr, "  Hint: The server rejected the value. Check the device's valid range.\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
