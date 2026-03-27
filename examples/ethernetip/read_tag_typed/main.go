// Command read_tag_typed demonstrates the generic typed read helpers
// ethernetip.Read[T] and ethernetip.ReadSlice[T], as well as the struct
// unmarshaling method ReadTagInto.
//
// These helpers provide a type-safe way to read PLC tags without manually
// parsing raw bytes. Under the hood they:
//
//  1. Send a CIP Read Tag request (service 0x4C) via SendRRData (EIP 0x006F).
//  2. Strip the 2-byte (or 4-byte for structs) CIP type code header from the
//     response.
//  3. Unmarshal the remaining bytes into the Go type T using binary.Read
//     (little-endian) or, if T implements cip.Unmarshaler, via UnmarshalCIP.
//
// The generic type parameter T must be a fixed-size type compatible with
// encoding/binary (int8, int16, int32, int64, uint8, uint16, uint32, uint64,
// float32, float64, bool) or a struct implementing cip.Unmarshaler.
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -tag MyDINT -type DINT
//	go run . -addr 192.168.1.10:44818 -tag MyArray -type DINT -count 10
//	go run . -addr 192.168.1.10:44818 -tag MyTimer -type TIMER
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags.
	// -----------------------------------------------------------------------
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format")
	tagName := flag.String("tag", "", "Tag name on the PLC")
	typeName := flag.String("type", "DINT", "CIP type: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, TIMER, COUNTER")
	count := flag.Uint("count", 1, "Number of elements to read (1 for scalar, >1 for array)")
	flag.Parse()

	if *tagName == "" {
		fmt.Fprintln(os.Stderr, "error: -tag is required")
		flag.Usage()
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Connect to the PLC with a 10-second overall timeout.
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *addr, err)
	}
	defer client.Close()

	fmt.Printf("Connected to %s\n\n", *addr)

	// -----------------------------------------------------------------------
	// Dispatch to the correct generic read based on the -type flag.
	//
	// ethernetip.Read[T] is a generic function:
	//
	//   func Read[T any](c *Client, ctx context.Context, tagName string) (T, error)
	//
	// The type parameter T tells Go which type to unmarshal into. The CIP type
	// code in the response is checked only for header-length purposes (2 bytes
	// for atomic types, 4 bytes for structs). The actual decoding is done by
	// binary.Read or cip.Unmarshaler, so T must match the tag's physical size.
	//
	// ethernetip.ReadSlice[T] is the array variant:
	//
	//   func ReadSlice[T any](c *Client, ctx context.Context, tagName string, count uint16) ([]T, error)
	//
	// It reads `count` elements and returns them as a Go slice.
	//
	// For structured types like Timer and Counter, we use ReadTagInto which
	// calls ReadTag and then cip.Unmarshal, allowing the struct to implement
	// cip.Unmarshaler for custom decoding logic.
	// -----------------------------------------------------------------------
	elements := uint16(*count)

	switch strings.ToUpper(*typeName) {
	// ---- Boolean ----
	case "BOOL":
		// CIP BOOL (0x00C1) -> Go bool (1 byte).
		// Note: Logix BOOLs in arrays may be packed as bits within DINTs,
		// but individual BOOL tags read as a full byte (0x00 or 0x01).
		if elements == 1 {
			val, err := ethernetip.Read[bool](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[bool] failed: %v", err)
			}
			fmt.Printf("BOOL value: %v\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[bool](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[bool] failed: %v", err)
			}
			fmt.Printf("BOOL values: %v\n", vals)
		}

	// ---- Signed integers ----
	case "SINT":
		// CIP SINT (0x00C2) -> Go int8 (1 byte, signed).
		if elements == 1 {
			val, err := ethernetip.Read[int8](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[int8] failed: %v", err)
			}
			fmt.Printf("SINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[int8](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[int8] failed: %v", err)
			}
			fmt.Printf("SINT values: %v\n", vals)
		}

	case "INT":
		// CIP INT (0x00C3) -> Go int16 (2 bytes, little-endian, signed).
		if elements == 1 {
			val, err := ethernetip.Read[int16](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[int16] failed: %v", err)
			}
			fmt.Printf("INT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[int16](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[int16] failed: %v", err)
			}
			fmt.Printf("INT values: %v\n", vals)
		}

	case "DINT":
		// CIP DINT (0x00C4) -> Go int32 (4 bytes, little-endian, signed).
		// This is the most commonly used integer type on Logix controllers.
		if elements == 1 {
			val, err := ethernetip.Read[int32](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[int32] failed: %v", err)
			}
			fmt.Printf("DINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[int32](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[int32] failed: %v", err)
			}
			fmt.Printf("DINT values: %v\n", vals)
		}

	case "LINT":
		// CIP LINT (0x00C5) -> Go int64 (8 bytes, little-endian, signed).
		if elements == 1 {
			val, err := ethernetip.Read[int64](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[int64] failed: %v", err)
			}
			fmt.Printf("LINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[int64](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[int64] failed: %v", err)
			}
			fmt.Printf("LINT values: %v\n", vals)
		}

	// ---- Unsigned integers ----
	case "USINT":
		// CIP USINT (0x00C6) -> Go uint8 (1 byte, unsigned).
		if elements == 1 {
			val, err := ethernetip.Read[uint8](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[uint8] failed: %v", err)
			}
			fmt.Printf("USINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[uint8](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[uint8] failed: %v", err)
			}
			fmt.Printf("USINT values: %v\n", vals)
		}

	case "UINT":
		// CIP UINT (0x00C7) -> Go uint16 (2 bytes, little-endian, unsigned).
		if elements == 1 {
			val, err := ethernetip.Read[uint16](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[uint16] failed: %v", err)
			}
			fmt.Printf("UINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[uint16](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[uint16] failed: %v", err)
			}
			fmt.Printf("UINT values: %v\n", vals)
		}

	case "UDINT":
		// CIP UDINT (0x00C8) -> Go uint32 (4 bytes, little-endian, unsigned).
		if elements == 1 {
			val, err := ethernetip.Read[uint32](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[uint32] failed: %v", err)
			}
			fmt.Printf("UDINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[uint32](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[uint32] failed: %v", err)
			}
			fmt.Printf("UDINT values: %v\n", vals)
		}

	case "ULINT":
		// CIP ULINT (0x00C9) -> Go uint64 (8 bytes, little-endian, unsigned).
		if elements == 1 {
			val, err := ethernetip.Read[uint64](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[uint64] failed: %v", err)
			}
			fmt.Printf("ULINT value: %d\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[uint64](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[uint64] failed: %v", err)
			}
			fmt.Printf("ULINT values: %v\n", vals)
		}

	// ---- Floating point ----
	case "REAL":
		// CIP REAL (0x00CA) -> Go float32 (4 bytes, IEEE 754 single precision).
		if elements == 1 {
			val, err := ethernetip.Read[float32](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[float32] failed: %v", err)
			}
			fmt.Printf("REAL value: %f\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[float32](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[float32] failed: %v", err)
			}
			fmt.Printf("REAL values: %v\n", vals)
		}

	case "LREAL":
		// CIP LREAL (0x00CB) -> Go float64 (8 bytes, IEEE 754 double precision).
		if elements == 1 {
			val, err := ethernetip.Read[float64](client, ctx, *tagName)
			if err != nil {
				log.Fatalf("Read[float64] failed: %v", err)
			}
			fmt.Printf("LREAL value: %f\n", val)
		} else {
			vals, err := ethernetip.ReadSlice[float64](client, ctx, *tagName, elements)
			if err != nil {
				log.Fatalf("ReadSlice[float64] failed: %v", err)
			}
			fmt.Printf("LREAL values: %v\n", vals)
		}

	// ---- Structured types ----
	case "TIMER":
		// Timers are structured CIP types (type code >= 0x02A0). The generic
		// Read[T] function works for them because cip.Timer implements
		// cip.Unmarshaler.
		//
		// Alternatively, you can use client.ReadTimer which handles the struct
		// header and calls cip.DecodeTimer directly.
		//
		// Here we demonstrate ReadTagInto, which reads the tag and unmarshals
		// it into an arbitrary pointer destination.
		fmt.Println("--- Using ReadTagInto for Timer ---")
		var timer cip.Timer
		if err := client.ReadTagInto(ctx, *tagName, &timer); err != nil {
			log.Fatalf("ReadTagInto (Timer) failed: %v", err)
		}
		fmt.Printf("Timer:\n")
		fmt.Printf("  PRE (preset):      %d ms\n", timer.PRE)
		fmt.Printf("  ACC (accumulated):  %d ms\n", timer.ACC)
		fmt.Printf("  EN  (enable):       %v\n", timer.EN)
		fmt.Printf("  TT  (timer timing): %v\n", timer.TT)
		fmt.Printf("  DN  (done):         %v\n", timer.DN)

	case "COUNTER":
		// Counters follow the same 14-byte layout as Timers but with
		// different status bits.
		fmt.Println("--- Using ReadTagInto for Counter ---")
		var counter cip.Counter
		if err := client.ReadTagInto(ctx, *tagName, &counter); err != nil {
			log.Fatalf("ReadTagInto (Counter) failed: %v", err)
		}
		fmt.Printf("Counter:\n")
		fmt.Printf("  PRE (preset):     %d\n", counter.PRE)
		fmt.Printf("  ACC (accumulated): %d\n", counter.ACC)
		fmt.Printf("  CU  (count up):    %v\n", counter.CU)
		fmt.Printf("  CD  (count down):  %v\n", counter.CD)
		fmt.Printf("  DN  (done):        %v\n", counter.DN)
		fmt.Printf("  OV  (overflow):    %v\n", counter.OV)
		fmt.Printf("  UN  (underflow):   %v\n", counter.UN)

	default:
		fmt.Fprintf(os.Stderr, "Unsupported type: %s\n", *typeName)
		fmt.Fprintln(os.Stderr, "Supported: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, TIMER, COUNTER")
		os.Exit(1)
	}

	fmt.Println("\nDone.")
}
