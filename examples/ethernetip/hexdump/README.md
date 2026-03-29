# Hex Dump Example (EtherNet/IP)

Demonstrates wire-level hex dump tracing for EtherNet/IP traffic using `ethernetip.WithHexDump`. Every byte exchanged over the TCP connection is printed in traditional `hexdump -C` format, showing the complete EIP encapsulation frames including the 24-byte header, CPF items, and CIP payloads.

## What It Does

1. Connects to an EtherNet/IP device with hex dump tracing enabled. The RegisterSession handshake is captured automatically.
2. Reads a tag -- the SendRRData request and response frames are hex-dumped.
3. Optionally logs the hex dump to a file for offline analysis.

## How to Run

```bash
# Console output (hex dump to stdout)
go run ./examples/ethernetip/hexdump/ -addr 192.168.1.10:44818 -tag MyDINT

# Console + file logging
go run ./examples/ethernetip/hexdump/ -addr 192.168.1.10:44818 -tag MyDINT -log trace.hex
```

### Flags

| Flag    | Default                | Description                                 |
|---------|------------------------|---------------------------------------------|
| `-addr` | `192.168.1.10:44818`   | PLC address (host:port)                     |
| `-tag`  | `MyDINT`               | Tag name to read                            |
| `-log`  | _(none)_               | Also write hex dump to this file            |

## Expected Output

```
Connecting to 192.168.1.10:44818...

--- Reading tag ---

>>> WRITE 24 bytes
00000000  65 00 04 00 00 00 00 00  00 00 00 00 00 00 00 00 |e...............|
00000010  00 00 00 00 00 00 00 00                          |........        |
>>> WRITE 4 bytes
00000000  01 00 00 00                                      |....            |
<<< READ 24 bytes
00000000  65 00 04 00 01 00 00 00  00 00 00 00 00 00 00 00 |e...............|
00000010  00 00 00 00 00 00 00 00                          |........        |
<<< READ 4 bytes
00000000  01 00 00 00                                      |....            |
>>> WRITE 24 bytes
00000000  6f 00 1e 00 01 00 00 00  00 00 00 00 00 00 00 00 |o...............|
00000010  00 00 00 00 00 00 00 00                          |........        |
...

--- Decoded ---
  Tag:  MyDINT
  Data: C4 00 2A 00 00 00

Done.
```

## Hex Dump Format

Each dump block starts with a direction marker and byte count:

- `>>> WRITE N bytes` -- data sent to the PLC
- `<<< READ N bytes` -- data received from the PLC

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
