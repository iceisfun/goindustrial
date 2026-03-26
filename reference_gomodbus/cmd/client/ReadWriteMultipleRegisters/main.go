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
	readAddress := common.Address(10)   // Starting address for reading registers
	readQuantity := common.Quantity(5)  // Number of registers to read
	writeAddress := common.Address(20)  // Starting address for writing registers

	// Create values to write
	writeValues := []common.RegisterValue{
		10000,  // First register
		20000,  // Second register
		30000,  // Third register
	}

	// Perform a combined read/write operation
	readValues, err := modbusClient.ReadWriteMultipleRegisters(
		ctx, readAddress, readQuantity, writeAddress, writeValues)
	if err != nil {
		fmt.Println("Failed to perform read/write operation:", err)
		os.Exit(1)
	}

	// Display the read values
	fmt.Printf("Read %d registers from address %d:\n", readQuantity, readAddress)
	for i, value := range readValues {
		fmt.Printf("Register %d: %d (0x%04X)\n", int(readAddress)+i, value, value)
	}

	// Also wrote values
	fmt.Printf("\nWrote %d registers to address %d\n", len(writeValues), writeAddress)
	for i, value := range writeValues {
		fmt.Printf("Register %d: %d (0x%04X)\n", int(writeAddress)+i, value, value)
	}

	// Verify the written values
	verifyValues, err := modbusClient.ReadHoldingRegisters(ctx, writeAddress, common.Quantity(len(writeValues)))
	if err != nil {
		fmt.Println("Failed to verify written values:", err)
		os.Exit(1)
	}

	fmt.Println("\nVerifying written values:")
	for i, value := range verifyValues {
		fmt.Printf("Register %d: %d (0x%04X)\n", int(writeAddress)+i, value, value)
	}
}