# Read Coils Example

This example demonstrates how to read coils from a Modbus server.

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

Function code: 0x01 (Read Coils)

This function reads the ON/OFF status of discrete outputs (coils) in the server device.

## Example Code

```go
// Read 10 coils starting at address 0
coils, err := modbusClient.ReadCoils(ctx, 0, 10)
if err != nil {
    fmt.Println("Failed to read coils:", err)
    return
}

// Print the results
for i, value := range coils {
    fmt.Printf("Coil %d: %t\n", i, value)
}
```