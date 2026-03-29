// Command hexdump demonstrates wire-level hex dump tracing for Modbus TCP
// traffic. Every byte read from or written to the TCP connection is printed
// in traditional hexdump -C format, making it easy to see exactly what goes
// on the wire during protocol exchanges.
//
// Two modes are shown:
//   - Console output: hex dumps are written to stdout for interactive debugging.
//   - File logging: hex dumps are written to a binary trace file for offline
//     analysis with tools like hexdump(1) or a hex editor.
//
// Usage:
//
//	go run . -addr 127.0.0.1 -port 5020
//	go run . -addr 127.0.0.1 -port 5020 -log trace.hex
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")
	port := flag.Int("port", modbus.DefaultTCPPort, "Modbus TCP port")
	unit := flag.Int("unit", 1, "Modbus unit ID (slave address)")
	logFile := flag.String("log", "", "Write hex dump to this file (in addition to stdout)")
	flag.Parse()

	// -----------------------------------------------------------------------
	// Set up the hex dump writer.
	//
	// By default, the hex dump goes to stdout. If -log is specified, the dump
	// is written to both stdout and the file using io.MultiWriter.
	// -----------------------------------------------------------------------
	var dumpWriter io.Writer = os.Stdout

	if *logFile != "" {
		f, err := os.Create(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		dumpWriter = io.MultiWriter(os.Stdout, f)
		fmt.Printf("Hex dump output also written to %s\n\n", *logFile)
	}

	// -----------------------------------------------------------------------
	// Connect with WithHexDump enabled.
	//
	// Every byte read from or written to the TCP socket will be formatted
	// as a hex dump and written to dumpWriter. This happens at the transport
	// level, so it captures the complete MBAP frame including headers.
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Connecting to Modbus TCP server at %s:%d...\n\n", *addr, *port)

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),
		modbus.WithUnitID(modbus.UnitID(*unit)),
		modbus.WithHexDump(dumpWriter),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// -----------------------------------------------------------------------
	// Read some holding registers — the hex dump appears automatically.
	// -----------------------------------------------------------------------
	fmt.Println("--- Reading 3 holding registers at address 0 ---")
	fmt.Println()

	regs, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadHoldingRegisters: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("--- Decoded values ---")
	for i, v := range regs {
		fmt.Printf("  Register %d = %d (0x%04X)\n", i, v, v)
	}

	// -----------------------------------------------------------------------
	// Write a register — the hex dump captures both request and response.
	// -----------------------------------------------------------------------
	fmt.Println()
	fmt.Println("--- Writing register 100 = 0x1234 ---")
	fmt.Println()

	if err := client.WriteSingleRegister(ctx, 100, 0x1234); err != nil {
		fmt.Fprintf(os.Stderr, "WriteSingleRegister: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Done.")
}
