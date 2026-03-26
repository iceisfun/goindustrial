package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Moonlight-Companies/gomodbus/cmd/args"
)

func main() {
	// Parse command-line arguments
	modbusArgs := args.ParseArgs()

	// Create a Modbus client
	modbusClient := modbusArgs.CreateClient()

	// Connect to the server
	ctx := context.Background()
	err := modbusClient.Connect(ctx)
	if err != nil {
		fmt.Println("Failed to connect to Modbus server:", err)
		os.Exit(1)
	}
	defer modbusClient.Disconnect(ctx)

	// Read exception status
	status, err := modbusClient.ReadExceptionStatus(ctx)
	if err != nil {
		fmt.Println("Failed to read exception status:", err)
		os.Exit(1)
	}

	// Display the status with our helpful String() method
	fmt.Printf("Exception Status: %s\n", status)

	// Check each bit individually
	fmt.Println("\nIndividual exception bits:")
	for i := 0; i < 8; i++ {
		if status&(1<<i) != 0 {
			fmt.Printf("  Exception bit %d is set\n", i)
		} else {
			fmt.Printf("  Exception bit %d is clear\n", i)
		}
	}
}