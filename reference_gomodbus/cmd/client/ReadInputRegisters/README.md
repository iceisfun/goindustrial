# Read Input Registers Example

This example demonstrates how to read input registers from a Modbus server.

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

Function code: 0x04 (Read Input Registers)

This function reads the contents of input registers in the server device. Input registers are read-only registers used to store data from I/O systems and sensors.

## Example Code

```go
// Read 10 input registers starting at address 0
registers, err := modbusClient.ReadInputRegisters(ctx, common.Address(0), common.Quantity(10))
if err != nil {
    fmt.Println("Failed to read input registers:", err)
    return
}

// Print the results
for i, value := range registers {
    fmt.Printf("Register %d: %d (0x%04X)\n", i, value, value)
}
```