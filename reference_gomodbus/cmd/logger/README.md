# Custom Logger Example

This example demonstrates how to implement a custom logger for the gomodbus library.

## Overview

The gomodbus library uses an interface-based design for logging, which allows you to integrate with any logging framework by implementing the `common.LoggerInterface` interface.

## Usage

```
go run main.go
```

## Implementation

The custom logger in this example:

1. Uses the standard Go `log` package with custom formatting
2. Implements the four required methods from the interface:
   - `Debug`
   - `Info`
   - `Warn`
   - `Error`
3. Respects the log level setting to filter log messages
4. Adds timestamps and microsecond precision to log entries

## Using with the Modbus Client

To use a custom logger with the Modbus client:

```go
// Create a custom logger
logger := NewCustomLogger(common.LevelDebug)

// Use it with the client
modbusClient := client.NewTCPClient(
    ip,
    transport.WithPort(port),
    transport.WithTransportLogger(logger),
).WithOptions(
    client.WithTCPLogger(logger),
)
```

The Modbus library will then use your custom logger for all log messages.