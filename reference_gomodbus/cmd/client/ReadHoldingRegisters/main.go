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
	startAddress := common.Address(0)  // Starting address for reading holding registers
	quantity := common.Quantity(10)    // Number of registers to read

	// Read holding registers
	registers, err := modbusClient.ReadHoldingRegisters(ctx, startAddress, quantity)
	if err != nil {
		fmt.Println("Failed to read holding registers:", err)
		os.Exit(1)
	}

	// Display the results
	fmt.Printf("Read %d holding registers starting at address %d:\n", quantity, startAddress)
	for i, value := range registers {
		fmt.Printf("Register %d: %d (0x%04X)\n", int(startAddress)+i, value, value)
	}
}