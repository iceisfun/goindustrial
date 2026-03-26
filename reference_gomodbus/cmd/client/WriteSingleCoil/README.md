# Write Single Coil Example

This example demonstrates how to write a single coil to a Modbus server.

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

Function code: 0x05 (Write Single Coil)

This function writes a single coil (discrete output) to either ON or OFF in the server device.

## Example Code

```go
// Write a single coil at address 0 with value ON (true)
err := modbusClient.WriteSingleCoil(ctx, common.Address(0), common.CoilValue(true))
if err != nil {
    fmt.Println("Failed to write coil:", err)
    return
}

fmt.Println("Successfully set coil to ON")
```