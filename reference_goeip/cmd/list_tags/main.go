package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	address := flag.String("addr", "192.168.1.100:44818", "PLC Address (IP:Port)")
	flag.Parse()

	logger := internal.NewConsoleLogger()
	logger.Infof("Connecting to %s...", *address)

	c, err := client.Connect(*address, logger)
	if err != nil {
		logger.Errorf("Failed to connect: %v", err)
		os.Exit(1)
	}
	defer c.Close()

	logger.Infof("Listing tags...")
	tags, err := c.ListTags()
	if err != nil {
		logger.Errorf("Failed to list tags: %v", err)
		os.Exit(1)
	}

	logger.Infof("Found %d tags:", len(tags))
	for _, t := range tags {
		fmt.Printf("ID: 0x%08X, Name: %-40s, Type: %s\n", t.InstanceID, t.Name, t.Type)
	}
}
