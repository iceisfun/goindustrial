# Read Exception Status Example

This example demonstrates how to read the exception status from a Modbus server.

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

Function code: 0x07 (Read Exception Status)

This function reads the contents of 8 exception status outputs in a server device. It provides a quick check of the server's exception status.

## Type Safety

This library uses a typed return value for better type safety:

```go
type ExceptionStatus byte
```

This type provides a helpful String() method for better debugging output:

```go
func (s ExceptionStatus) String() string {
    // Outputs something like: "ExceptionStatus(Bits: [0 5], Value: 0x21)"
}
```

## Example Code

```go
// Read the exception status
status, err := modbusClient.ReadExceptionStatus(ctx)
if err != nil {
    fmt.Println("Failed to read exception status:", err)
    return
}

// Use the String() method for display
fmt.Printf("Exception Status: %s\n", status)

// Check individual bits
for i := 0; i < 8; i++ {
    if status&(1<<i) != 0 {
        fmt.Printf("Exception bit %d is set\n", i)
    }
}
```