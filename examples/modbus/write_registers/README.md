# Write Registers Example

Demonstrates writing holding registers to a Modbus TCP server using single-register writes (FC 0x06) and multiple-register writes (FC 0x10), including the write-then-read-back verification pattern.

## What It Does

This example connects to a Modbus TCP server and performs one of two operations depending on the flags provided:

1. **Single register write (FC 0x06)** -- Writes one 16-bit value to a single register, then reads it back to verify.
2. **Multiple register write (FC 0x10)** -- Writes a block of 16-bit values to contiguous registers, then reads them all back to verify.

Both modes follow the write-then-read-back pattern: read the current value, write the new value, then read back to confirm the device accepted it.

## Modbus Concepts

### Write Single Register (FC 0x06)

Function code 0x06 writes a single 16-bit register. The request contains the register address and the new value. The normal response is an exact echo of the request PDU, serving as a built-in acknowledgment. Defined in Section 6.6 of the Modbus specification.

### Write Multiple Registers (FC 0x10)

Function code 0x10 writes a contiguous block of registers in a single request. This is the preferred method when updating multiple related values that should change together (e.g., a setpoint and its limits). The specification allows up to 123 registers per request. Defined in Section 6.12.

### Write-Then-Read-Back Pattern

In safety-critical and industrial applications, it is common practice to read back written values to verify the device accepted them. Devices may clamp values to valid ranges, reject writes to read-only addresses, or apply scaling. The read-back confirms the actual device state.

## How to Run

```bash
# Write a single value (1234) to register 0
go run ./examples/modbus/write_registers/ -addr 127.0.0.1 -port 502 -address 0 -value 1234

# Write multiple values to registers starting at address 10
go run ./examples/modbus/write_registers/ -addr 127.0.0.1 -port 502 -address 10 -values "100,200,300,400,500"

# Write to a specific unit/slave
go run ./examples/modbus/write_registers/ -addr 192.168.1.100 -unit 2 -address 0 -value 42

# Write to a simulator on a non-standard port
go run ./examples/modbus/write_registers/ -addr 127.0.0.1 -port 5020 -address 0 -values "1000,2000,3000"
```

### Flags

| Flag       | Default     | Description                                          |
|------------|-------------|------------------------------------------------------|
| `-addr`    | `127.0.0.1` | Modbus TCP server address                            |
| `-port`    | `502`       | Modbus TCP port                                      |
| `-unit`    | `1`         | Modbus unit ID (slave address, 0-247)                |
| `-address` | `0`         | Target register address (0-65535)                    |
| `-value`   | *(none)*    | Single register value to write (0-65535)             |
| `-values`  | *(none)*    | Comma-separated values for multi-write (e.g., "100,200,300") |

You must provide either `-value` or `-values` (not both).

## Expected Output

### Single Register Write

```
Connecting to Modbus TCP server at 127.0.0.1:502 (unit ID 1)...
Connected successfully.

--- Writing single register (FC 0x06) ---
  Address: 0
  Value:   1234 (0x04D2)

  Reading current value before write...
  Current value: 0 (0x0000)

  Writing value 1234 to register 0...
  Write successful.

  Reading back to verify...
  Read-back value: 1234 (0x04D2)
  Verification: PASS -- read-back matches written value.

Done.
```

### Multiple Register Write

```
--- Writing 3 registers (FC 0x10) ---
  Starting address: 10
  Values: 100, 200, 300

  Reading current values before write...
  Address    Current Value
  -------    -------------
  10         0          (0x0000)
  11         0          (0x0000)
  12         0          (0x0000)

  Writing 3 registers starting at address 10...
  Write successful.

  Reading back to verify...
  Address    Written         Read Back       Match
  -------    -------         ---------       -----
  10         100             100             OK
  11         200             200             OK
  12         300             300             OK

  Verification: PASS -- all values match.

Done.
```

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `connection refused` | No Modbus server listening on the specified host:port | Verify the server address and port |
| `Modbus exception: Data Address Not Available (0x02)` | Register address is not writable or does not exist | Check the device's register map for writable addresses |
| `Modbus exception: Invalid Data Value (0x03)` | Value is outside the device's acceptable range | Consult device documentation for valid value ranges |
| `Verification: MISMATCH` | Device clamped or transformed the written value | This is not necessarily an error; some devices apply scaling or limits |
| `invalid value at position N` | Non-numeric or out-of-range entry in -values | Values must be integers 0-65535, separated by commas |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.6 -- Write Single Register (FC 0x06)
- Modbus Application Protocol V1.1b3, Section 6.12 -- Write Multiple Registers (FC 0x10)
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
