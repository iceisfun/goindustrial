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
	address := common.Address(0)   // Address of the coil to write
	value := common.CoilValue(true) // Value to write (ON/true or OFF/false)

	// Write single coil
	err = modbusClient.WriteSingleCoil(ctx, address, value)
	if err != nil {
		fmt.Println("Failed to write coil:", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully set coil at address %d to %t\n", address, value)

	// Read back the value to verify it was written
	coils, err := modbusClient.ReadCoils(ctx, address, 1)
	if err != nil {
		fmt.Println("Failed to read back coil value:", err)
		os.Exit(1)
	}

	if len(coils) > 0 {
		fmt.Printf("Read back coil %d: %t\n", address, coils[0])
	}
}