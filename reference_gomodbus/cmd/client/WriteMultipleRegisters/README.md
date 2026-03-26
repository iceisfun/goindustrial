# Write Multiple Registers Example

This example demonstrates how to write multiple registers to a Modbus server.

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

Function code: 0x10 (Write Multiple Registers)

This function writes a block of holding registers in the server device.

## Example Code

```go
// Create values to write
registerValues := []common.RegisterValue{
    1000,  // First register
    2000,  // Second register
    3000,  // Third register
    4000,  // Fourth register
    5000,  // Fifth register
}

// Write multiple registers starting at address 0
err := modbusClient.WriteMultipleRegisters(ctx, common.Address(0), registerValues)
if err != nil {
    fmt.Println("Failed to write registers:", err)
    return
}

fmt.Printf("Successfully wrote %d registers\n", len(registerValues))
```