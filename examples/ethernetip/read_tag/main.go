// Command read_tag reads one or more elements of a tag from a Rockwell Logix PLC
// over EtherNet/IP.
//
// EtherNet/IP (Ethernet Industrial Protocol) is an application-layer protocol
// built on top of TCP/IP. It uses the CIP (Common Industrial Protocol) at its
// core to access objects inside a controller. Every CIP request travels inside
// an EIP encapsulation frame:
//
//	[TCP] -> [EIP Header (24 bytes)] -> [CPF (Common Packet Format)] -> [CIP Message Router Request]
//
// Before any CIP traffic can flow, the client must open a TCP connection to the
// PLC on port 44818 and send a RegisterSession (0x0065) command. The PLC
// replies with a 32-bit session handle that the client includes in all
// subsequent encapsulated requests. The ethernetip.Connect helper performs
// this handshake automatically.
//
// Reading a tag uses CIP Service 0x4C (Read Tag) with a symbolic segment path.
// The symbolic path encodes the tag name as an ANSI string directly in the
// request, so there is no need to know numeric class/instance IDs.
//
// The response contains a 2-byte CIP type code followed by the raw data bytes.
// For example, reading a DINT tag returns: [0xC4, 0x00, <4 bytes LE int32>].
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -tag MyDINT
//	go run . -addr 192.168.1.10:44818 -tag MyArray -count 10
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags.
	// -----------------------------------------------------------------------
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format (EIP default port is 44818)")
	tagName := flag.String("tag", "MyDINT", "Tag name on the PLC (symbolic addressing)")
	count := flag.Uint("count", 1, "Number of elements to read (1 for scalar, >1 for array slice)")
	flag.Parse()

	if *tagName == "" {
		fmt.Fprintln(os.Stderr, "error: -tag is required")
		flag.Usage()
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Create a context with a generous timeout. All EtherNet/IP operations
	// (TCP dial, EIP session registration, CIP request/response) honour the
	// context deadline, so a single timeout covers the entire workflow.
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// -----------------------------------------------------------------------
	// Connect to the PLC.
	//
	// ethernetip.Connect performs two steps atomically:
	//   1. Dials TCP to the given address (default timeout 5 s).
	//   2. Sends RegisterSession (EIP command 0x0065) and waits for the PLC
	//      to return a session handle.
	//
	// If either step fails the function returns an error and no resources are
	// leaked.
	//
	// We also pass WithRetries(2) so that transient transport errors (e.g. a
	// brief network hiccup) are retried up to two times before surfacing.
	// CIP-level errors (wrong tag name, bad type) are never retried because
	// they indicate a logical problem rather than a transport one.
	// -----------------------------------------------------------------------
	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *addr, err)
	}
	// Always close the client when done. This sends an UnregisterSession
	// (0x0066) to the PLC and closes the TCP socket.
	defer client.Close()

	fmt.Printf("Connected to %s\n\n", *addr)

	// -----------------------------------------------------------------------
	// Raw byte read: ReadTag / ReadTagElements
	//
	// ReadTag reads a single element. ReadTagElements reads `count` elements
	// of an array tag starting at index 0.
	//
	// Both return the raw CIP response data, which always starts with a
	// 2-byte little-endian type code:
	//
	//   Bytes 0-1: CIP data type (e.g. 0x00C4 = DINT)
	//   Bytes 2+:  Actual data
	//
	// For structured types (type code >= 0x02A0), there is an additional
	// 2-byte structure handle after the type code (4-byte header total).
	// -----------------------------------------------------------------------
	fmt.Println("--- Raw byte read ---")

	var raw []byte
	if *count == 1 {
		// ReadTag is a convenience wrapper for ReadTagElements(ctx, name, 1).
		raw, err = client.ReadTag(ctx, *tagName)
	} else {
		raw, err = client.ReadTagElements(ctx, *tagName, uint16(*count))
	}
	if err != nil {
		log.Fatalf("ReadTag failed: %v", err)
	}

	// Extract the CIP type code from the first two bytes.
	if len(raw) < 2 {
		log.Fatalf("Response too short (%d bytes) - expected at least 2 for type code", len(raw))
	}
	typeCode := cip.DataType(binary.LittleEndian.Uint16(raw[0:2]))

	fmt.Printf("Tag:        %s\n", *tagName)
	fmt.Printf("Type Code:  0x%04X (%s)\n", uint16(typeCode), typeCode)
	fmt.Printf("Raw bytes:  % X\n", raw)
	fmt.Printf("Data bytes: % X\n", raw[2:]) // skip type code

	// -----------------------------------------------------------------------
	// Typed read using the generic helpers: ethernetip.Read[T] / ReadSlice[T]
	//
	// These functions read elements, strip the type-code header, and unmarshal
	// the remaining bytes into the Go type T using binary.Read (little-endian).
	//
	// We use the type code from the raw read to select the matching Go type.
	// -----------------------------------------------------------------------
	fmt.Println("\n--- Typed read ---")

	switch typeCode {
	case cip.TypeBOOL:
		typedRead[bool](client, ctx, *tagName, uint16(*count))
	case cip.TypeSINT:
		typedRead[int8](client, ctx, *tagName, uint16(*count))
	case cip.TypeINT:
		typedRead[int16](client, ctx, *tagName, uint16(*count))
	case cip.TypeDINT:
		typedRead[int32](client, ctx, *tagName, uint16(*count))
	case cip.TypeLINT:
		typedRead[int64](client, ctx, *tagName, uint16(*count))
	case cip.TypeUSINT:
		typedRead[uint8](client, ctx, *tagName, uint16(*count))
	case cip.TypeUINT:
		typedRead[uint16](client, ctx, *tagName, uint16(*count))
	case cip.TypeUDINT:
		typedRead[uint32](client, ctx, *tagName, uint16(*count))
	case cip.TypeULINT:
		typedRead[uint64](client, ctx, *tagName, uint16(*count))
	case cip.TypeREAL:
		typedRead[float32](client, ctx, *tagName, uint16(*count))
	case cip.TypeLREAL:
		typedRead[float64](client, ctx, *tagName, uint16(*count))
	default:
		fmt.Printf("No typed read for type code 0x%04X; use raw bytes above.\n", uint16(typeCode))
	}

	// -----------------------------------------------------------------------
	// Summary of the CIP type codes you will encounter most often:
	//
	//   Code    CIP Name   Go Type    Size
	//   0x00C1  BOOL       bool       1 byte
	//   0x00C2  SINT       int8       1 byte
	//   0x00C3  INT        int16      2 bytes
	//   0x00C4  DINT       int32      4 bytes
	//   0x00C5  LINT       int64      8 bytes
	//   0x00C6  USINT      uint8      1 byte
	//   0x00C7  UINT       uint16     2 bytes
	//   0x00C8  UDINT      uint32     4 bytes
	//   0x00C9  ULINT      uint64     8 bytes
	//   0x00CA  REAL       float32    4 bytes
	//   0x00CB  LREAL      float64    8 bytes
	//   0x00D0  STRING     string     variable (4-byte len prefix + chars)
	//   0x02A0  STRUCT     struct     variable (extra 2-byte handle)
	//
	// The type code in the response tells you exactly how to interpret the
	// following bytes. If you use the generic Read[T] helper you do not need
	// to worry about this - but it is useful for debugging.
	// -----------------------------------------------------------------------

	fmt.Println("\nDone.")
}

func typedRead[T any](client *ethernetip.Client, ctx context.Context, tag string, count uint16) {
	if count == 1 {
		val, err := ethernetip.Read[T](client, ctx, tag)
		if err != nil {
			log.Fatalf("Typed Read failed: %v", err)
		}
		fmt.Printf("Value (%T): %v\n", val, val)
	} else {
		vals, err := ethernetip.ReadSlice[T](client, ctx, tag, count)
		if err != nil {
			log.Fatalf("Typed ReadSlice failed: %v", err)
		}
		fmt.Printf("Values (%T): %v\n", vals, vals)
	}
}
