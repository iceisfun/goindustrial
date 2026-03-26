package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Moonlight-Companies/gomodbus/cmd/args"
	"github.com/Moonlight-Companies/gomodbus/common"
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

	// Example parameters
	startAddress := common.Address(0)  // Starting address for writing coils
	
	// Create a pattern of coil values to write
	coilValues := []common.CoilValue{
		true,   // First coil ON
		false,  // Second coil OFF
		true,   // Third coil ON
		true,   // Fourth coil ON
		false,  // Fifth coil OFF
	}

	// Write multiple coils
	err = modbusClient.WriteMultipleCoils(ctx, startAddress, coilValues)
	if err != nil {
		fmt.Println("Failed to write coils:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote %d coils starting at address %d\n", len(coilValues), startAddress)

	// Read back the values to verify they were written
	readCoils, err := modbusClient.ReadCoils(ctx, startAddress, common.Quantity(len(coilValues)))
	if err != nil {
		fmt.Println("Failed to read back coil values:", err)
		os.Exit(1)
	}

	// Display the values that were read back
	fmt.Println("\nVerifying written values:")
	for i, value := range readCoils {
		fmt.Printf("Coil %d: %t\n", int(startAddress)+i, value)
	}
}