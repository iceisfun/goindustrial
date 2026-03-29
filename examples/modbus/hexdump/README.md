# Hex Dump Example (Modbus TCP)

Demonstrates wire-level hex dump tracing for Modbus TCP traffic using `modbus.WithHexDump`. Every byte exchanged over the TCP connection is printed in traditional `hexdump -C` format, showing the complete MBAP frame including transaction IDs, protocol identifiers, and PDU data.

## What It Does

1. Connects to a Modbus TCP server with hex dump tracing enabled.
2. Reads 3 holding registers -- the request and response frames are hex-dumped.
3. Writes a single register -- the write request and acknowledgement are hex-dumped.
4. Optionally logs the hex dump to a file for offline analysis.

## How to Run

```bash
# Console output (hex dump to stdout)
go run ./examples/modbus/hexdump/ -addr 127.0.0.1 -port 5020

# Console + file logging
go run ./examples/modbus/hexdump/ -addr 127.0.0.1 -port 5020 -log trace.hex
```

### Flags

| Flag    | Default     | Description                                 |
|---------|-------------|---------------------------------------------|
| `-addr` | `127.0.0.1` | Modbus TCP server address                   |
| `-port` | `502`       | Modbus TCP port                             |
| `-unit` | `1`         | Modbus unit ID (slave address)              |
| `-log`  | _(none)_    | Also write hex dump to this file            |

## Expected Output

```
Connecting to Modbus TCP server at 127.0.0.1:5020...

--- Reading 3 holding registers at address 0 ---

>>> WRITE 12 bytes
00000000  00 00 00 00 00 06 01 03  00 00 00 03              |............    |
<<< READ 7 bytes
00000000  00 00 00 00 00 09 01                               |.......         |
<<< READ 8 bytes
00000000  03 06 00 01 00 02 00 03                            |........        |

--- Decoded values ---
  Register 0 = 1 (0x0001)
  Register 1 = 2 (0x0002)
  Register 2 = 3 (0x0003)

--- Writing register 100 = 0x1234 ---

>>> WRITE 12 bytes
00000000  00 01 00 00 00 06 01 06  00 64 12 34              |.........d.4    |
<<< READ 7 bytes
00000000  00 01 00 00 00 06 01                               |.......         |
<<< READ 5 bytes
00000000  06 00 64 12 34                                     |..d.4           |

Done.
```

## Hex Dump Format

Each dump block starts with a direction marker and byte count:

- `>>> WRITE N bytes` -- data sent to the server
- `<<< READ N bytes` -- data received from the server

Each data line shows:

```
OFFSET    HEX BYTES (8 + 8)                                  |ASCII COLUMN    |
00000000  xx xx xx xx xx xx xx xx  xx xx xx xx xx xx xx xx   |................|
```

Short final lines are space-padded so the ASCII column always aligns.

## File Logging

Use `-log trace.hex` to write the hex dump to a file alongside stdout. This uses `io.MultiWriter` to duplicate the output. The trace file is useful for:

- Comparing captures before and after a change
- Sharing with colleagues for debugging
- Archiving protocol exchanges for documentation
