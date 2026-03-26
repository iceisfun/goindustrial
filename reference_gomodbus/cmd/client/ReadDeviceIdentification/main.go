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

	// Read basic device identification
	fmt.Println("Reading basic device identification...")
	identity, err := modbusClient.ReadDeviceIdentification(
		ctx, common.ReadDeviceIDBasicStream, common.DeviceIDObjectCode(0))
	
	if err != nil {
		// Check if the error is due to unsupported function
		if common.IsFunctionNotSupportedError(err) {
			fmt.Println("Note: Device identification is not supported by this device")
			os.Exit(1)
		} else {
			// It's a different kind of error
			fmt.Println("Error reading device identification:", err)
			os.Exit(1)
		}
	}

	// Display basic device identification
	fmt.Println("Basic Device Information:")
	fmt.Println("-------------------------")
	fmt.Printf("Vendor Name:    %s\n", identity.GetVendorName())
	fmt.Printf("Product Code:   %s\n", identity.GetProductCode())
	fmt.Printf("Revision:       %s\n", identity.GetRevision())

	// Try to read extended device identification
	fmt.Println("\nAttempting to read extended device identification...")
	extendedIdentity, err := modbusClient.ReadDeviceIdentification(
		ctx, common.ReadDeviceIDExtendedStream, common.DeviceIDObjectCode(0))
	
	if err == nil {
		fmt.Println("Extended Device Information:")
		fmt.Println("---------------------------")
		
		// Display optional fields if they exist
		if vendorURL := extendedIdentity.GetVendorURL(); vendorURL != "" {
			fmt.Printf("Vendor URL:     %s\n", vendorURL)
		}
		if productName := extendedIdentity.GetProductName(); productName != "" {
			fmt.Printf("Product Name:   %s\n", productName)
		}
		if modelName := extendedIdentity.GetModelName(); modelName != "" {
			fmt.Printf("Model Name:     %s\n", modelName)
		}
		if appName := extendedIdentity.GetUserApplicationName(); appName != "" {
			fmt.Printf("User App Name:  %s\n", appName)
		}
		
		// Display any additional objects
		foundExtended := false
		for _, obj := range extendedIdentity.Objects {
			if obj.ID >= 0x80 {
				if !foundExtended {
					fmt.Println("\nExtended Objects:")
					fmt.Println("----------------")
					foundExtended = true
				}
				fmt.Printf("Object 0x%02X:    %s\n", byte(obj.ID), obj.Value)
			}
		}
	} else if !common.IsFunctionNotSupportedError(err) {
		fmt.Printf("\nError reading extended device identification: %v\n", err)
	} else {
		fmt.Println("\nExtended device identification not supported")
	}
}