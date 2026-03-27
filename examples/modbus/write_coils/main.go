// Example: write_coils
//
// Demonstrates writing coils to a Modbus TCP server using both single-coil
// writes (FC 0x05) and multiple-coil writes (FC 0x0F).
//
// Modbus provides two function codes for writing coils:
//
//   - FC 0x05 (Write Single Coil) -- Writes a single boolean value to one
//     coil address. The coil value is encoded as 0xFF00 for ON and 0x0000
//     for OFF on the wire. The response echoes back the request.
//
//   - FC 0x0F (Write Multiple Coils) -- Writes a contiguous block of boolean
//     values to multiple coils in a single request. Values are packed as bits
//     within bytes, with the first coil in the LSB of the first byte.
//
// This example also demonstrates the write-then-read-back pattern to verify
// that coil values were accepted by the device.
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Sections 6.5 and 6.11
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

	// -address: The target coil address for the write operation.
	address := flag.Int("address", 0, "Target coil address (0-65535)")

	// -value: A single coil value for FC 0x05 (Write Single Coil).
	// Accepts "on"/"off", "true"/"false", or "1"/"0".
	value := flag.String("value", "", "Single coil value: \"on\" or \"off\" (also accepts \"1\"/\"0\", \"true\"/\"false\")")

	// -values: Comma-separated list of bit values for FC 0x0F (Write
	// Multiple Coils). Example: "1,0,1,1,0"
	// Each value represents one coil: 1=ON, 0=OFF.
	values := flag.String("values", "", "Comma-separated coil values for multi-write (e.g., \"1,0,1,1,0\")")

	flag.Parse()

	// Validate that at least one write mode was specified.
	if *value == "" && *values == "" {
		fmt.Fprintln(os.Stderr, "Error: specify -value for single coil write or -values for multiple coil write")
		fmt.Fprintln(os.Stderr, "  Example single:   -address 0 -value on")
		fmt.Fprintln(os.Stderr, "  Example multiple: -address 0 -values \"1,0,1,1,0\"")
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Parse the single coil value
	// -----------------------------------------------------------------------
	var singleValue bool
	var useSingle bool
	if *value != "" {
		useSingle = true
		switch strings.ToLower(strings.TrimSpace(*value)) {
		case "on", "true", "1":
			singleValue = true
		case "off", "false", "0":
			singleValue = false
		default:
			fmt.Fprintf(os.Stderr, "Error: invalid coil value %q. Use \"on\"/\"off\", \"true\"/\"false\", or \"1\"/\"0\"\n", *value)
			os.Exit(1)
		}
	}

	// -----------------------------------------------------------------------
	// Parse the multiple coil values
	// -----------------------------------------------------------------------
	var multiValues []modbus.CoilValue
	if *values != "" {
		useSingle = false
		parts := strings.Split(*values, ",")
		multiValues = make([]modbus.CoilValue, 0, len(parts))
		for i, part := range parts {
			part = strings.TrimSpace(part)
			switch strings.ToLower(part) {
			case "1", "on", "true":
				multiValues = append(multiValues, true)
			case "0", "off", "false":
				multiValues = append(multiValues, false)
			default:
				fmt.Fprintf(os.Stderr, "Error: invalid coil value at position %d (%q). Use 1/0, on/off, or true/false\n", i, part)
				os.Exit(1)
			}
		}
		if len(multiValues) == 0 {
			fmt.Fprintln(os.Stderr, "Error: -values must contain at least one value")
			os.Exit(1)
		}
		if len(multiValues) > int(modbus.MaxWriteCoilCount) {
			fmt.Fprintf(os.Stderr, "Error: cannot write more than %d coils in a single request\n", modbus.MaxWriteCoilCount)
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
	if useSingle {
		writeSingleCoil(client, modbus.Address(*address), singleValue)
	} else {
		writeMultipleCoils(client, modbus.Address(*address), multiValues)
	}

	fmt.Println()
	fmt.Println("Done.")
}

// writeSingleCoil demonstrates FC 0x05 (Write Single Coil).
//
// The Modbus Write Single Coil request encodes the coil value as a 16-bit
// value in the "Output Value" field:
//   - 0xFF00 = ON (coil energized)
//   - 0x0000 = OFF (coil de-energized)
//   - Any other value is invalid and should produce an exception
//
// The normal response is an exact echo of the request, confirming the write.
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.5
func writeSingleCoil(client *modbus.Client, address modbus.Address, value modbus.CoilValue) {
	stateStr := "OFF"
	if value {
		stateStr = "ON"
	}

	fmt.Printf("--- Writing single coil (FC 0x05) ---\n")
	fmt.Printf("  Address: %d\n", address)
	fmt.Printf("  Value:   %s\n", stateStr)
	fmt.Println()

	// Step 1: Read the current coil state.
	fmt.Println("  Reading current coil state before write...")
	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	before, err := client.ReadCoils(readCtx, address, 1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current state: %v\n", err)
	} else {
		beforeStr := "OFF"
		if before[0] {
			beforeStr = "ON"
		}
		fmt.Printf("  Current state: %s\n", beforeStr)
	}
	fmt.Println()

	// Step 2: Write the new coil value using FC 0x05.
	fmt.Printf("  Writing coil %d to %s...\n", address, stateStr)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()

	err = client.WriteSingleCoil(writeCtx, address, value)
	if err != nil {
		handleError("WriteSingleCoil", err)
		return
	}
	fmt.Println("  Write successful.")
	fmt.Println()

	// Step 3: Read back to verify.
	fmt.Println("  Reading back to verify...")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer verifyCancel()

	after, err := client.ReadCoils(verifyCtx, address, 1)
	if err != nil {
		handleError("ReadCoils (verify)", err)
		return
	}

	afterStr := "OFF"
	if after[0] {
		afterStr = "ON"
	}
	fmt.Printf("  Read-back state: %s\n", afterStr)
	if after[0] == value {
		fmt.Println("  Verification: PASS -- read-back matches written value.")
	} else {
		fmt.Printf("  Verification: MISMATCH -- wrote %s but read %s.\n", stateStr, afterStr)
	}
}

// writeMultipleCoils demonstrates FC 0x0F (Write Multiple Coils).
//
// The Modbus Write Multiple Coils request contains:
//   - Function Code: 0x0F
//   - Starting Address: 2 bytes (big-endian)
//   - Quantity of Outputs: 2 bytes (big-endian), 1-1968
//   - Byte Count: 1 byte (= ceil(quantity / 8))
//   - Output Values: packed bits, LSB of first byte = first coil
//
// The normal response contains the starting address and quantity, confirming
// how many coils were written.
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.11
func writeMultipleCoils(client *modbus.Client, address modbus.Address, values []modbus.CoilValue) {
	qty := len(values)
	fmt.Printf("--- Writing %d coils (FC 0x0F) ---\n", qty)
	fmt.Printf("  Starting address: %d\n", address)
	fmt.Printf("  Values: ")
	for i, v := range values {
		if i > 0 {
			fmt.Print(",")
		}
		if v {
			fmt.Print("1")
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println()
	fmt.Println()

	// Step 1: Read the current coil states.
	fmt.Println("  Reading current coil states before write...")
	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()

	before, err := client.ReadCoils(readCtx, address, modbus.Quantity(qty))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current states: %v\n", err)
	} else {
		fmt.Printf("  %-10s %-10s\n", "Address", "Current")
		fmt.Printf("  %-10s %-10s\n", "-------", "-------")
		for i, v := range before {
			state := "OFF"
			if v {
				state = "ON"
			}
			fmt.Printf("  %-10d %-10s\n", int(address)+i, state)
		}
	}
	fmt.Println()

	// Step 2: Write all coils in a single FC 0x0F request.
	fmt.Printf("  Writing %d coils starting at address %d...\n", qty, address)
	writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer writeCancel()

	err = client.WriteMultipleCoils(writeCtx, address, values)
	if err != nil {
		handleError("WriteMultipleCoils", err)
		return
	}
	fmt.Println("  Write successful.")
	fmt.Println()

	// Step 3: Read back all coils to verify.
	fmt.Println("  Reading back to verify...")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer verifyCancel()

	after, err := client.ReadCoils(verifyCtx, address, modbus.Quantity(qty))
	if err != nil {
		handleError("ReadCoils (verify)", err)
		return
	}

	fmt.Printf("  %-10s %-10s %-10s %-10s\n", "Address", "Written", "Read Back", "Match")
	fmt.Printf("  %-10s %-10s %-10s %-10s\n", "-------", "-------", "---------", "-----")
	allMatch := true
	for i, v := range values {
		written := "OFF"
		if v {
			written = "ON"
		}
		readBack := "OFF"
		if after[i] {
			readBack = "ON"
		}
		match := "OK"
		if after[i] != v {
			match = "MISMATCH"
			allMatch = false
		}
		fmt.Printf("  %-10d %-10s %-10s %-10s\n", int(address)+i, written, readBack, match)
	}
	fmt.Println()
	if allMatch {
		fmt.Println("  Verification: PASS -- all coils match.")
	} else {
		fmt.Println("  Verification: FAIL -- some coils do not match.")
	}
}

// handleError prints a human-readable error message with Modbus-specific
// context when applicable.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)
		if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
			fmt.Fprintf(os.Stderr, "  Hint: The target coil address is not writable or does not exist.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionInvalidDataValue) {
			fmt.Fprintf(os.Stderr, "  Hint: The server rejected the coil value.\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
