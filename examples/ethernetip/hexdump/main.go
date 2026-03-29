// Command hexdump demonstrates wire-level hex dump tracing for EtherNet/IP
// traffic. Every byte read from or written to the TCP connection is printed
// in traditional hexdump -C format, showing the complete EIP encapsulation
// frames including the 24-byte header and CIP payload.
//
// Two modes are shown:
//   - Console output: hex dumps are written to stdout for interactive debugging.
//   - File logging: hex dumps are written to a binary trace file for offline
//     analysis with tools like hexdump(1) or a hex editor.
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -tag MyDINT
//	go run . -addr 192.168.1.10:44818 -tag MyDINT -log trace.hex
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
)

func main() {
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address (host:port)")
	tag := flag.String("tag", "MyDINT", "Tag name to read")
	logFile := flag.String("log", "", "Write hex dump to this file (in addition to stdout)")
	flag.Parse()

	// -----------------------------------------------------------------------
	// Set up the hex dump writer.
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
	// The hex dump captures the full EIP encapsulation layer: the 24-byte
	// header, CPF items, and CIP message router requests/responses. This
	// includes the RegisterSession handshake at connection time.
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Connecting to %s...\n\n", *addr)

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithHexDump(dumpWriter),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// -----------------------------------------------------------------------
	// Read a tag — the hex dump captures the SendRRData exchange.
	// -----------------------------------------------------------------------
	fmt.Println("--- Reading tag ---")
	fmt.Println()

	data, err := client.ReadTag(ctx, *tag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadTag: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("--- Decoded ---")
	fmt.Printf("  Tag:  %s\n", *tag)
	fmt.Printf("  Data: % X\n", data)

	fmt.Println()
	fmt.Println("Done.")
}
