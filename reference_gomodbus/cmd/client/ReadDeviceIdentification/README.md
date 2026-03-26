# Read Device Identification Example

This example demonstrates how to read device identification information from a Modbus server.

## Usage

```
go run main.go --ip=192.168.1.100 --port=502
```

### Command-line Arguments

- `--ip`: Modbus server IP address (default: 127.0.0.1)
- `--port`: Modbus server port (default: 502)
- `--unit`: Modbus unit ID (slave ID) (default: 1)
- `--timeout`: Timeout for Modbus operations (default: 5s)
- `--log`: Log level (debug, info, warn, error) (default: info)

## Function Details

Function code: 0x2B (with MEI Type 0x0E)

This function reads identification information from a Modbus device. The information is organized in objects that each contain a specific piece of information about the device.

## Device Identification Types

The library supports different access types:

- `ReadDeviceIDBasicStream` (0x01): Reads the basic device identification (objects 0x00-0x02)
- `ReadDeviceIDRegularStream` (0x02): Reads regular device identification (objects 0x00-0x06)
- `ReadDeviceIDExtendedStream` (0x03): Reads all device identification objects
- `ReadDeviceIDSpecificObject` (0x04): Reads a specific identification object

## Identification Objects

Standard objects include:

- 0x00: VendorName
- 0x01: ProductCode
- 0x02: MajorMinorRevision
- 0x03: VendorURL
- 0x04: ProductName
- 0x05: ModelName
- 0x06: UserApplicationName

Extended objects (vendor-specific) start at 0x80.

## Example Code

```go
// Read basic device identification (objects 0x00-0x02)
identity, err := modbusClient.ReadDeviceIdentification(
    ctx, common.ReadDeviceIDBasicStream, common.DeviceIDObjectCode(0))
if err != nil {
    // Check if the function is not supported by the device
    if common.IsFunctionNotSupportedError(err) {
        fmt.Println("Device identification not supported")
        return
    }
    fmt.Println("Error:", err)
    return
}

// Access the basic information
fmt.Printf("Vendor Name: %s\n", identity.GetVendorName())
fmt.Printf("Product Code: %s\n", identity.GetProductCode())
fmt.Printf("Revision: %s\n", identity.GetRevision())
```