# Read/Write Multiple Registers Example

This example demonstrates how to read and write multiple registers in a single Modbus transaction.

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

Function code: 0x17 (Read/Write Multiple Registers)

This function performs a combined read and write operation in a single Modbus transaction. It can read from one address range and write to another address range in a single operation.

## Example Code

```go
// Define parameters
readAddress := common.Address(10)   // Starting address for reading
readQuantity := common.Quantity(5)  // Number of registers to read
writeAddress := common.Address(20)  // Starting address for writing

// Values to write
writeValues := []common.RegisterValue{
    10000, 20000, 30000
}

// Perform the combined read/write operation
readValues, err := modbusClient.ReadWriteMultipleRegisters(
    ctx, readAddress, readQuantity, writeAddress, writeValues)
if err != nil {
    fmt.Println("Failed to perform read/write operation:", err)
    return
}

// Display the read values
for i, value := range readValues {
    fmt.Printf("Read register %d: %d\n", int(readAddress)+i, value)
}
```