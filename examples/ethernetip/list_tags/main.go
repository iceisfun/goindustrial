// Command list_tags enumerates all tags (symbols) on a Rockwell Logix PLC
// and displays their instance ID, name, and CIP data type.
//
// Logix controllers maintain a Symbol Object (CIP class 0x6B) that maps
// tag names to memory addresses and type information. Each tag is an
// instance of this class. To list all tags, this program:
//
//  1. Queries class-level attributes (instance 0) of the Symbol class to
//     discover the maximum instance ID.
//  2. Iterates from instance 1 through the maximum, requesting each
//     instance's Name (attribute 1) and Type (attribute 2) via CIP
//     GetAttributeList (service 0x03).
//  3. Skips instances that return "object does not exist" (0x16) -- these
//     are gaps where a tag was deleted but the instance ID was not reused.
//
// This is the same mechanism that Studio 5000 and FactoryTalk use to
// populate their tag browsers.
//
// CIP addressing for the Symbol Object:
//
//	Class:    0x6B (Symbol Object)
//	Instance: 0 for class attributes, 1..N for individual tags
//	Service:  0x03 (GetAttributeList)
//
// The request data for GetAttributeList contains:
//
//	[Attribute Count: UINT (2 bytes)]
//	[Attribute ID 1:  UINT (2 bytes)]  -> 1 = Name
//	[Attribute ID 2:  UINT (2 bytes)]  -> 2 = Type
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818
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
	// -----------------------------------------------------------------------
	// Parse command-line flags.
	// -----------------------------------------------------------------------
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format")
	flag.Parse()

	// -----------------------------------------------------------------------
	// Connect to the PLC.
	//
	// We use a longer timeout here because listing tags involves many
	// sequential CIP requests (one per instance ID). A controller with 500
	// tags can take several seconds.
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *addr, err)
	}
	defer client.Close()

	fmt.Printf("Connected to %s\n", *addr)
	fmt.Println("Enumerating tags via CIP Symbol Object (class 0x6B)...")
	fmt.Println()

	// -----------------------------------------------------------------------
	// List all tags.
	//
	// ListTags performs the full enumeration inside a single retry-able
	// operation. If the connection drops mid-enumeration, the entire
	// iteration restarts from instance 1 on the next retry attempt.
	//
	// Internally it:
	//   1. Sends GetAttributeList to class 0x6B, instance 0, requesting
	//      attributes 1 (Revision) and 2 (Max Instance).
	//   2. Loops from instance 1 to Max Instance, sending GetAttributeList
	//      for attributes 1 (Name) and 2 (Type) on each instance.
	//   3. Collects all valid responses into a []cip.SymbolInstance.
	//
	// Each CIP request is wrapped in an EIP SendRRData (0x006F) frame with
	// the session handle from the RegisterSession handshake.
	// -----------------------------------------------------------------------
	tags, err := client.ListTags(ctx)
	if err != nil {
		log.Fatalf("ListTags failed: %v", err)
	}

	// -----------------------------------------------------------------------
	// Display the results.
	// -----------------------------------------------------------------------
	if len(tags) == 0 {
		fmt.Println("No tags found on the controller.")
		fmt.Println("This can happen if:")
		fmt.Println("  - The controller has no user-defined tags")
		fmt.Println("  - External access is restricted for all tags")
		fmt.Println("  - The controller firmware does not support symbol enumeration")
		return
	}

	fmt.Printf("Found %d tags:\n\n", len(tags))
	fmt.Printf("  %-10s  %-40s  %s\n", "Instance", "Name", "Type")
	fmt.Printf("  %-10s  %-40s  %s\n", "--------", "----", "----")

	for _, tag := range tags {
		// tag.Type is a cip.DataType with a String() method that resolves
		// the numeric code to a human-readable name (e.g. "DINT", "REAL").
		// If the type code has the array bit (0x8000) set, String() appends
		// "[]" to indicate an array type.
		fmt.Printf("  %-10d  %-40s  %s (0x%04X)\n",
			tag.InstanceID,
			tag.Name,
			tag.Type,
			uint16(tag.Type),
		)
	}

	fmt.Printf("\nTotal: %d tags\n", len(tags))
	fmt.Println("\nDone.")
}
