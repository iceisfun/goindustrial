// Command list_identity sends the EIP ListIdentity and ListServices commands
// to an EtherNet/IP device and displays the device's identity and supported
// services.
//
// These are the two most basic EIP discovery commands. Unlike CIP tag
// operations, they do not require an active session -- they can be sent before
// RegisterSession. However, when sent over an existing TCP session (as this
// example does), they work identically.
//
// EIP ListIdentity (command 0x0063):
//
//	This command asks the device to identify itself. The response contains a
//	CIP Identity Object (class 0x01) with fields like vendor ID, device type,
//	product code, firmware revision, serial number, and product name. This is
//	the same information you see in RSLinx's "Who Is" browser.
//
//	The response is wrapped in a CPF (Common Packet Format) item with type ID
//	0x000C (ListIdentity). The identity data follows this structure:
//
//	  [Encapsulation Version: UINT]
//	  [Socket Address: 16 bytes (struct sockaddr_in)]
//	  [Vendor ID: UINT]
//	  [Device Type: UINT]
//	  [Product Code: UINT]
//	  [Revision: 2 bytes (major, minor)]
//	  [Status: UINT]
//	  [Serial Number: UDINT]
//	  [Product Name Length: USINT]
//	  [Product Name: N bytes]
//	  [State: USINT]
//
// EIP ListServices (command 0x0004):
//
//	This command asks the device which EIP services it supports. Most devices
//	support the "Communications" service (type ID 0x0100), which indicates
//	support for CIP encapsulation over TCP. The response includes:
//
//	  [Type ID: UINT]       -- 0x0100 for Communications
//	  [Length: UINT]        -- Length of remaining data
//	  [Version: UINT]       -- Protocol version (usually 1)
//	  [Capability Flags: UINT] -- Bitmask of capabilities
//	  [Name: 16 bytes]      -- Service name (null-terminated ASCII)
//
//	Capability flags:
//	  Bit 5: Supports CIP encapsulation via TCP
//	  Bit 8: Supports CIP encapsulation via UDP
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
	addr := flag.String("addr", "192.168.1.10:44818", "EtherNet/IP device address in host:port format")
	flag.Parse()

	// -----------------------------------------------------------------------
	// Connect to the device.
	//
	// Although ListIdentity and ListServices can technically be sent without
	// a registered session, the ethernetip.Connect helper always registers a
	// session as part of the connection handshake. This is the standard
	// approach and works with all EIP devices.
	//
	// The connection sequence:
	//   1. TCP dial to host:44818
	//   2. Send RegisterSession (command 0x0065)
	//   3. Receive session handle from the device
	//
	// After this, the session handle is included in the EIP header of every
	// subsequent command, including ListIdentity and ListServices.
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
	// Send ListIdentity (EIP command 0x0063).
	//
	// This asks the device "who are you?" and returns identity information
	// from the CIP Identity Object (class 0x01, instance 1). The response
	// is formatted as a CPF with one item of type 0x000C.
	//
	// The returned []eip.ListIdentityItem contains one item per device that
	// responds. Over a point-to-point TCP connection there is always exactly
	// one item (the device we connected to). Over UDP broadcast there could
	// be multiple responses, but that is not the case here.
	// -----------------------------------------------------------------------
	fmt.Println("=== ListIdentity ===")
	fmt.Println()

	identities, err := client.ListIdentity(ctx)
	if err != nil {
		log.Fatalf("ListIdentity failed: %v", err)
	}

	if len(identities) == 0 {
		fmt.Println("No identity items returned.")
	}

	for i, id := range identities {
		if len(identities) > 1 {
			fmt.Printf("--- Device %d ---\n", i+1)
		}

		fmt.Printf("  Product Name:   %s\n", id.ProductName)
		fmt.Printf("  Vendor ID:      %d\n", id.VendorID)
		fmt.Printf("  Device Type:    %d\n", id.DeviceType)
		fmt.Printf("  Product Code:   %d\n", id.ProductCode)
		fmt.Printf("  Revision:       %d.%d\n", id.Revision[0], id.Revision[1])
		fmt.Printf("  Serial Number:  0x%08X\n", id.SerialNumber)
		fmt.Printf("  Status:         0x%04X\n", id.Status)
		fmt.Printf("  State:          %d\n", id.State)

		// Decode common vendor IDs for context.
		switch id.VendorID {
		case 1:
			fmt.Printf("  Vendor:         Rockwell Automation / Allen-Bradley\n")
		case 9:
			fmt.Printf("  Vendor:         Schneider Electric\n")
		case 47:
			fmt.Printf("  Vendor:         WAGO\n")
		case 283:
			fmt.Printf("  Vendor:         Beckhoff\n")
		}

		// Decode common device types.
		switch id.DeviceType {
		case 0x00:
			fmt.Printf("  Device Class:   Generic Device\n")
		case 0x0C:
			fmt.Printf("  Device Class:   Communications Adapter\n")
		case 0x0E:
			fmt.Printf("  Device Class:   Programmable Logic Controller\n")
		case 0x21:
			fmt.Printf("  Device Class:   CIP Motion Drive\n")
		}

		fmt.Println()
	}

	// -----------------------------------------------------------------------
	// Send ListServices (EIP command 0x0004).
	//
	// This asks the device which EIP communication services it supports.
	// The response is a list of service items. Almost every EIP device
	// reports at least the "Communications" service (type 0x0100).
	//
	// The capability flags in the response indicate whether the device
	// supports CIP over TCP (bit 5) and/or CIP over UDP (bit 8).
	// -----------------------------------------------------------------------
	fmt.Println("=== ListServices ===")
	fmt.Println()

	services, err := client.ListServices(ctx)
	if err != nil {
		log.Fatalf("ListServices failed: %v", err)
	}

	if len(services) == 0 {
		fmt.Println("No service items returned.")
	}

	for _, svc := range services {
		fmt.Printf("  Service Name:    %s\n", svc.Name)
		fmt.Printf("  Type ID:         0x%04X\n", svc.TypeID)
		fmt.Printf("  Version:         %d\n", svc.Version)
		fmt.Printf("  Capabilities:    0x%04X\n", svc.CapabilityFlags)

		// Decode capability flags.
		if svc.CapabilityFlags&(1<<5) != 0 {
			fmt.Printf("    - Supports CIP encapsulation via TCP\n")
		}
		if svc.CapabilityFlags&(1<<8) != 0 {
			fmt.Printf("    - Supports CIP encapsulation via UDP\n")
		}

		fmt.Println()
	}

	fmt.Println("Done.")
}
