// Command custom_type demonstrates how to register a custom CIP struct type
// with the type registry so that vendor-specific or site-specific UDT/AOI
// types can be decoded, encoded, and displayed by name.
//
// Many Rockwell Logix projects define Add-On Instructions (AOIs) or
// User-Defined Types (UDTs) that appear in ListTags output as UNKNOWN(0x…).
// By implementing cip.TypeCodec and calling cip.RegisterType, these types
// participate in:
//
//   - cip.DataType.String(): "SET_ON_3_TMR" instead of "UNKNOWN(0x2F83)"
//   - cip.LookupType(): automatic codec lookup for typed reads
//   - cip.GoTypeToCIPType(): WriteTag support for custom structs
//
// This example defines a SET_ON_3_TMR AOI (a timer with three enable
// inputs) and shows registration, decoding, encoding, and hex dump.
//
// Usage:
//
//	go run .
//	go run . -addr 192.168.1.10:44818 -tag SET_ON_3_TMR
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/hexdump"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// -----------------------------------------------------------------------
// Step 1: Define your POD struct.
//
// This is a site-specific AOI (Add-On Instruction) timer with three
// independent enable inputs. The wire layout (as observed on the PLC
// via a raw ReadTag hex dump) is:
//
//	Offset 0-3:  Control/status DINT — EN, EN2, EN3, TT, DN packed bits
//	Offset 4-7:  PRE (DINT) — preset in milliseconds
//	Offset 8-11: ACC (DINT) — accumulated in milliseconds
//
// Total: 12 bytes. Note: this does NOT have the 2-byte reserved prefix
// that standard Rockwell Timer/Counter types use — AOI memory layouts
// are defined by the AOI author.
// -----------------------------------------------------------------------

// SetOn3Timer is a site-specific AOI that gates a timer behind three
// independent enable inputs.
type SetOn3Timer struct {
	PRE int32 // Preset (ms)
	ACC int32 // Accumulated (ms)
	EN  bool  // Enable 1
	EN2 bool  // Enable 2
	EN3 bool  // Enable 3
	TT  bool  // Timer Timing
	DN  bool  // Done
}

// Status-bit positions within the 32-bit control DINT.
const (
	setOn3EN  = 31
	setOn3EN2 = 28
	setOn3EN3 = 27
	setOn3TT  = 30
	setOn3DN  = 29
)

// -----------------------------------------------------------------------
// Step 2: Implement cip.TypeCodec.
//
// TypeCodec = Marshaler + Unmarshaler + CIPType().
// Optionally implement fmt.Stringer for name display.
// -----------------------------------------------------------------------

// CIPType returns the CIP DataType code for this struct. This code is
// controller-specific — discover it via ListTags.
func (s *SetOn3Timer) CIPType() cip.DataType { return 0x2F83 }

// String returns the human-readable type name. When registered, this is used
// by cip.DataType(0x2F83).String() instead of "UNKNOWN(0x2F83)".
func (s *SetOn3Timer) String() string { return "SET_ON_3_TMR" }

// UnmarshalCIP decodes the 12-byte AOI memory layout.
func (s *SetOn3Timer) UnmarshalCIP(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("SET_ON_3_TMR: need 12 bytes, got %d", len(data))
	}

	status := binary.LittleEndian.Uint32(data[0:4])
	s.PRE = int32(binary.LittleEndian.Uint32(data[4:8]))
	s.ACC = int32(binary.LittleEndian.Uint32(data[8:12]))

	s.EN = (status & (1 << setOn3EN)) != 0
	s.EN2 = (status & (1 << setOn3EN2)) != 0
	s.EN3 = (status & (1 << setOn3EN3)) != 0
	s.TT = (status & (1 << setOn3TT)) != 0
	s.DN = (status & (1 << setOn3DN)) != 0
	return nil
}

// MarshalCIP encodes the struct into the 12-byte wire format.
func (s *SetOn3Timer) MarshalCIP() ([]byte, error) {
	var buf [12]byte

	var status uint32
	if s.EN {
		status |= 1 << setOn3EN
	}
	if s.EN2 {
		status |= 1 << setOn3EN2
	}
	if s.EN3 {
		status |= 1 << setOn3EN3
	}
	if s.TT {
		status |= 1 << setOn3TT
	}
	if s.DN {
		status |= 1 << setOn3DN
	}
	binary.LittleEndian.PutUint32(buf[0:4], status)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(s.PRE))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(s.ACC))
	return buf[:], nil
}

// -----------------------------------------------------------------------
// Step 3: Register at init() time.
//
// Call cip.RegisterType once per type code. The factory returns a new
// zero-value pointer. After this, cip.DataType(0x2F83).String() returns
// "SET_ON_3_TMR" and cip.LookupType(0x2F83) returns a ready codec.
// -----------------------------------------------------------------------

func init() {
	cip.RegisterType(0x2F83, func() cip.TypeCodec {
		return new(SetOn3Timer)
	})
}

func main() {
	addr := flag.String("addr", "", "PLC address (host:port). If empty, runs offline demo")
	tag := flag.String("tag", "MyCustomTimer", "Tag name to read")
	flag.Parse()

	// -----------------------------------------------------------------
	// Offline demo: show registration, encode, decode, and hex dump
	// without a real PLC connection.
	// -----------------------------------------------------------------
	if *addr == "" {
		offlineDemo()
		return
	}

	// -----------------------------------------------------------------
	// Online: read the tag from a real PLC.
	// -----------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithHexDump(os.Stdout),
	)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Close()

	// Use the generic Read helper — it strips the struct header and calls
	// UnmarshalCIP automatically because SetOn3Timer implements Unmarshaler.
	val, err := ethernetip.Read[SetOn3Timer](client, ctx, *tag)
	if err != nil {
		log.Fatalf("read %q: %v", *tag, err)
	}

	fmt.Printf("\n%s:\n", *tag)
	printSetOn3Timer(&val)
}

func offlineDemo() {
	fmt.Println("=== Custom CIP Type Registry Demo ===")
	fmt.Println()

	// Verify name resolution works.
	fmt.Printf("DataType(0x2F83).String() = %s\n", cip.DataType(0x2F83))
	fmt.Printf("DataType(0xAF83).String() = %s  (array variant)\n", cip.DataType(0xAF83))
	fmt.Println()

	// Verify LookupType returns our codec.
	codec := cip.LookupType(0x2F83)
	if codec == nil {
		log.Fatal("LookupType returned nil — registration failed")
	}
	fmt.Printf("LookupType(0x2F83) -> %T\n", codec)
	fmt.Println()

	// Build a sample value and encode it.
	original := SetOn3Timer{
		PRE: 8000,
		ACC: 0,
		EN:  true,
		EN2: true,
		EN3: false,
		TT:  true,
		DN:  false,
	}

	encoded, err := original.MarshalCIP()
	if err != nil {
		log.Fatalf("MarshalCIP: %v", err)
	}

	// Hex dump the encoded bytes.
	fmt.Printf("Encoded wire bytes (%d bytes):\n", len(encoded))
	fmt.Println()
	hexdump.NewDumper(os.Stdout).Dump(encoded, hexdump.DirWrite)
	fmt.Println()

	// Decode back.
	var decoded SetOn3Timer
	if err := decoded.UnmarshalCIP(encoded); err != nil {
		log.Fatalf("UnmarshalCIP: %v", err)
	}

	fmt.Println("Decoded fields:")
	printSetOn3Timer(&decoded)

	// Verify GoTypeToCIPType works.
	dt, err := cip.GoTypeToCIPType(&decoded)
	if err != nil {
		log.Fatalf("GoTypeToCIPType: %v", err)
	}
	fmt.Printf("\nGoTypeToCIPType(*SetOn3Timer) = %s (0x%04X)\n", dt, uint16(dt))

	// Show what ListTags output would look like.
	fmt.Println()
	fmt.Println("Simulated ListTags output:")
	fmt.Printf("  %-10s  %-40s  %s (0x%04X)\n",
		"Instance", "Name", "Type", 0)
	fmt.Printf("  %-10s  %-40s  %s (0x%04X)\n",
		"--------", "----", "----", 0)
	fmt.Printf("  %-10d  %-40s  %s (0x%04X)\n",
		432, "MyCustomTimer", cip.DataType(0xAF83), uint16(0xAF83))
	fmt.Printf("  %-10d  %-40s  %s (0x%04X)\n",
		100, "MyDINT", cip.DataType(0x00C4), uint16(0x00C4))
}

func printSetOn3Timer(s *SetOn3Timer) {
	fmt.Printf("  PRE: %d ms", s.PRE)
	if s.PRE > 0 {
		fmt.Printf("  (%.1f s)", float64(s.PRE)/1000)
	}
	fmt.Println()

	fmt.Printf("  ACC: %d ms", s.ACC)
	if s.ACC > 0 {
		fmt.Printf("  (%.1f s)", float64(s.ACC)/1000)
	}
	fmt.Println()

	if s.PRE > 0 {
		pct := float64(s.ACC) / float64(s.PRE) * 100
		if pct > 100 {
			pct = 100
		}
		fmt.Printf("  Progress: %.1f%%\n", pct)
	}
	fmt.Println()
	fmt.Printf("  EN:  %v\n", s.EN)
	fmt.Printf("  EN2: %v\n", s.EN2)
	fmt.Printf("  EN3: %v\n", s.EN3)
	fmt.Printf("  TT:  %v\n", s.TT)
	fmt.Printf("  DN:  %v\n", s.DN)

	// Hex dump the raw encoded bytes for inspection.
	encoded, err := s.MarshalCIP()
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Println("  Raw bytes:")

	var buf bytes.Buffer
	hexdump.NewDumper(&buf).Dump(encoded, hexdump.DirWrite)
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		if len(line) > 0 {
			fmt.Printf("    %s\n", line)
		}
	}
}
