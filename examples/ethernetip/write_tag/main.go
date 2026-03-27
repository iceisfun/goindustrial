// Command write_tag writes a value to a tag on a Rockwell Logix PLC over
// EtherNet/IP and reads it back to verify.
//
// The CIP Write Tag service (service code 0x4D) sends:
//   - A symbolic path identifying the tag name.
//   - The CIP data type code (e.g. 0x00C4 for DINT).
//   - The number of elements (always 1 in this example).
//   - The raw data bytes in little-endian order.
//
// This example accepts the CIP type name as a command-line flag (-type) and
// parses the -value string into the appropriate Go type before writing.
//
// Supported type names: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT,
// ULINT, REAL, LREAL, STRING.
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -tag MyDINT -type DINT -value 42
//	go run . -addr 192.168.1.10:44818 -tag MyREAL -type REAL -value 3.14
//	go run . -addr 192.168.1.10:44818 -tag MyBool -type BOOL -value true
//	go run . -addr 192.168.1.10:44818 -tag MyString -type STRING -value "Hello PLC"
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags.
	// -----------------------------------------------------------------------
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format (EIP default port is 44818)")
	tagName := flag.String("tag", "", "Tag name on the PLC (symbolic addressing)")
	typeName := flag.String("type", "DINT", "CIP data type: BOOL, SINT, INT, DINT, LINT, USINT, UINT, UDINT, ULINT, REAL, LREAL, STRING")
	value := flag.String("value", "", "Value to write (parsed according to -type)")
	flag.Parse()

	if *tagName == "" || *value == "" {
		fmt.Fprintln(os.Stderr, "error: -tag and -value are required")
		flag.Usage()
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Parse the value string into a Go type that matches the requested CIP
	// data type.
	//
	// The ethernetip client's WriteTag method calls cip.GoTypeToCIPType to
	// detect the CIP type from the Go value, and then cip.Marshal to encode
	// it into bytes. So we need to provide the correct Go type here.
	//
	// CIP type -> Go type mapping:
	//   BOOL   -> bool
	//   SINT   -> int8
	//   INT    -> int16
	//   DINT   -> int32
	//   LINT   -> int64
	//   USINT  -> uint8
	//   UINT   -> uint16
	//   UDINT  -> uint32
	//   ULINT  -> uint64
	//   REAL   -> float32
	//   LREAL  -> float64
	//   STRING -> string
	// -----------------------------------------------------------------------
	goValue, err := parseValue(*typeName, *value)
	if err != nil {
		log.Fatalf("Failed to parse value %q as %s: %v", *value, *typeName, err)
	}

	// -----------------------------------------------------------------------
	// Connect to the PLC.
	//
	// ethernetip.Connect dials TCP and registers an EIP session in one step.
	// The session handle returned by the PLC is stored internally and used
	// for all subsequent CIP requests wrapped in SendRRData (0x006F).
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
	// Write the tag.
	//
	// WriteTag(ctx, tagName, value) performs three internal steps:
	//   1. Calls cip.GoTypeToCIPType(value) to determine the 2-byte CIP type
	//      code from the Go type.
	//   2. Calls cip.Marshal(value) to encode the value into little-endian
	//      bytes.
	//   3. Builds a CIP Write Tag request (service 0x4D) with a symbolic path,
	//      the type code, element count 1, and the data bytes. Sends it via
	//      SendRRData within the current EIP session.
	//
	// If the PLC returns a non-zero CIP general status, the error will be a
	// cip.Error with the status code and optional extended status words.
	// -----------------------------------------------------------------------
	fmt.Printf("Writing tag %q = %v (type %s)\n", *tagName, goValue, *typeName)

	if err := client.WriteTag(ctx, *tagName, goValue); err != nil {
		log.Fatalf("WriteTag failed: %v", err)
	}

	fmt.Println("Write successful.")

	// -----------------------------------------------------------------------
	// Read back the tag to verify the write.
	//
	// We use the raw ReadTag to show both the type code and the data bytes,
	// so you can confirm the type is what you expect.
	// -----------------------------------------------------------------------
	fmt.Println("\n--- Read-back verification ---")

	raw, err := client.ReadTag(ctx, *tagName)
	if err != nil {
		log.Fatalf("ReadTag (verification) failed: %v", err)
	}

	if len(raw) < 2 {
		log.Fatalf("Response too short (%d bytes)", len(raw))
	}

	typeCode := cip.DataType(binary.LittleEndian.Uint16(raw[0:2]))
	fmt.Printf("Type Code:  0x%04X (%s)\n", uint16(typeCode), typeCode)
	fmt.Printf("Raw bytes:  % X\n", raw)

	// Decode the read-back value using the same type we wrote.
	readBack, err := decodeValue(*typeName, raw)
	if err != nil {
		log.Fatalf("Failed to decode read-back: %v", err)
	}
	fmt.Printf("Read-back:  %v\n", readBack)

	fmt.Println("\nDone.")
}

// parseValue converts a string representation of a value into the Go type that
// corresponds to the given CIP type name. The Go type determines which CIP
// type code WriteTag will use.
func parseValue(typeName, s string) (any, error) {
	switch strings.ToUpper(typeName) {
	case "BOOL":
		// Accept "true", "false", "1", "0".
		v, err := strconv.ParseBool(s)
		return v, err

	case "SINT":
		v, err := strconv.ParseInt(s, 10, 8)
		return int8(v), err

	case "INT":
		v, err := strconv.ParseInt(s, 10, 16)
		return int16(v), err

	case "DINT":
		v, err := strconv.ParseInt(s, 10, 32)
		return int32(v), err

	case "LINT":
		v, err := strconv.ParseInt(s, 10, 64)
		return int64(v), err

	case "USINT":
		v, err := strconv.ParseUint(s, 10, 8)
		return uint8(v), err

	case "UINT":
		v, err := strconv.ParseUint(s, 10, 16)
		return uint16(v), err

	case "UDINT":
		v, err := strconv.ParseUint(s, 10, 32)
		return uint32(v), err

	case "ULINT":
		v, err := strconv.ParseUint(s, 10, 64)
		return uint64(v), err

	case "REAL":
		v, err := strconv.ParseFloat(s, 32)
		return float32(v), err

	case "LREAL":
		v, err := strconv.ParseFloat(s, 64)
		return float64(v), err

	case "STRING":
		// Strings are passed through as-is.
		return s, nil

	default:
		return nil, fmt.Errorf("unsupported CIP type %q", typeName)
	}
}

// decodeValue interprets the raw CIP response bytes (including the 2-byte type
// code prefix) according to the given CIP type name and returns a displayable
// Go value.
func decodeValue(typeName string, raw []byte) (any, error) {
	if len(raw) < 2 {
		return nil, fmt.Errorf("raw data too short")
	}

	// Determine header length. Structured types (>= 0x02A0) have a 4-byte
	// header (type code + structure handle). Atomic types have a 2-byte header.
	typeCode := cip.DataType(binary.LittleEndian.Uint16(raw[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(raw) < hdrLen {
		return nil, fmt.Errorf("raw data too short for header")
	}
	data := raw[hdrLen:]

	switch strings.ToUpper(typeName) {
	case "BOOL":
		if len(data) < 1 {
			return nil, fmt.Errorf("not enough data for BOOL")
		}
		return data[0] != 0, nil

	case "SINT":
		if len(data) < 1 {
			return nil, fmt.Errorf("not enough data for SINT")
		}
		return int8(data[0]), nil

	case "INT":
		if len(data) < 2 {
			return nil, fmt.Errorf("not enough data for INT")
		}
		return int16(binary.LittleEndian.Uint16(data[0:2])), nil

	case "DINT":
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough data for DINT")
		}
		return int32(binary.LittleEndian.Uint32(data[0:4])), nil

	case "LINT":
		if len(data) < 8 {
			return nil, fmt.Errorf("not enough data for LINT")
		}
		return int64(binary.LittleEndian.Uint64(data[0:8])), nil

	case "USINT":
		if len(data) < 1 {
			return nil, fmt.Errorf("not enough data for USINT")
		}
		return uint8(data[0]), nil

	case "UINT":
		if len(data) < 2 {
			return nil, fmt.Errorf("not enough data for UINT")
		}
		return binary.LittleEndian.Uint16(data[0:2]), nil

	case "UDINT":
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough data for UDINT")
		}
		return binary.LittleEndian.Uint32(data[0:4]), nil

	case "ULINT":
		if len(data) < 8 {
			return nil, fmt.Errorf("not enough data for ULINT")
		}
		return binary.LittleEndian.Uint64(data[0:8]), nil

	case "REAL":
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough data for REAL")
		}
		bits := binary.LittleEndian.Uint32(data[0:4])
		return float32FromBits(bits), nil

	case "LREAL":
		if len(data) < 8 {
			return nil, fmt.Errorf("not enough data for LREAL")
		}
		bits := binary.LittleEndian.Uint64(data[0:8])
		return float64FromBits(bits), nil

	case "STRING":
		// Rockwell STRING: 4-byte length (DINT) followed by character data.
		if len(data) < 4 {
			return nil, fmt.Errorf("not enough data for STRING length prefix")
		}
		strLen := binary.LittleEndian.Uint32(data[0:4])
		if len(data) < 4+int(strLen) {
			return nil, fmt.Errorf("not enough data for STRING body (need %d, have %d)", strLen, len(data)-4)
		}
		return string(data[4 : 4+strLen]), nil

	default:
		return nil, fmt.Errorf("unsupported CIP type %q for decoding", typeName)
	}
}

// float32FromBits converts a uint32 bit pattern to float32.
func float32FromBits(bits uint32) float32 {
	return math.Float32frombits(bits)
}

// float64FromBits converts a uint64 bit pattern to float64.
func float64FromBits(bits uint64) float64 {
	return math.Float64frombits(bits)
}
