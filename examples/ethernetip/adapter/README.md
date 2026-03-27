# EtherNet/IP Adapter (Implicit I/O Server)

Demonstrates an EtherNet/IP adapter (target device) that accepts implicit I/O connections from a scanner via Forward_Open and exchanges cyclic assembly data over UDP.

## What This Example Does

1. Creates **assembly instances** for I/O buffers (consume and produce)
2. Starts a **UDP runtime** for implicit I/O packet handling
3. Starts a **scheduler** that sends produce data at the negotiated RPI
4. Wires the **Connection Manager** so Forward_Open automatically creates I/O connections in the runtime
5. Starts the **EIP server** on TCP for session management and CIP routing
6. Runs an **application loop** that reads consumed data and echoes it back with a cycle counter

## Architecture

```
Scanner (PLC / io_scanner example)
    │
    ├── TCP :44818 ── Forward_Open ──→  Server → ConnMgr → OnOpen callback
    │                                                          │
    │                                                   ┌──────┴──────┐
    │                                                   │   Runtime   │
    ├── UDP :2222 ──── O→T data ────→  consume assembly (instance 100)
    │                                                   │
    │                  Application loop reads 100, writes 101
    │                                                   │
    └── UDP :2222 ◄──── T→O data ────  produce assembly (instance 101)
                                                        │
                                                   Scheduler (5ms tick)
```

## Running the Example

Start the adapter:

```bash
go run ./examples/ethernetip/adapter/
```

With custom assembly sizes:

```bash
go run ./examples/ethernetip/adapter/ -ot-size 4 -to-size 4
```

Custom ports:

```bash
go run ./examples/ethernetip/adapter/ -tcp :44818 -udp :2222
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-tcp` | `:44818` | TCP listen address for EIP sessions |
| `-udp` | `:2222` | UDP listen address for implicit I/O |
| `-ot-size` | `12` | O→T (consume) assembly size in bytes |
| `-to-size` | `12` | T→O (produce) assembly size in bytes |

## Testing with the IO Scanner Example

In one terminal, start the adapter:

```bash
go run ./examples/ethernetip/adapter/ -ot-size 4 -to-size 4
```

In another terminal, connect with the scanner:

```bash
go run ./examples/ethernetip/io_scanner/ \
  -addr 127.0.0.1 \
  -ot-instance 100 -to-instance 101 -config-instance 0 \
  -ot-size 4 -to-size 4 \
  -rpi 50ms -cycles 20
```

You can also probe the adapter first:

```bash
go run ./examples/ethernetip/probe/ 127.0.0.1
```

## How the Wiring Works

The key pattern is the Connection Manager's `OnOpen` callback. When a scanner sends Forward_Open, the callback:

1. **Extracts the RPI** from the Forward_Open request
2. **Creates a consumer IOConnection** keyed by `OTConnectionID` — this receives UDP packets from the scanner and writes to assembly instance 100
3. **Creates a producer IOConnection** keyed by `TOConnectionID` — the scheduler reads from assembly instance 101 and sends UDP packets to the scanner
4. **Registers both** with the runtime via `AddConnection`

On Forward_Close, the `OnClose` callback removes both connections from the runtime.

```go
cm := connmgr.NewConnectionManager(
    connmgr.WithOnOpen(func(c *connmgr.Connection, req *connmgr.ForwardOpenRequest) {
        rpi := time.Duration(req.OTRPI) * time.Microsecond

        rt.AddConnection(&runtime.IOConnection{
            ConnectionID: c.OTConnectionID,
            RPI:          rpi,
            Assembly:     ao.GetInstance(100),
            IsConsumer:   true,
        })

        rt.AddConnection(&runtime.IOConnection{
            ConnectionID: c.TOConnectionID,
            RPI:          rpi,
            Assembly:     ao.GetInstance(101),
            IsProducer:   true,
        })
    }),
    connmgr.WithOnClose(func(c *connmgr.Connection) {
        rt.RemoveConnection(c.OTConnectionID)
        rt.RemoveConnection(c.TOConnectionID)
    }),
)
```

## Expected Output

### Adapter

```
Assembly instances:
  Instance 100 (consume): 12 bytes
  Instance 101 (produce): 12 bytes
UDP runtime listening on [::]:2222
EIP server listening on :44818

Waiting for scanner connections (Forward_Open)...
Press Ctrl+C to stop.

[OPEN] Connection from scanner
  OT Connection ID: 0x00000001
  TO Connection ID: 0x80000002
  RPI:              50ms
  Serial:           0x0001

[cycle    10] consume=DE AD BE EF 00 00 00 00 00 00 00 00  produce=0A 00 DE AD BE EF 00 00 00 00 00 00
[cycle    20] consume=DE AD BE EF 00 00 00 00 00 00 00 00  produce=14 00 DE AD BE EF 00 00 00 00 00 00

[CLOSE] Connection 0x00000001 / 0x80000002

Shutting down...
Active connections: 0
Done.
```

### Scanner

```
Forward_Open succeeded!
  OT Connection ID: 0x00000001
  TO Connection ID: 0x80000002
  Actual RPI:       50ms

Cyclic I/O running. Displaying input assembly:
---
[   1] T→O: 01 00 00 00 00 00 00 00 00 00 00 00  (age:   48ms)
[   2] T→O: 02 00 00 00 00 00 00 00 00 00 00 00  (age:   45ms)
```
