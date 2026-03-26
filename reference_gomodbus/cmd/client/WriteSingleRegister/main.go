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
	address := common.Address(0)          // Address of the register to write
	value := common.RegisterValue(12345)  // Value to write (0-65535)

	// Write single register
	err = modbusClient.WriteSingleRegister(ctx, address, value)
	if err != nil {
		fmt.Println("Failed to write register:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully set register at address %d to %d (0x%04X)\n", address, value, value)

	// Read back the value to verify it was written
	registers, err := modbusClient.ReadHoldingRegisters(ctx, address, 1)
	if err != nil {
		fmt.Println("Failed to read back register value:", err)
		os.Exit(1)
	}

	if len(registers) > 0 {
		fmt.Printf("Read back register %d: %d (0x%04X)\n", address, registers[0], registers[0])
	}
}