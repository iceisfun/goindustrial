# EtherNet/IP I/O Scanner (Implicit Messaging Client)

Demonstrates an EtherNet/IP scanner (originator) that sends Forward_Open to a target device, establishing a cyclic implicit I/O connection over UDP. The scanner sends output assembly data and receives input assembly data at the negotiated RPI (Requested Packet Interval).

## What This Example Does

1. Opens a **TCP session** to the target device and registers an EIP session
2. Reads the **Identity object** to verify connectivity
3. Creates an **IOScanner** with a local UDP socket
4. Sends **Forward_Open** with the configured assembly instances, sizes, and RPI
5. Displays the **T→O (input) assembly** data at half-second intervals
6. Sends **Forward_Close** on shutdown to cleanly release the connection

## Architecture

```
This example (Scanner)
    │
    ├── TCP :44818 ── RegisterSession + Forward_Open ──→  Target (PLC / adapter)
    │
    ├── UDP :local ──── O→T data ────→  target consume assembly
    │
    └── UDP :local ◄──── T→O data ────  target produce assembly
```

## Running the Example

Against a real PLC (you must know the correct assembly instances):

```bash
go run ./examples/ethernetip/io_scanner/ \
  -addr 192.168.1.20 \
  -ot-instance 150 -to-instance 100 -config-instance 151 \
  -ot-size 8 -to-size 8
```

Against the adapter example on localhost:

```bash
# Terminal 1: Start the adapter
go run ./examples/ethernetip/adapter/ -ot-size 4 -to-size 4

# Terminal 2: Connect with the scanner
go run ./examples/ethernetip/io_scanner/ \
  -addr 127.0.0.1 \
  -ot-instance 100 -to-instance 101 -config-instance 0 \
  -ot-size 4 -to-size 4 \
  -rpi 50ms -cycles 20
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | (required) | PLC address (host or host:port, default port 44818) |
| `-ot-instance` | `150` | O→T (output) assembly instance on the PLC |
| `-to-instance` | `100` | T→O (input) assembly instance on the PLC |
| `-config-instance` | `151` | Config assembly instance (0 to skip) |
| `-ot-size` | `8` | O→T assembly size in bytes |
| `-to-size` | `8` | T→O assembly size in bytes |
| `-rpi` | `10ms` | Requested Packet Interval |
| `-timeout-mult` | `3` | Timeout multiplier (timeout = RPI * 4 << mult) |
| `-cycles` | `0` | Number of display cycles (0 = run until Ctrl+C) |
| `-udp-port` | `2222` | Target UDP port on PLC |

## Assembly Instances

Assembly instances vary by PLC configuration. Common defaults for CompactLogix/ControlLogix:

| Assembly | Instance | Direction | Description |
|----------|----------|-----------|-------------|
| Input (T→O) | 100 (0x64) | PLC → Scanner | Produced by the PLC |
| Output (O→T) | 150 (0x96) | Scanner → PLC | Consumed by the PLC |
| Config | 151 (0x97) | — | Configuration data sent during Forward_Open |

Check the controller properties in Studio 5000 under the Ethernet module's connection parameters for the correct values.

## Expected Output

```
Connecting to 192.168.1.20...
TCP session registered.

Connection parameters:
  O→T assembly:  instance 150, 8 bytes
  T→O assembly:  instance 100, 8 bytes
  Config assembly: instance 151
  RPI:           10ms
  Timeout mult:  3 (timeout = 320ms)

Identity object read OK (56 bytes)

Sending Forward_Open...
Forward_Open succeeded!
  OT Connection ID: 0x00000001
  TO Connection ID: 0x80000002
  Actual RPI:       10ms

Cyclic I/O running. Displaying input assembly:
---
[   1] T→O: 00 00 00 00 00 00 00 00  (age:    8ms)
[   2] T→O: 01 00 00 00 00 00 00 00  (age:    5ms)
[   3] T→O: 02 00 00 00 00 00 00 00  (age:    7ms)

--- Shutting down ---
Sending Forward_Close...
Forward_Close succeeded.
Done.
```

## Troubleshooting

If Forward_Open fails, common causes include:

- **Wrong assembly instance numbers** — these vary per PLC program and must match the controller configuration
- **Wrong assembly sizes** — sizes must match exactly or the PLC will reject the connection
- **PLC not configured for I/O** — the controller must have an Ethernet module configured for implicit messaging
- **Connection already owned** — another scanner may already have an active connection to these assemblies
- **Firewall blocking UDP** — implicit I/O uses UDP, which may be blocked between subnets
