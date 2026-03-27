// Example: device_identification
//
// Demonstrates reading device identification data from a Modbus TCP server
// using function code 0x2B / MEI type 0x0E (Read Device Identification).
//
// Device identification is part of the Modbus Encapsulated Interface (MEI)
// and allows a client to discover information about a Modbus device such as:
//
//   - Vendor Name (Object ID 0x00) -- mandatory
//   - Product Code (Object ID 0x01) -- mandatory
//   - Major/Minor Revision (Object ID 0x02) -- mandatory
//   - Vendor URL (Object ID 0x03) -- optional (regular identification)
//   - Product Name (Object ID 0x04) -- optional (regular identification)
//   - Model Name (Object ID 0x05) -- optional (regular identification)
//   - User Application Name (Object ID 0x06) -- optional (regular identification)
//
// The Read Device Identification function supports several access types:
//
//   - Basic Stream (0x01) -- reads the three mandatory objects (0x00-0x02)
//   - Regular Stream (0x02) -- reads mandatory + standard optional objects
//   - Extended Stream (0x03) -- reads all objects including vendor-specific
//   - Specific Object (0x04) -- reads a single object by its ID
//
// This example requests basic identification (the three mandatory objects)
// and displays all returned information. It also shows the device's
// conformity level, which indicates what identification categories it
// supports.
//
// Note: Not all Modbus devices implement FC 0x2B/0x0E. Simpler devices may
// only support data read/write function codes. If the device returns an
// exception, this example reports it clearly.
//
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags
	// -----------------------------------------------------------------------

	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")
	port := flag.Int("port", modbus.DefaultTCPPort, "Modbus TCP port")
	unit := flag.Int("unit", 1, "Modbus unit ID (slave address, 0-247)")

	flag.Parse()

	// -----------------------------------------------------------------------
	// Connect to the Modbus TCP server
	// -----------------------------------------------------------------------
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fmt.Printf("Connecting to Modbus TCP server at %s:%d (unit ID %d)...\n", *addr, *port, *unit)

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),
		modbus.WithTimeout(5*time.Second),
		modbus.WithUnitID(modbus.UnitID(*unit)),
		modbus.WithRetries(2),
		modbus.WithRetryDelay(500*time.Millisecond),
		modbus.WithLogger(logger),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Connected successfully.")
	fmt.Println()

	// -----------------------------------------------------------------------
	// Read Basic Device Identification (Objects 0x00, 0x01, 0x02)
	// -----------------------------------------------------------------------
	// We use ReadDeviceIDBasicStream (0x01) to request the three mandatory
	// identification objects. The request starts at DeviceIDVendorName (0x00)
	// and the server should return all basic objects in a single response.
	//
	// The Modbus Read Device Identification request contains:
	//   - Function Code: 0x2B (MEI Transport)
	//   - MEI Type: 0x0E (Read Device Identification)
	//   - Read Device ID Code: 0x01 (Basic Stream)
	//   - Object ID: 0x00 (starting object)
	//
	// The response includes:
	//   - Conformity Level: what identification the device supports
	//   - More Follows: whether additional objects are available
	//   - Next Object ID: which object to request next (if more follows)
	//   - Object list: one or more {ID, Length, Value} tuples
	fmt.Println("--- Reading Device Identification (FC 0x2B/0x0E) ---")
	fmt.Println()
	fmt.Println("Requesting basic identification (mandatory objects)...")
	fmt.Println()

	idCtx, idCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer idCancel()

	deviceID, err := client.ReadDeviceIdentification(
		idCtx,
		modbus.ReadDeviceIDBasicStream, // Request type: basic stream (0x01)
		modbus.DeviceIDVendorName,      // Start from the first object (0x00)
	)
	if err != nil {
		handleError("ReadDeviceIdentification", err)
		os.Exit(1)
	}

	// -----------------------------------------------------------------------
	// Display the response metadata
	// -----------------------------------------------------------------------
	// The conformity level tells us what categories of identification the
	// device supports. A device at "Basic" level only has the three mandatory
	// objects. "Regular" adds standard optional objects (URL, product name,
	// model name, user app name). "Extended" adds vendor-specific objects.
	fmt.Println("  Response Metadata:")
	fmt.Printf("    Conformity Level: %s\n", deviceID.ConformityLevel)
	fmt.Printf("    More Follows:     %s\n", deviceID.MoreFollows)
	fmt.Printf("    Next Object ID:   0x%02X\n", deviceID.NextObjectID)
	fmt.Printf("    Number of Objects: %d\n", deviceID.NumberOfObjects)
	fmt.Println()

	// -----------------------------------------------------------------------
	// Display the identification objects
	// -----------------------------------------------------------------------
	// Each object has an ID, a length, and a string value. The three
	// mandatory basic objects are:
	//
	//   0x00 - VendorName: the manufacturer of the device
	//   0x01 - ProductCode: a unique product identifier
	//   0x02 - MajorMinorRevision: the firmware/software version
	fmt.Println("  Device Identification Objects:")
	fmt.Printf("    %-6s %-25s %s\n", "ID", "Name", "Value")
	fmt.Printf("    %-6s %-25s %s\n", "------", "-------------------------", "-----")

	for _, obj := range deviceID.Objects {
		fmt.Printf("    0x%02X   %-25s %s\n", byte(obj.ID), obj.ID.String(), obj.Value)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Display individual fields using convenience methods
	// -----------------------------------------------------------------------
	// The DeviceIdentification type provides helper methods for the three
	// mandatory objects, which is convenient for programmatic access.
	fmt.Println("  Parsed Fields:")
	fmt.Printf("    Vendor Name: %s\n", deviceID.GetVendorName())
	fmt.Printf("    Product Code: %s\n", deviceID.GetProductCode())
	fmt.Printf("    Revision: %s\n", deviceID.GetRevision())

	// Also check for optional regular identification fields that some
	// devices provide even in a basic stream response.
	if url := deviceID.GetVendorURL(); url != "" {
		fmt.Printf("    Vendor URL: %s\n", url)
	}
	if name := deviceID.GetProductName(); name != "" {
		fmt.Printf("    Product Name: %s\n", name)
	}
	if model := deviceID.GetModelName(); model != "" {
		fmt.Printf("    Model Name: %s\n", model)
	}
	if appName := deviceID.GetUserApplicationName(); appName != "" {
		fmt.Printf("    User App Name: %s\n", appName)
	}
	fmt.Println()

	// -----------------------------------------------------------------------
	// Handle "More Follows" for devices with many objects
	// -----------------------------------------------------------------------
	// If the device has more objects than fit in one response, the
	// MoreFollows field is set to 0xFF and NextObjectID indicates where
	// to resume. In practice this is rare for basic identification but
	// can occur with extended/vendor-specific objects.
	if deviceID.MoreFollows == modbus.MoreFollowsYes {
		fmt.Printf("  Note: The device has additional identification objects.\n")
		fmt.Printf("  Request again with object ID 0x%02X to continue.\n", deviceID.NextObjectID)
		fmt.Println()

		// Fetch the next batch of objects.
		fmt.Println("  Fetching additional objects...")
		moreCtx, moreCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer moreCancel()

		moreID, err := client.ReadDeviceIdentification(
			moreCtx,
			modbus.ReadDeviceIDBasicStream,
			deviceID.NextObjectID,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not fetch additional objects: %v\n", err)
		} else {
			for _, obj := range moreID.Objects {
				fmt.Printf("    0x%02X   %-25s %s\n", byte(obj.ID), obj.ID.String(), obj.Value)
			}
			fmt.Println()
		}
	}

	fmt.Println("Done.")
}

// handleError prints a human-readable error message with Modbus-specific
// context for device identification errors.
func handleError(operation string, err error) {
	if modbus.IsModbusError(err) {
		fmt.Fprintf(os.Stderr, "  %s: Modbus exception: %v\n", operation, err)

		if modbus.IsExceptionError(err, modbus.ExceptionFunctionCodeNotSupported) {
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "  This device does not support Read Device Identification (FC 0x2B/0x0E).\n")
			fmt.Fprintf(os.Stderr, "  This is common with simpler Modbus devices that only implement basic\n")
			fmt.Fprintf(os.Stderr, "  read/write function codes (FC 0x01-0x10).\n")
			fmt.Fprintf(os.Stderr, "\n")
			fmt.Fprintf(os.Stderr, "  Device identification is part of the Modbus Encapsulated Interface (MEI),\n")
			fmt.Fprintf(os.Stderr, "  which is optional per the Modbus specification.\n")
		}
		if modbus.IsExceptionError(err, modbus.ExceptionInvalidDataValue) {
			fmt.Fprintf(os.Stderr, "  Hint: The requested Read Device ID code or object ID is not supported.\n")
		}
	} else {
		fmt.Fprintf(os.Stderr, "  %s: error: %v\n", operation, err)
	}
}
