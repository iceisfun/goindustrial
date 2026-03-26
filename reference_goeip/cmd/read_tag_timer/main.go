package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/client"
)

func main() {
	ip := flag.String("addr", "localhost:44818", "IP address of the PLC")
	tagName := flag.String("tag", "", "Name of the Timer tag to read")
	flag.Parse()

	if *tagName == "" {
		fmt.Println("Error: --tag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Create a simple logger
	logger := internal.NewConsoleLogger()

	// Create client
	c, err := client.Connect(*ip, logger)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Read Timer
	timer, err := c.ReadTimer(*tagName)
	if err != nil {
		log.Fatalf("Failed to read timer '%s': %v", *tagName, err)
	}

	// Print results
	fmt.Printf("Timer: %s\n", *tagName)
	fmt.Printf("  PRE: %d\n", timer.PRE)
	fmt.Printf("  ACC: %d\n", timer.ACC)
	fmt.Printf("  EN:  %v\n", timer.EN)
	fmt.Printf("  TT:  %v\n", timer.TT)
	fmt.Printf("  DN:  %v\n", timer.DN)
}
