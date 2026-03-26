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
	startAddress := common.Address(0)  // Starting address for writing registers
	
	// Create values to write
	registerValues := []common.RegisterValue{
		1000,  // First register
		2000,  // Second register
		3000,  // Third register
		4000,  // Fourth register
		5000,  // Fifth register
	}

	// Write multiple registers
	err = modbusClient.WriteMultipleRegisters(ctx, startAddress, registerValues)
	if err != nil {
		fmt.Println("Failed to write registers:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully wrote %d registers starting at address %d\n", len(registerValues), startAddress)

	// Read back the values to verify they were written
	readRegisters, err := modbusClient.ReadHoldingRegisters(ctx, startAddress, common.Quantity(len(registerValues)))
	if err != nil {
		fmt.Println("Failed to read back register values:", err)
		os.Exit(1)
	}

	// Display the values that were read back
	fmt.Println("\nVerifying written values:")
	for i, value := range readRegisters {
		fmt.Printf("Register %d: %d (0x%04X)\n", int(startAddress)+i, value, value)
	}
}