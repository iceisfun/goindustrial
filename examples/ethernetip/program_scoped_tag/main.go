// Command program_scoped_tag demonstrates reading a program-scoped Rockwell
// Logix tag with member access -- specifically a COUNTER instruction's whole
// struct and its individual .ACC member.
//
// Logix controllers expose tags under two scopes:
//
//   - Controller scope:   "MyTag"
//   - Program scope:      "Program:MainProgram.MyTag"
//
// And structured tags expose their members via dotted syntax:
//
//   - Whole struct:       "Program:MainProgram.MyCounter"
//   - Individual member:  "Program:MainProgram.MyCounter.ACC"
//
// Internally these are not single ANSI Extended Symbol segments -- the
// controller requires them to be split on "." into multiple symbol segments
// and (for arrays) Element segments. The goindustrial library handles this
// automatically via cip.ParseTagPath, which the tag-level APIs
// (ReadTag, ReadCounter, ReadTimer, WriteTag, ...) call internally. So all
// you need to do is pass the tag string in its natural form.
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -tag Program:MainProgram.MyCounter
//	go run . -addr 192.168.1.10:44818 -tag Program:MainProgram.MyCounter -member ACC
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
)

func main() {
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format")
	tag := flag.String("tag", "Program:MainProgram.MyCounter", "Counter tag (controller- or program-scoped)")
	member := flag.String("member", "", "Optional member to read individually (e.g. ACC, PRE). Empty = whole struct only.")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr)
	if err != nil {
		log.Fatalf("connect %s: %v", *addr, err)
	}
	defer client.Close()
	fmt.Printf("Connected to %s\n\n", *addr)

	// -----------------------------------------------------------------------
	// 1. Read the whole COUNTER struct.
	//
	// ReadCounter sends Read Tag (service 0x4C) against the multi-segment
	// EPATH built from `tag` (e.g. ["Program:MainProgram", "MyCounter"]) and
	// decodes the 14-byte response payload into a cip.Counter.
	// -----------------------------------------------------------------------
	fmt.Printf("--- Whole struct: %s ---\n", *tag)
	c, err := client.ReadCounter(ctx, *tag)
	if err != nil {
		log.Fatalf("ReadCounter: %v", err)
	}
	fmt.Printf("PRE=%d  ACC=%d\n", c.PRE, c.ACC)
	fmt.Printf("CU=%t  CD=%t  DN=%t  OV=%t  UN=%t\n\n", c.CU, c.CD, c.DN, c.OV, c.UN)

	// -----------------------------------------------------------------------
	// 2. (Optional) Read just one member of the counter.
	//
	// Asking for "Program:MainProgram.MyCounter.ACC" causes ParseTagPath to
	// build a 3-segment EPATH: ["Program:MainProgram", "MyCounter", "ACC"].
	// The controller returns a plain DINT (4 bytes) instead of the whole
	// struct, so use Read[int32] for a typed result.
	// -----------------------------------------------------------------------
	if *member == "" {
		return
	}
	full := *tag + "." + *member
	fmt.Printf("--- Member read: %s ---\n", full)
	val, err := ethernetip.Read[int32](client, ctx, full)
	if err != nil {
		log.Fatalf("Read[int32](%s): %v", full, err)
	}
	fmt.Printf("%s = %d\n", *member, val)
}
