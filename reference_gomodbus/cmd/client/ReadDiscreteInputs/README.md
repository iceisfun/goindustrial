# Read Discrete Inputs Example

This example demonstrates how to read discrete inputs from a Modbus server.

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

Function code: 0x02 (Read Discrete Inputs)

This function reads the ON/OFF status of discrete inputs in the server device. Discrete inputs are physically read-only and cannot be altered by the program running in the server device.

## Example Code

```go
// Read 10 discrete inputs starting at address 0
inputs, err := modbusClient.ReadDiscreteInputs(ctx, common.Address(0), common.Quantity(10))
if err != nil {
    fmt.Println("Failed to read discrete inputs:", err)
    return
}

// Print the results
for i, value := range inputs {
    fmt.Printf("Input %d: %t\n", i, value)
}
```