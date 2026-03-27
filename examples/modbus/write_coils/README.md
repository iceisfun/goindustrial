# Write Coils Example

Demonstrates writing coils to a Modbus TCP server using single-coil writes (FC 0x05) and multiple-coil writes (FC 0x0F), including write-then-read-back verification.

## What It Does

This example connects to a Modbus TCP server and performs one of two operations:

1. **Single coil write (FC 0x05)** -- Sets one coil to ON or OFF, then reads it back to verify.
2. **Multiple coil write (FC 0x0F)** -- Sets a block of coils to specified ON/OFF values, then reads them all back to verify.

## Modbus Concepts

### Write Single Coil (FC 0x05)

Function code 0x05 writes a single coil. The coil value is encoded on the wire as a 16-bit value: `0xFF00` for ON and `0x0000` for OFF. Any other value is illegal per the specification. The response is an exact echo of the request. Defined in Section 6.5.

### Write Multiple Coils (FC 0x0F)

Function code 0x0F writes multiple coils in a single request. The values are packed as bits within bytes (LSB of the first byte corresponds to the first coil), matching the bit-packing format used in Read Coils responses. The specification allows up to 1968 coils per request. Defined in Section 6.11.

### Coil Wire Encoding

For FC 0x05 (single coil), the value is sent as `0xFF00` (ON) or `0x0000` (OFF) -- a full 16-bit register value. For FC 0x0F (multiple coils), values are bit-packed: 8 coils per byte, with padding zeros in the last byte if the count is not a multiple of 8.

## How to Run

```bash
# Turn ON coil 0
go run ./examples/modbus/write_coils/ -addr 127.0.0.1 -port 502 -address 0 -value on

# Turn OFF coil 5
go run ./examples/modbus/write_coils/ -addr 127.0.0.1 -port 502 -address 5 -value off

# Write multiple coils: address 0=ON, 1=OFF, 2=ON, 3=ON, 4=OFF
go run ./examples/modbus/write_coils/ -addr 127.0.0.1 -port 502 -address 0 -values "1,0,1,1,0"

# Write to a simulator on a non-standard port
go run ./examples/modbus/write_coils/ -addr 127.0.0.1 -port 5020 -address 0 -values "on,off,on,off"

# Write to unit 3
go run ./examples/modbus/write_coils/ -addr 192.168.1.100 -unit 3 -address 10 -value on
```

### Flags

| Flag       | Default     | Description                                                    |
|------------|-------------|----------------------------------------------------------------|
| `-addr`    | `127.0.0.1` | Modbus TCP server address                                      |
| `-port`    | `502`       | Modbus TCP port                                                |
| `-unit`    | `1`         | Modbus unit ID (slave address, 0-247)                          |
| `-address` | `0`         | Target coil address (0-65535)                                  |
| `-value`   | *(none)*    | Single coil value: "on"/"off", "true"/"false", or "1"/"0"     |
| `-values`  | *(none)*    | Comma-separated coil values for multi-write (e.g., "1,0,1,1,0") |

You must provide either `-value` or `-values` (not both).

## Expected Output

### Single Coil Write

```
Connecting to Modbus TCP server at 127.0.0.1:502 (unit ID 1)...
Connected successfully.

--- Writing single coil (FC 0x05) ---
  Address: 0
  Value:   ON

  Reading current coil state before write...
  Current state: OFF

  Writing coil 0 to ON...
  Write successful.

  Reading back to verify...
  Read-back state: ON
  Verification: PASS -- read-back matches written value.

Done.
```

### Multiple Coil Write

```
--- Writing 5 coils (FC 0x0F) ---
  Starting address: 0
  Values: 1,0,1,1,0

  Reading current coil states before write...
  Address    Current
  -------    -------
  0          OFF
  1          OFF
  2          OFF
  3          OFF
  4          OFF

  Writing 5 coils starting at address 0...
  Write successful.

  Reading back to verify...
  Address    Written    Read Back  Match
  -------    -------    ---------  -----
  0          ON         ON         OK
  1          OFF        OFF        OK
  2          ON         ON         OK
  3          ON         ON         OK
  4          OFF        OFF        OK

  Verification: PASS -- all coils match.

Done.
```

## Common Errors and Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `connection refused` | No Modbus server on the specified host:port | Verify server address and port |
| `Modbus exception: Data Address Not Available (0x02)` | Coil address does not exist or is not writable | Check device documentation for valid coil addresses |
| `Modbus exception: Invalid Data Value (0x03)` | Server rejected the coil value | Unlikely for boolean values; check device state |
| `Verification: MISMATCH` | Device did not accept the written value | Some coils may be hardware-locked or interlocked |
| `invalid coil value` | Unrecognized value string | Use "on"/"off", "1"/"0", or "true"/"false" |

## Specification References

- Modbus Application Protocol V1.1b3, Section 6.5 -- Write Single Coil (FC 0x05)
- Modbus Application Protocol V1.1b3, Section 6.11 -- Write Multiple Coils (FC 0x0F)
- Modbus Application Protocol V1.1b3, Section 7 -- Exception Responses
