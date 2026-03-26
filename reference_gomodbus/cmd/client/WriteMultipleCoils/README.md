# Write Multiple Coils Example

This example demonstrates how to write multiple coils to a Modbus server.

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

Function code: 0x0F (Write Multiple Coils)

This function writes a series of coils (discrete outputs) to either ON or OFF in the server device.

## Example Code

```go
// Create a pattern of coil values to write
coilValues := []common.CoilValue{
    true,   // First coil ON
    false,  // Second coil OFF
    true,   // Third coil ON
    true,   // Fourth coil ON
    false,  // Fifth coil OFF
}

// Write multiple coils starting at address 0
err := modbusClient.WriteMultipleCoils(ctx, common.Address(0), coilValues)
if err != nil {
    fmt.Println("Failed to write coils:", err)
    return
}

fmt.Printf("Successfully wrote %d coils\n", len(coilValues))
```