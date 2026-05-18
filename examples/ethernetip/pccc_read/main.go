// Command pccc_read reads a single data-table address from an Allen-Bradley
// SLC 500 / MicroLogix controller over EtherNet/IP.
//
// PCCC (Programmable Controller Communication Commands) is a legacy
// Allen-Bradley application protocol. Unlike a Logix controller, an SLC/
// MicroLogix has no named tags — data lives in data-table files such as
// N7:0 (integer file 7, element 0), F8:5, B3:0/2, T4:0.ACC, or S:1.
//
// On EtherNet/IP, every PCCC command is tunneled inside the CIP
// Execute_PCCC service (class 0x67, service 0x4B):
//
//	[TCP] -> [EIP Encap] -> [CPF] -> [CIP Execute_PCCC] -> [requestor-ID] -> [PCCC command]
//
// This example builds the PCCC typed-read command (FNC 0xA2) from a parsed
// address, ships it through the existing EIP session, decodes the reply,
// and interprets the bytes based on the file type.
//
// Usage:
//
//	# Read integer file 7, element 0:
//	go run . -addr 10.30.40.71:44818 -tag N7:0
//
//	# Read float file 8, element 5:
//	go run . -addr 10.30.40.71:44818 -tag F8:5
//
//	# Read a single bit:
//	go run . -addr 10.30.40.71:44818 -tag B3:0/2
//
//	# Read a timer accumulator:
//	go run . -addr 10.30.40.71:44818 -tag T4:0.ACC
//
//	# Read status word:
//	go run . -addr 10.30.40.71:44818 -tag S:1
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/pccc"
)

func main() {
	addr := flag.String("addr", "10.30.40.71:44818", "SLC/MicroLogix address in host:port format (PCCC default port is 44818)")
	tag := flag.String("tag", "N7:0", "PCCC data-table address (e.g. N7:0, F8:5, B3:0/2, T4:0.ACC, S:1)")
	flag.Parse()
	if *tag == "" {
		fmt.Fprintln(os.Stderr, "error: -tag is required")
		flag.Usage()
		os.Exit(1)
	}

	// --------------------------------------------------------------------
	// 1. Parse the SLC address.
	// --------------------------------------------------------------------
	a, err := pccc.ParseAddress(*tag)
	if err != nil {
		log.Fatalf("parse address %q: %v", *tag, err)
	}
	fmt.Printf("Parsed address: %s\n", a)
	fmt.Printf("  FileType:   %s\n", a.FileType)
	fmt.Printf("  FileNumber: %d\n", a.FileNumber)
	fmt.Printf("  Element:    %d\n", a.Element)
	fmt.Printf("  SubElement: %d\n", a.SubElement)
	if a.BitNum >= 0 {
		fmt.Printf("  Bit:        %d\n", a.BitNum)
	}

	// How many bytes to request. For scalar word-sized files (N, B, S, I,
	// O, D) it is 2; for F it is 4; for a single word of a multi-word
	// element (T/C/R sub-fields) it is 2.
	byteSize := elementBytes(a)
	fmt.Printf("  Read size:  %d bytes\n", byteSize)

	// --------------------------------------------------------------------
	// 2. Connect to the controller. ethernetip.Connect performs the TCP
	//    dial + RegisterSession handshake in one step.
	// --------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("connect %s: %v", *addr, err)
	}
	defer client.Close()
	fmt.Printf("\nConnected to %s\n", *addr)

	// --------------------------------------------------------------------
	// 3. Build the PCCC typed-read command (FNC 0xA2). The transaction
	//    number (TNS) just has to be unique within an outstanding-request
	//    window; the SLC echoes it back in the reply.
	// --------------------------------------------------------------------
	tns := uint16(time.Now().UnixNano() & 0xFFFF)
	cmd, err := pccc.EncodeTypedRead(tns, byteSize, a.FileNumber, a.FileType, a.Element, a.SubElement)
	if err != nil {
		log.Fatalf("encode PCCC read: %v", err)
	}
	fmt.Printf("PCCC request:   % X\n", cmd)

	// --------------------------------------------------------------------
	// 4. Ship the PCCC bytes via CIP Execute_PCCC. ExecutePCCC handles
	//    the CIP framing and strips the 7-byte requestor-ID echo from
	//    the response, returning the raw PCCC reply bytes.
	// --------------------------------------------------------------------
	rawReply, err := client.ExecutePCCC(ctx, cmd)
	if err != nil {
		log.Fatalf("ExecutePCCC: %v", err)
	}
	fmt.Printf("PCCC reply:     % X\n", rawReply)

	// --------------------------------------------------------------------
	// 5. Decode the reply. A non-zero STS surfaces as *pccc.Error.
	// --------------------------------------------------------------------
	reply, err := pccc.DecodeReply(rawReply)
	if err != nil {
		var pe *pccc.Error
		if errors.As(err, &pe) {
			log.Fatalf("PCCC error: %v", pe)
		}
		log.Fatalf("decode PCCC reply: %v", err)
	}
	if reply.TNS != tns {
		log.Printf("warning: TNS mismatch (sent 0x%04X, got 0x%04X)", tns, reply.TNS)
	}

	// --------------------------------------------------------------------
	// 6. Interpret the bytes based on the file type.
	// --------------------------------------------------------------------
	fmt.Println()
	fmt.Println("--- Decoded value ---")
	printValue(a, reply.Data)
}

// elementBytes returns the number of PCCC data-table bytes to read for the
// given address. For multi-word element types (T, C, R) we read a single
// word (2 bytes) when a sub-field is specified; otherwise we read the
// whole 6-byte element.
func elementBytes(a pccc.Address) int {
	switch a.FileType {
	case pccc.FileTypeFloat:
		return 4
	case pccc.FileTypeTimer, pccc.FileTypeCounter, pccc.FileTypeControl:
		// Whole element is control(2) + PRE(2) + ACC(2) = 6 bytes.
		// If the caller asked for a sub-field, just read that word.
		if a.SubElement > 0 || a.BitNum >= 0 {
			return 2
		}
		return 6
	default:
		return 2
	}
}

func printValue(a pccc.Address, data []byte) {
	switch a.FileType {
	case pccc.FileTypeFloat:
		if len(data) < 4 {
			fmt.Printf("(short reply for F: %d bytes)\n", len(data))
			return
		}
		f := math.Float32frombits(binary.LittleEndian.Uint32(data[:4]))
		fmt.Printf("%s = %g  (REAL)\n", a, f)

	case pccc.FileTypeTimer, pccc.FileTypeCounter, pccc.FileTypeControl:
		if a.SubElement == 0 && a.BitNum < 0 {
			// Whole element: control word + PRE + ACC.
			if len(data) < 6 {
				fmt.Printf("(short reply for full %s element: %d bytes)\n", a.FileType.Letter(), len(data))
				return
			}
			ctl := binary.LittleEndian.Uint16(data[0:2])
			pre := int16(binary.LittleEndian.Uint16(data[2:4]))
			acc := int16(binary.LittleEndian.Uint16(data[4:6]))
			fmt.Printf("%s = { control: 0x%04X, PRE: %d, ACC: %d }\n", a, ctl, pre, acc)
			return
		}
		// Single-word read: integer or bit-within-word.
		fallthrough

	default:
		if len(data) < 2 {
			fmt.Printf("(short reply: %d bytes)\n", len(data))
			return
		}
		word := binary.LittleEndian.Uint16(data[:2])
		if a.BitNum >= 0 {
			bit := (word >> uint(a.BitNum)) & 1
			fmt.Printf("%s = %d  (bit %d of 0x%04X)\n", a, bit, a.BitNum, word)
			return
		}
		// Word read — present as both signed and hex.
		fmt.Printf("%s = %d (0x%04X)\n", a, int16(word), word)
	}
}
