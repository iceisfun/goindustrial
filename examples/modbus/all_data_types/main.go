// Example: All Modbus Data Types
//
// This example provides a comprehensive demonstration of ALL four Modbus data
// areas and every standard read/write operation supported by the goindustrial
// client. It exercises:
//
//   - Coils (boolean, read/write)            - FC 01, 05, 0F
//   - Discrete Inputs (boolean, read-only)   - FC 02
//   - Holding Registers (uint16, read/write) - FC 03, 06, 10
//   - Input Registers (uint16, read-only)    - FC 04
//   - ReadWriteMultipleRegisters             - FC 17
//   - ReadExceptionStatus                    - FC 07
//
// Each operation is shown with clear section headers and verbose output so you
// can see exactly what is sent and received.
//
// Usage:
//
//	go run ./examples/modbus/all_data_types -addr 127.0.0.1 -port 5020
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	modbus "github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// ---------------------------------------------------------------------------
	// Parse command-line flags
	// ---------------------------------------------------------------------------

	// -addr: hostname or IP of the Modbus TCP server.
	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")

	// -port: TCP port of the target server.
	port := flag.Int("port", 5020, "Modbus TCP server port")

	// -unit: Modbus unit ID (slave address). Typically 0 for direct TCP, or 1-247
	// when routing through a Modbus gateway to serial devices.
	unit := flag.Int("unit", 0, "Modbus unit ID (slave address)")

	flag.Parse()

	// ---------------------------------------------------------------------------
	// Set up logging and context
	// ---------------------------------------------------------------------------

	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))
	ctx := context.Background()

	// ---------------------------------------------------------------------------
	// Connect to the server using the convenience function
	// ---------------------------------------------------------------------------
	//
	// modbus.Connect() creates a ReconnectingTransport, dials the server, and
	// returns a ready-to-use Client. We pass connection options (WithPort) and
	// client options (WithUnitID, WithLogger) together; Connect() sorts them
	// by type internally.

	logger.Info(ctx, "Connecting to Modbus TCP server at %s:%d (unit %d)", *addr, *port, *unit)

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),
		modbus.WithUnitID(modbus.UnitID(*unit)),
		modbus.WithLogger(logger),
	)
	if err != nil {
		logger.Error(ctx, "Failed to connect: %v", err)
		os.Exit(1)
	}

	// Ensure the client and transport are closed when we exit.
	defer func() {
		logger.Info(ctx, "Disconnecting from server")
		if err := client.Close(); err != nil {
			logger.Error(ctx, "Error closing client: %v", err)
		}
	}()

	logger.Info(ctx, "Connected successfully. Running all data type demonstrations.\n")

	// We use a per-operation timeout to prevent any single call from hanging.
	// 5 seconds is generous for a local TCP connection.
	const opTimeout = 5 * time.Second

	// Track whether any operation fails so we can report a summary at the end.
	allOK := true

	// =========================================================================
	// SECTION 1: COILS (FC 01, 05, 0F)
	// =========================================================================
	//
	// Coils are single-bit (boolean) values that are both readable and writable.
	// In the Modbus data model, coils represent discrete outputs: relay contacts,
	// digital outputs, motor starters, solenoid valves, etc.
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.1:  Read Coils (FC 01)
	//   - Section 6.5:  Write Single Coil (FC 05)
	//   - Section 6.11: Write Multiple Coils (FC 0F)
	//
	// Address range: 0 - 65535

	printSection("COILS (Boolean, Read/Write)")

	// --- Read Coils (FC 01) ---
	// Read 10 coils starting at address 0. The server packs coil values into
	// bytes (8 coils per byte, LSB first). The client unpacks them into a
	// []CoilValue ([]bool) slice.
	printSubsection("Read Coils (FC 01)")
	fmt.Println("Reading 10 coils from address 0...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		coils, err := client.ReadCoils(opCtx, 0, 10)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range coils {
			state := "OFF"
			if val {
				state = "ON"
			}
			fmt.Printf("  Coil %d = %s\n", i, state)
		}
		fmt.Println()
	}()

	// --- Write Single Coil (FC 05) ---
	// Write a single coil at address 5. The Modbus protocol encodes coil ON as
	// 0xFF00 and coil OFF as 0x0000 in the request PDU. The client handles this
	// encoding automatically; you just pass a bool.
	printSubsection("Write Single Coil (FC 05)")
	fmt.Println("Writing coil 5 = ON...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		err := client.WriteSingleCoil(opCtx, 5, true)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}
		fmt.Println("  Success: coil 5 is now ON")
		fmt.Println()
	}()

	// --- Write Multiple Coils (FC 0F) ---
	// Write a pattern of coils starting at address 0. This is more efficient
	// than writing one at a time when you need to update a contiguous block.
	// The client packs the booleans into bytes (8 per byte, LSB first) per the
	// Modbus specification.
	printSubsection("Write Multiple Coils (FC 0F)")
	coilPattern := []modbus.CoilValue{true, false, true, false, true, true, false, true}
	fmt.Printf("Writing 8 coils starting at address 0: %v\n", formatBools(coilPattern))

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		err := client.WriteMultipleCoils(opCtx, 0, coilPattern)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}
		fmt.Println("  Success: 8 coils written")
		fmt.Println()
	}()

	// --- Verify coil writes by reading back ---
	printSubsection("Verify Coil Writes (FC 01)")
	fmt.Println("Reading back 10 coils from address 0 to verify writes...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		coils, err := client.ReadCoils(opCtx, 0, 10)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range coils {
			state := "OFF"
			if val {
				state = "ON"
			}
			fmt.Printf("  Coil %d = %s\n", i, state)
		}
		fmt.Println()
	}()

	// =========================================================================
	// SECTION 2: DISCRETE INPUTS (FC 02)
	// =========================================================================
	//
	// Discrete inputs are single-bit (boolean) values that are read-only from
	// the client perspective. They represent physical inputs: proximity sensors,
	// limit switches, pushbuttons, safety interlocks, etc.
	//
	// The Modbus specification does not define a write function code for discrete
	// inputs. Only the server application can update them (e.g., from field I/O).
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.2: Read Discrete Inputs (FC 02)
	//
	// Address range: 0 - 65535

	printSection("DISCRETE INPUTS (Boolean, Read-Only)")

	// --- Read Discrete Inputs (FC 02) ---
	printSubsection("Read Discrete Inputs (FC 02)")
	fmt.Println("Reading 8 discrete inputs from address 0...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		inputs, err := client.ReadDiscreteInputs(opCtx, 0, 8)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range inputs {
			state := "OFF"
			if val {
				state = "ON"
			}
			fmt.Printf("  Discrete Input %d = %s\n", i, state)
		}
		fmt.Println()
	}()

	// =========================================================================
	// SECTION 3: HOLDING REGISTERS (FC 03, 06, 10)
	// =========================================================================
	//
	// Holding registers are 16-bit (uint16) values that are both readable and
	// writable. They are the most commonly used Modbus data area, typically
	// holding setpoints, configuration parameters, and control values.
	//
	// Each register is exactly 2 bytes, transmitted big-endian (MSB first).
	// For values larger than 16 bits (e.g., 32-bit floats, 32-bit integers),
	// applications conventionally use two consecutive registers and agree on
	// byte order (big-endian or little-endian word order).
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.3:  Read Holding Registers (FC 03)
	//   - Section 6.6:  Write Single Register (FC 06)
	//   - Section 6.12: Write Multiple Registers (FC 10)
	//
	// Address range: 0 - 65535

	printSection("HOLDING REGISTERS (uint16, Read/Write)")

	// --- Read Holding Registers (FC 03) ---
	printSubsection("Read Holding Registers (FC 03)")
	fmt.Println("Reading 10 holding registers from address 0...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		registers, err := client.ReadHoldingRegisters(opCtx, 0, 10)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range registers {
			fmt.Printf("  Holding Register %d = %d (0x%04X)\n", i, val, val)
		}
		fmt.Println()
	}()

	// --- Write Single Register (FC 06) ---
	// Write a single holding register at address 0. The new value (42) replaces
	// whatever was there before. The server echoes the address and value in its
	// response to confirm the write.
	printSubsection("Write Single Register (FC 06)")
	fmt.Println("Writing holding register 0 = 42...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		err := client.WriteSingleRegister(opCtx, 0, 42)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}
		fmt.Println("  Success: holding register 0 is now 42")
		fmt.Println()
	}()

	// --- Write Multiple Registers (FC 10) ---
	// Write a block of consecutive registers starting at address 5. This is
	// more efficient than individual writes when updating several related
	// parameters (e.g., PID tuning constants, recipe values).
	printSubsection("Write Multiple Registers (FC 10)")
	regValues := []modbus.RegisterValue{100, 200, 300, 400, 500}
	fmt.Printf("Writing 5 holding registers starting at address 5: %v\n", regValues)

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		err := client.WriteMultipleRegisters(opCtx, 5, regValues)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}
		fmt.Println("  Success: 5 holding registers written")
		fmt.Println()
	}()

	// --- Verify register writes by reading back ---
	printSubsection("Verify Register Writes (FC 03)")
	fmt.Println("Reading back 10 holding registers from address 0 to verify writes...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		registers, err := client.ReadHoldingRegisters(opCtx, 0, 10)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range registers {
			fmt.Printf("  Holding Register %d = %d (0x%04X)\n", i, val, val)
		}
		fmt.Println()
	}()

	// =========================================================================
	// SECTION 4: INPUT REGISTERS (FC 04)
	// =========================================================================
	//
	// Input registers are 16-bit (uint16) values that are read-only from the
	// client perspective. They typically hold measured process values: analog
	// inputs, sensor readings, counters, and status words.
	//
	// Like discrete inputs, there is no write function code for input registers
	// in the Modbus specification. Only the server application updates them.
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.4: Read Input Registers (FC 04)
	//
	// Address range: 0 - 65535

	printSection("INPUT REGISTERS (uint16, Read-Only)")

	// --- Read Input Registers (FC 04) ---
	printSubsection("Read Input Registers (FC 04)")
	fmt.Println("Reading 10 input registers from address 0...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		registers, err := client.ReadInputRegisters(opCtx, 0, 10)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range registers {
			fmt.Printf("  Input Register %d = %d (0x%04X)\n", i, val, val)
		}
		fmt.Println()
	}()

	// =========================================================================
	// SECTION 5: READ/WRITE MULTIPLE REGISTERS (FC 17)
	// =========================================================================
	//
	// FC 17 (0x17) performs a simultaneous read and write of holding registers
	// in a single atomic transaction. This is useful for:
	//
	//   - Updating setpoints while simultaneously reading back process values
	//   - Implementing handshake protocols (write a command, read the status)
	//   - Reducing round trips when you need both read and write
	//
	// The read and write address ranges can be different and can overlap.
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.17: Read/Write Multiple Registers (FC 17)
	//
	// Limits:
	//   - Read quantity: 1-125 registers
	//   - Write quantity: 1-121 registers

	printSection("READ/WRITE MULTIPLE REGISTERS (FC 17)")

	printSubsection("ReadWriteMultipleRegisters (FC 17)")
	writeVals := []modbus.RegisterValue{9999, 8888}
	fmt.Println("Simultaneously:")
	fmt.Println("  - Reading 3 holding registers from address 0")
	fmt.Printf("  - Writing 2 holding registers to address 8: %v\n", writeVals)

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		// ReadWriteMultipleRegisters:
		//   readAddress=0, readQuantity=3  -> reads registers 0, 1, 2
		//   writeAddress=8, writeValues=[9999, 8888] -> writes registers 8, 9
		readBack, err := client.ReadWriteMultipleRegisters(opCtx, 0, 3, 8, writeVals)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		fmt.Println("  Read result:")
		for i, val := range readBack {
			fmt.Printf("    Holding Register %d = %d (0x%04X)\n", i, val, val)
		}
		fmt.Println()
	}()

	// Verify the FC 17 write took effect by reading the written registers back.
	printSubsection("Verify FC 17 Write (FC 03)")
	fmt.Println("Reading holding registers 8-9 to verify FC 17 write...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		registers, err := client.ReadHoldingRegisters(opCtx, 8, 2)
		if err != nil {
			fmt.Printf("  ERROR: %v\n\n", err)
			allOK = false
			return
		}

		for i, val := range registers {
			fmt.Printf("  Holding Register %d = %d (0x%04X)\n", 8+i, val, val)
		}
		fmt.Println()
	}()

	// =========================================================================
	// SECTION 6: READ EXCEPTION STATUS (FC 07)
	// =========================================================================
	//
	// FC 07 reads the 8-bit exception status from the server. This is a legacy
	// function from Modbus RTU that returns the state of 8 predefined coils
	// (coils 1-8 in the original spec, mapped to bits 0-7 of the returned byte).
	//
	// In modern Modbus TCP implementations, servers may or may not support this
	// function. If the server does not implement it, it will return a
	// "function code not supported" exception (0x01).
	//
	// Ref: Modbus Application Protocol V1.1b3:
	//   - Section 6.7: Read Exception Status (FC 07)

	printSection("READ EXCEPTION STATUS (FC 07)")

	printSubsection("ReadExceptionStatus (FC 07)")
	fmt.Println("Reading exception status from server...")

	func() {
		opCtx, cancel := context.WithTimeout(ctx, opTimeout)
		defer cancel()

		status, err := client.ReadExceptionStatus(opCtx)
		if err != nil {
			// FC 07 is commonly not supported by many servers, so this error
			// is expected in many setups.
			if modbus.IsModbusError(err) {
				fmt.Printf("  Server returned Modbus exception: %v\n", err)
				fmt.Println("  (This is normal - many servers do not implement FC 07)")
			} else {
				fmt.Printf("  ERROR: %v\n", err)
				allOK = false
			}
			fmt.Println()
			return
		}

		fmt.Printf("  Exception Status: %s\n", status)
		fmt.Printf("  Raw value: 0x%02X (binary: %08b)\n", byte(status), byte(status))
		fmt.Println()
	}()

	// =========================================================================
	// SUMMARY
	// =========================================================================

	printSection("SUMMARY")
	if allOK {
		fmt.Println("All operations completed successfully.")
	} else {
		fmt.Println("Some operations failed. Review the output above for ERROR lines.")
	}
	fmt.Println()

	logger.Info(ctx, "All data type demonstrations complete.")
}

// ---------------------------------------------------------------------------
// Output formatting helpers
// ---------------------------------------------------------------------------

// printSection prints a prominent section header.
func printSection(title string) {
	width := 72
	fmt.Println(strings.Repeat("=", width))
	padding := (width - len(title) - 2) / 2
	if padding < 0 {
		padding = 0
	}
	fmt.Printf("%s %s %s\n", strings.Repeat("=", padding), title, strings.Repeat("=", width-padding-len(title)-2))
	fmt.Println(strings.Repeat("=", width))
	fmt.Println()
}

// printSubsection prints a subsection header.
func printSubsection(title string) {
	fmt.Printf("--- %s ---\n", title)
}

// formatBools formats a boolean slice as a human-readable string.
func formatBools(vals []bool) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		if v {
			parts[i] = "ON"
		} else {
			parts[i] = "OFF"
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
