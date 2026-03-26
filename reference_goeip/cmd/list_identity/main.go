package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	addr := flag.String("addr", "192.168.1.100:44818", "PLC Address")
	flag.Parse()

	logger := internal.NewConsoleLogger()
	c, err := client.Connect(*addr, logger)
	if err != nil {
		logger.Errorf("Failed to create client: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	logger.Infof("Listing Identity...")
	identities, err := c.ListIdentity()
	if err != nil {
		logger.Errorf("Failed to list identity: %v", err)
	} else {
		logger.Infof("Found %d identities:", len(identities))
		for i, id := range identities {
			fmt.Printf("Identity %d:\n", i+1)
			fmt.Printf("  Encapsulation Version: %d\n", id.EncapsVersion)
			// SocketAddr is [16]byte (struct sockaddr_in)
			// Port: bytes 2-3 (Big Endian)
			// IP: bytes 4-7 (Big Endian)
			port := uint16(id.SocketAddr[2])<<8 | uint16(id.SocketAddr[3])
			ip := fmt.Sprintf("%d.%d.%d.%d", id.SocketAddr[4], id.SocketAddr[5], id.SocketAddr[6], id.SocketAddr[7])
			fmt.Printf("  Socket Address: %s:%d\n", ip, port)
			fmt.Printf("  Vendor ID: %d\n", id.VendorID)
			fmt.Printf("  Device Type: %d\n", id.DeviceType)
			fmt.Printf("  Product Code: %d\n", id.ProductCode)
			fmt.Printf("  Revision: %d.%d\n", id.Revision[0], id.Revision[1])
			fmt.Printf("  Status: 0x%04X\n", id.Status)
			fmt.Printf("  Serial Number: 0x%08X\n", id.SerialNumber)
			fmt.Printf("  Product Name: %s\n", id.ProductName)
			fmt.Printf("  State: %d\n", id.State)
		}
	}

	logger.Infof("Listing Services...")
	services, err := c.ListServices()
	if err != nil {
		logger.Errorf("Failed to list services: %v", err)
	} else {
		logger.Infof("Found %d services:", len(services))
		for i, s := range services {
			fmt.Printf("Service %d:\n", i+1)
			fmt.Printf("  Version: %d\n", s.Version)
			fmt.Printf("  Flags: 0x%04X\n", s.CapabilityFlags)
			fmt.Printf("  Name: %s\n", s.Name)
		}
	}
}
