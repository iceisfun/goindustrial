# Read Coils Example

Demonstrates reading coils (FC 0x01) and discrete inputs (FC 0x02) from a Modbus TCP server, with bit-level display of the results.

## What It Does

This example connects to a Modbus TCP server and reads boolean (single-bit) values. It displays the results in three formats:

1. **Individual listing** -- Each coil/input address with its ON/OFF state.
2. **Bit-level wire view** -- Values packed into bytes the way they appear on the wire, with hex byte values.
3. **Summary** -- Count of ON vs total values.

Use the `-discrete` flag to switch between reading coils (FC 0x01) and discrete inputs (FC 0x02).

## Modbus Concepts

### Coils (FC 0x01)

Coils are read/write single-bit values. The name "coil" comes from the relay coils in early industrial control systems. In modern usage, coils represent any binary output: relays, solenoids, motor starters, indicator lights, etc. Function code 0x01 (`ReadCoils`) is defined in Section 6.1 of the Modbus specification.

### Discrete Inputs (FC 0x02)

Discrete inputs are read-only single-bit values representing physical input devices: push buttons, limit switches, proximity sensors, etc. Function code 0x02 (`ReadDiscreteInputs`) is defined in Section 6.2. The wire format is identical to Read Coils.

### Bit Packing

On the wire, boolean values are packed as bits within bytes. The first coil requested corresponds to the least significant bit (bit 0) of the first data byte. If the number of coils is not a multiple of 8, the remaining bits in the last byte are padded with zeros. For example, reading 10 coils produces 2 bytes, with bits 0-7 in byte 0 and bits 8-9 in the low bits of byte 1.

### Count Limits

The Modbus specification allows reading up to 2000 coils or discrete inputs in a single request.

## How to Run

```bash
# Read 16 coils starting at address 0
go run ./examples/modbus/read_coils/ -addr 127.0.0.1 -port 502

# Read 32 coils from a remote server
go run ./examples/modbus/read_coils/ -addr 192.168.1.100 -count 32

# Read discrete inputs instead of coils
go run ./examples/modbus/read_coils/ -addr 127.0.0.1 -port 502 -discrete

# Read 100 coils starting at address 1000
go run ./examples/modbus/read_coils/ -addr 127.0.0.1 -port 5020 -address 1000 -count 100

# Read discrete inputs from unit 3
go run ./examples/modbus/read_coils/ -addr 192.168.1.100 -unit 3 -discrete -count 8
```

### Flags

| Flag        | Default     | Description                                           |
|-------------|-------------|-------------------------------------------------------|
| `-addr`     | `127.0.0.1` | Modbus TCP server address                             |
| `-port`     | `502`       | Modbus TCP port                                       |
| `-unit`     | `1`         | Modbus unit ID (slave address, 0-247)                 |
| `-address`  | `0`         | Starting coil/discrete input address (0-65535)        |
| `-count`    | `16`        | Number of coils/discrete inputs to read (1-2000)      |
| `-discrete` | `false`     | Read discrete inputs (FC 0x02) instead of coils (FC 0x01) |

## Expected Output

```
Connecting to Modbus TCP server at 127.0.0.1:502 (unit ID 1)...
Connected successfully.

--- Reading 16 coils starting at address 0 (FC 0x01) ---

  Address    State    Bit
  -------    -----    ---
  0          ON       1
  1          OFF      0
  2          ON       1
  3          ON       1
  4          OFF      0
  5          OFF      0
  6          OFF      0
  7          ON       1
  8          OFF      0
  9          ON       1
  10         OFF      0
  11         OFF      0
  12         ON       1
  13         OFF      0
  14         OFF      0
  15         OFF      0

  Bit-level view (wire format, LSB first within each byte):

  Byte  0 (addr 0-7):  10001101 = 0x8D
  Byte  1 (addr 8-15): 00010010 = 0x12

  Summary: 6 of 16 Coils are ON

Done.
```

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `connection refused` | No Modbus server on the specified host:port | Verify server address and port |
| `Modbus exception: Data Address Not Available (0x02)` | Requested coil/input range does not exist | Use valid addresses from the device register map |
| `Modbus exception: Function Code Not Supported (0x01)` | Device does not implement FC 0x02 | Not all devices support discrete inputs; try reading coils instead |
| `count must be between 1 and 2000` | Invalid count value | Reduce the number of coils per request |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.1 -- Read Coils (FC 0x01)
- Modbus Application Protocol V1.1b3, Section 6.2 -- Read Discrete Inputs (FC 0x02)
- Modbus Application Protocol V1.1b3, Section 4.3 -- MODBUS Data Model
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
