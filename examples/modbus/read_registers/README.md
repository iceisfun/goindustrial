# Read Registers Example

Demonstrates reading holding registers (FC 0x03) and input registers (FC 0x04) from a Modbus TCP server using the `goindustrial` library.

## What It Does

This example connects to a Modbus TCP server and performs three read operations:

1. **Single holding register read** -- Reads one 16-bit register at the specified address.
2. **Multiple holding register read** -- Reads a contiguous block of registers in a single request, which is more efficient than individual reads.
3. **Input register read** -- Reads read-only input registers using a different function code (FC 0x04 vs FC 0x03).

All values are displayed in both decimal and hexadecimal formats.

## Modbus Concepts

### Holding Registers (FC 0x03)

Holding registers are 16-bit read/write registers. They are the most commonly used data type in Modbus and typically store configuration values, setpoints, and control parameters. Function code 0x03 (`ReadHoldingRegisters`) is defined in Section 6.3 of the Modbus Application Protocol specification.

### Input Registers (FC 0x04)

Input registers are 16-bit read-only registers. They typically represent measured or computed values such as sensor readings, counters, or diagnostic data. Function code 0x04 (`ReadInputRegisters`) is defined in Section 6.4.

### Addressing

Modbus uses zero-based addressing internally (0-65535). Some device documentation uses a 1-based convention where holding registers start at 40001 and input registers start at 30001. This example uses the zero-based protocol addresses directly.

### Register Count Limits

The Modbus specification limits a single read request to 125 registers (due to the 253-byte PDU size constraint). To read more than 125 registers, issue multiple requests.

## How to Run

```bash
# Read 10 holding registers starting at address 0 (default server on localhost:502)
go run ./examples/modbus/read_registers/

# Read from a specific server
go run ./examples/modbus/read_registers/ -addr 192.168.1.100 -port 502

# Read 5 registers starting at address 100, unit ID 2
go run ./examples/modbus/read_registers/ -addr 192.168.1.100 -address 100 -count 5 -unit 2

# Read from a simulator running on a non-standard port
go run ./examples/modbus/read_registers/ -addr 127.0.0.1 -port 5020 -address 0 -count 20
```

### Flags

| Flag       | Default       | Description                              |
|------------|---------------|------------------------------------------|
| `-addr`    | `127.0.0.1`   | Modbus TCP server address                |
| `-port`    | `502`         | Modbus TCP port                          |
| `-unit`    | `1`           | Modbus unit ID (slave address, 0-247)    |
| `-address` | `0`           | Starting register address (0-65535)      |
| `-count`   | `10`          | Number of registers to read (1-125)      |

## Expected Output

```
Connecting to Modbus TCP server at 192.168.1.100:502 (unit ID 1)...
Connected successfully.

--- Reading single holding register at address 0 ---
  Address 0 = 1234 (0x04D2)

--- Reading 10 holding registers starting at address 0 ---
  Address    Decimal    Hex
  -------    -------    ------
  0          1234       0x04D2
  1          5678       0x162E
  2          0          0x0000
  3          65535      0xFFFF
  4          100        0x0064
  5          200        0x00C8
  6          300        0x012C
  7          400        0x0190
  8          500        0x01F4
  9          600        0x0258

--- Reading 10 input registers starting at address 0 ---
  Address    Decimal    Hex
  -------    -------    ------
  0          2500       0x09C4
  1          3300       0x0CE4
  ...

Done.
```

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `connection refused` | No Modbus server listening on the specified host:port | Verify the server address and port; ensure the server is running |
| `i/o timeout` | Server is unreachable | Check network connectivity, firewall rules, and IP address |
| `Modbus exception: Data Address Not Available (0x02)` | Requested address range does not exist on the device | Consult the device's register map and use valid addresses |
| `Modbus exception: Function Code Not Supported (0x01)` | Device does not support FC 0x04 (input registers) | Some devices only use holding registers; try FC 0x03 instead |
| `Modbus exception: Server Device Failure (0x04)` | Internal error on the Modbus server | Check the server's diagnostic logs |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.3 -- Read Holding Registers (FC 0x03)
- Modbus Application Protocol V1.1b3, Section 6.4 -- Read Input Registers (FC 0x04)
- Modbus Application Protocol V1.1b3, Section 4.3 -- MODBUS Data Model
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
