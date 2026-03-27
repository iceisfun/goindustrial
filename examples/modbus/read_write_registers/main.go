// Example: read_write_registers
//
// Demonstrates the Read/Write Multiple Registers function (FC 0x17), which
// performs a read and a write in a single atomic Modbus transaction.
//
// FC 0x17 (Read/Write Multiple Registers) is a combined function code that:
//   1. Writes a set of values to a contiguous block of holding registers
//   2. Reads a (potentially different) contiguous block of holding registers
//
// Both operations happen atomically within a single request/response cycle.
// The write is performed BEFORE the read, so if the read and write address
// ranges overlap, the read will return the newly written values.
//
// This function code is particularly useful in scenarios where you need to:
//   - Update a control parameter and immediately read back related status
//   - Implement a handshake protocol where writing a command triggers a
//     response in a status register
//   - Reduce round-trip time by combining two operations into one
//
// Limitations:
//   - Read: up to 125 registers (same as FC 0x03)
//   - Write: up to 121 registers (slightly less than FC 0x10's 123 limit,
//     because the request PDU must also carry the read parameters)
//   - Not all devices support FC 0x17; many only implement FC 0x03/0x10
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.17
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

	// Read parameters: the starting address and quantity of registers to
	// read in the combined operation.
	readAddress := flag.Int("read-address", 0, "Starting address for the read portion (0-65535)")
	readCount := flag.Int("read-count", 5, "Number of registers to read (1-125)")

	// Write parameters: the starting address and values to write. The write
	// is performed before the read within the same transaction.
	writeAddress := flag.Int("write-address", 0, "Starting address for the write portion (0-65535)")
	writeValues := flag.String("write-values", "100,200,300", "Comma-separated register values to write (e.g., \"100,200,300\")")

	flag.Parse()

	// Validate read count.
	if *readCount < 1 || *readCount > int(modbus.MaxReadWriteReadCount) {
		fmt.Fprintf(os.Stderr, "Error: read-count must be between 1 and %d\n", modbus.MaxReadWriteReadCount)
		os.Exit(1)
	}

	// Parse the write values from the comma-separated string.
	var writeVals []modbus.RegisterValue
	if *writeValues != "" {
		parts := strings.Split(*writeValues, ",")
		writeVals = make([]modbus.RegisterValue, 0, len(parts))
		for i, part := range parts {
			part = strings.TrimSpace(part)
			v, err := strconv.ParseUint(part, 10, 16)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid write value at position %d (%q): %v\n", i, part, err)
				os.Exit(1)
			}
			writeVals = append(writeVals, modbus.RegisterValue(v))
		}
	}
	if len(writeVals) == 0 {
		fmt.Fprintln(os.Stderr, "Error: -write-values must contain at least one value")
		os.Exit(1)
	}
	if len(writeVals) > int(modbus.MaxReadWriteWriteCount) {
		fmt.Fprintf(os.Stderr, "Error: cannot write more than %d registers in a read/write request\n", modbus.MaxReadWriteWriteCount)
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
	// Step 1: Show what we are about to do
	// -----------------------------------------------------------------------
	fmt.Println("--- Read/Write Multiple Registers (FC 0x17) ---")
	fmt.Println()
	fmt.Println("This function performs an atomic write-then-read in a single transaction.")
	fmt.Println("The write is executed BEFORE the read on the server side.")
	fmt.Println()
	fmt.Printf("  Write: %d register(s) starting at address %d\n", len(writeVals), *writeAddress)
	fmt.Printf("  Write values: ")
	for i, v := range writeVals {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%d", v)
	}
	fmt.Println()
	fmt.Printf("  Read:  %d register(s) starting at address %d\n", *readCount, *readAddress)
	fmt.Println()

	// -----------------------------------------------------------------------
	// Step 2: Read current state of both address ranges (for comparison)
	// -----------------------------------------------------------------------
	// Read the registers that will be written to, so we can see the before/after.
	fmt.Println("  Current state of write-target registers:")
	preWriteCtx, preWriteCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer preWriteCancel()

	preWriteRegs, err := client.ReadHoldingRegisters(preWriteCtx, modbus.Address(*writeAddress), modbus.Quantity(len(writeVals)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current state: %v\n", err)
	} else {
		for i, v := range preWriteRegs {
			fmt.Printf("    Address %d = %d (0x%04X)\n", *writeAddress+i, v, v)
		}
	}
	fmt.Println()

	// Also read the current state of the read-target registers.
	fmt.Println("  Current state of read-target registers:")
	preReadCtx, preReadCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer preReadCancel()

	preReadRegs, err := client.ReadHoldingRegisters(preReadCtx, modbus.Address(*readAddress), modbus.Quantity(*readCount))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not read current state: %v\n", err)
	} else {
		for i, v := range preReadRegs {
			fmt.Printf("    Address %d = %d (0x%04X)\n", *readAddress+i, v, v)
		}
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Step 3: Execute the combined Read/Write operation (FC 0x17)
	// -----------------------------------------------------------------------
	// This is the key operation. A single Modbus transaction is sent that:
	//   1. Writes writeVals to writeAddress..writeAddress+len(writeVals)-1
	//   2. Reads readCount registers from readAddress..readAddress+readCount-1
	//
	// The response contains only the read data. The write is confirmed
	// implicitly (if no exception is returned, the write succeeded).
	fmt.Println("  Executing ReadWriteMultipleRegisters (FC 0x17)...")
	rwCtx, rwCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rwCancel()

	readResults, err := client.ReadWriteMultipleRegisters(
		rwCtx,
		modbus.Address(*readAddress),
		modbus.Quantity(*readCount),
		modbus.Address(*writeAddress),
		writeVals,
	)
	if err != nil {
		handleError("ReadWriteMultipleRegisters", err)
		os.Exit(1)
	}

	fmt.Println("  Operation successful.")
	fmt.Println()

	// -----------------------------------------------------------------------
	// Step 4: Display the read results from the combined operation
	// -----------------------------------------------------------------------
	// These are the register values returned by the read portion of FC 0x17.
	// If the read and write address ranges overlap, these values reflect the
	// just-written data (since the write happens first).
	fmt.Println("  Read results (from the FC 0x17 response):")
	fmt.Printf("  %-10s %-10s %-10s\n", "Address", "Decimal", "Hex")
	fmt.Printf("  %-10s %-10s %-10s\n", "-------", "-------", "------")
	for i, val := range readResults {
		fmt.Printf("  %-10d %-10d 0x%04X\n", *readAddress+i, val, val)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Step 5: Verify the write by reading back the written registers
	// -----------------------------------------------------------------------
	// Although FC 0x17 implicitly confirms the write (no exception = success),
	// we do a separate read-back to demonstrate verification and to show the
	// written values explicitly.
	fmt.Println("  Verifying write by reading back written registers (FC 0x03):")
	verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer verifyCancel()

	verifyRegs, err := client.ReadHoldingRegisters(verifyCtx, modbus.Address(*writeAddress), modbus.Quantity(len(writeVals)))
	if err != nil {
		handleError("ReadHoldingRegisters (verify)", err)
	} else {
		fmt.Printf("  %-10s %-12s %-12s %-10s\n", "Address", "Written", "Read Back", "Match")
		fmt.Printf("  %-10s %-12s %-12s %-10s\n", "-------", "-------", "---------", "-----")
		allMatch := true
		for i, v := range writeVals {
			match := "OK"
			if verifyRegs[i] != v {
				match = "MISMATCH"
				allMatch = false
			}
			fmt.Printf("  %-10d %-12d %-12d %-10s\n", *writeAddress+i, v, verifyRegs[i], match)
		}
		fmt.Println()
		if allMatch {
			fmt.Println("  Verification: PASS -- all written values confirmed.")
		} else {
			fmt.Println("  Verification: FAIL -- some written values do not match.")
		}
	}

	fmt.Println()
	fmt.Println("Done.")
}

// handleError prints a human-readable error message with Modbus-specific
// context when applicable.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)

		if modbus.IsExceptionError(err, modbus.ExceptionFunctionCodeNotSupported) {
			fmt.Fprintf(os.Stderr, "  Hint: FC 0x17 (ReadWriteMultipleRegisters) is not supported by this device.\n")
			fmt.Fprintf(os.Stderr, "  Many devices only support FC 0x03 (ReadHoldingRegisters) and FC 0x10 (WriteMultipleRegisters).\n")
			fmt.Fprintf(os.Stderr, "  Use separate read and write operations instead.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
			fmt.Fprintf(os.Stderr, "  Hint: One of the address ranges (read or write) is not available on this device.\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
