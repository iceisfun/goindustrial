# Write Single Register Example

This example demonstrates how to write a single register to a Modbus server.

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

Function code: 0x06 (Write Single Register)

This function writes a single holding register in the server device.

## Example Code

```go
// Write a single register at address 0 with value 12345
err := modbusClient.WriteSingleRegister(ctx, common.Address(0), common.RegisterValue(12345))
if err != nil {
    fmt.Println("Failed to write register:", err)
    return
}

fmt.Printf("Successfully set register to 12345 (0x%04X)\n", 12345)
```