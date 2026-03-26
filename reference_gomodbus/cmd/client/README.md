# gomodbus Client Examples

This directory contains example code for each Modbus function supported by the gomodbus library.

## Examples

### Basic Functions

- [Read Coils](ReadCoils/) - Function code 0x01
- [Read Discrete Inputs](ReadDiscreteInputs/) - Function code 0x02
- [Read Holding Registers](ReadHoldingRegisters/) - Function code 0x03
- [Read Input Registers](ReadInputRegisters/) - Function code 0x04
- [Write Single Coil](WriteSingleCoil/) - Function code 0x05
- [Write Single Register](WriteSingleRegister/) - Function code 0x06
- [Read Exception Status](ReadExceptionStatus/) - Function code 0x07

### Advanced Functions

- [Write Multiple Coils](WriteMultipleCoils/) - Function code 0x0F
- [Write Multiple Registers](WriteMultipleRegisters/) - Function code 0x10
- [Read/Write Multiple Registers](ReadWriteMultipleRegisters/) - Function code 0x17
- [Read Device Identification](ReadDeviceIdentification/) - Function code 0x2B/0x0E

### Other Examples

- [Custom Logger](../logger/) - Implementing a custom logger for Modbus operations

## Common Arguments

All example programs use the `args` package to handle command-line arguments:

```go
// Parse command-line arguments
modbusArgs := args.ParseArgs()

// Create a Modbus client
modbusClient := modbusArgs.CreateClient()
```

### Available Arguments

- `--ip`: Modbus server IP address (default: 127.0.0.1)
- `--port`: Modbus server port (default: 502)
- `--unit`: Modbus unit ID (slave ID) (default: 1)
- `--timeout`: Timeout for Modbus operations (default: 5s)
- `--log`: Log level (debug, info, warn, error) (default: info)

## Running Examples

Each example can be run directly using:

```bash
cd ReadCoils
go run main.go --ip=192.168.1.100 --port=502
```

## Type Safety

This library uses semantic type aliases for better type safety and code clarity:

```go
type Address uint16          // Modbus address
type Quantity uint16         // Number of coils/registers to read/write
type CoilValue = bool        // Coil value
type RegisterValue = uint16  // Register value
```