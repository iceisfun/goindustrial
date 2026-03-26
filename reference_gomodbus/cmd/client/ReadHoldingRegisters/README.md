# Read Holding Registers Example

This example demonstrates how to read holding registers from a Modbus server.

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

Function code: 0x03 (Read Holding Registers)

This function reads the contents of holding registers in the server device. Holding registers are read/write registers used to store and retrieve data from the server device.

## Example Code

```go
// Read 10 holding registers starting at address 0
registers, err := modbusClient.ReadHoldingRegisters(ctx, common.Address(0), common.Quantity(10))
if err != nil {
    fmt.Println("Failed to read holding registers:", err)
    return
}

// Print the results
for i, value := range registers {
    fmt.Printf("Register %d: %d (0x%04X)\n", i, value, value)
}
```