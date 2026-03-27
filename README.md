# goindustrial

A unified Go library for industrial communication protocols. Consolidates **Modbus TCP** and **EtherNet/IP (CIP)** under common abstractions for transport, logging, monitoring, and PLC access.

Zero external dependencies. Pure Go. Go 1.25+.

## Package Structure

```
goindustrial/
    logging/                     Unified structured logging
    transport/                   Generic transport lifecycle (reconnect, retry)
    plc/                         Protocol-agnostic PLC interface
    monitor/                     Polling engine with change detection

    protocol/modbus/             Modbus TCP client, server, and protocol
    protocol/ethernetip/         EtherNet/IP client, server, and protocol
    protocol/ethernetip/cip/     Common Industrial Protocol
    protocol/ethernetip/eip/     EIP encapsulation layer
    protocol/ethernetip/objects/ CIP objects (Assembly, Connection Manager)
    protocol/ethernetip/runtime/ UDP I/O for implicit messaging

    examples/                    Runnable examples with READMEs (see below)
```

## Quick Start

### Modbus TCP

```go
ctx := context.Background()

// Connect to a Modbus TCP device
client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithUnitID(1),
    modbus.WithRetries(3),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Read holding registers
regs, err := client.ReadHoldingRegisters(ctx, 0, 10)

// Write a register
err = client.WriteSingleRegister(ctx, 100, 0x1234)

// Write coils
err = client.WriteMultipleCoils(ctx, 0, []bool{true, false, true})
```

### EtherNet/IP (CIP)

```go
ctx := context.Background()

// Connect to a Logix PLC
client, err := ethernetip.Connect(ctx, "192.168.1.20",
    ethernetip.WithRetries(3),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Read a tag
data, err := client.ReadTag(ctx, "MyDINT")

// Typed read
val, err := ethernetip.Read[int32](client, ctx, "MyDINT")

// Write a tag
err = client.WriteTag(ctx, "MyFloat", float32(3.14))

// Read a timer
timer, err := client.ReadTimer(ctx, "MyTimer")
// timer.PRE, timer.ACC, timer.EN, timer.TT, timer.DN
```

### Cross-Protocol Monitoring

```go
// Monitor can poll both Modbus and EtherNet/IP data points
m, _ := monitor.NewMonitor(myReader,
    monitor.WithEventBuffer(128),
)
defer m.Close()

m.Subscribe(modbus.HoldingRegister{Addr: 0, Qty: 10},
    monitor.WithFrequency(100*time.Millisecond),
    monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
)

m.Subscribe(ethernetip.Tag{Name: "Position", Elements: 1},
    monitor.WithFrequency(50*time.Millisecond),
)

for evt := range m.Events() {
    fmt.Printf("[%s] %s changed: %x\n",
        evt.Snapshot.Timestamp.Format(time.RFC3339),
        evt.Snapshot.Point,
        evt.Snapshot.Value.Raw,
    )
}
```

## Architecture

### Shared Infrastructure

- **`logging.Logger`** -- Context-aware, leveled, structured fields. Pluggable: supply your own or use the default.
- **`transport.Transport[C]`** -- Generic connection lifecycle with `DirectTransport` and `ReconnectingTransport` (RWMutex double-check locking, lifecycle hooks).
- **`plc.PLC`** -- Protocol-agnostic interface (`Reader`, `Writer`, `Connect`, `Disconnect`). Both protocol clients implement this.
- **`monitor.Monitor`** -- Subscription-per-goroutine polling engine with frequency control, read variance (jitter), change detection, and handler callbacks.

### Protocol Implementations

**Modbus TCP** (`protocol/modbus/`)
- All 11 function codes (read/write coils, registers, device identification)
- MBAP header framing with 65536-entry transaction pool
- `TCPConn` with concurrent read/write goroutines
- `Server` with `MemoryStore`, handler dispatch, client tracking

**EtherNet/IP** (`protocol/ethernetip/`)
- EIP session management (RegisterSession, SendRRData)
- CIP types, EPATH building, Marshal/Unmarshal
- ReadTag/WriteTag with typed generics (`Read[T]`, `ReadSlice[T]`)
- Timer and Counter struct decoding
- Server with CIP MessageRouter dispatch
- Assembly Object (Class 0x04) and Connection Manager (Class 0x06)
- UDP I/O runtime for implicit messaging

### Testing with net.Pipe

Both protocols support `net.Conn` injection for deterministic, in-process testing:

```go
serverConn, clientConn := net.Pipe()

// Modbus
srv := modbus.NewServer("", modbus.WithServerConn(serverConn))
conn := modbus.NewTCPConn("", modbus.WithConn(clientConn))

// EtherNet/IP
srv := ethernetip.NewServer(router)
go srv.HandleConn(serverConn)
conn, _ := ethernetip.NewTCPConn("", ethernetip.WithConn(clientConn))
```

## Examples

Every example is a standalone `main.go` with its own README explaining the relevant protocol concepts, how to run it, and expected output.

### Modbus TCP

| Example | Description |
|---------|-------------|
| [`modbus/read_registers`](examples/modbus/read_registers/) | Read holding registers (FC 0x03) and input registers (FC 0x04) |
| [`modbus/write_registers`](examples/modbus/write_registers/) | Write single (FC 0x06) and multiple registers (FC 0x10) with readback |
| [`modbus/read_coils`](examples/modbus/read_coils/) | Read coils (FC 0x01) and discrete inputs (FC 0x02) with bit display |
| [`modbus/write_coils`](examples/modbus/write_coils/) | Write single (FC 0x05) and multiple coils (FC 0x0F) with readback |
| [`modbus/read_write_registers`](examples/modbus/read_write_registers/) | Atomic read+write in one transaction (FC 0x17) |
| [`modbus/device_identification`](examples/modbus/device_identification/) | Read vendor info and product metadata (FC 0x2B/0x0E) |
| [`modbus/server`](examples/modbus/server/) | TCP server with data store, client tracking, graceful shutdown |
| [`modbus/reconnecting`](examples/modbus/reconnecting/) | Manual transport build, lifecycle hooks, error classification |
| [`modbus/all_data_types`](examples/modbus/all_data_types/) | All four data areas and every function code in one demo |

### EtherNet/IP (CIP)

| Example | Description |
|---------|-------------|
| [`ethernetip/read_tag`](examples/ethernetip/read_tag/) | Raw and typed tag reads with type code display |
| [`ethernetip/write_tag`](examples/ethernetip/write_tag/) | Write all CIP types (BOOL through STRING) with readback |
| [`ethernetip/read_tag_typed`](examples/ethernetip/read_tag_typed/) | Generic `Read[T]` and `ReadSlice[T]` for every Go type |
| [`ethernetip/timer_counter`](examples/ethernetip/timer_counter/) | Read Timer and Counter 14-byte structures |
| [`ethernetip/list_tags`](examples/ethernetip/list_tags/) | Enumerate all tags via CIP Symbol Object (Class 0x6B) |
| [`ethernetip/list_identity`](examples/ethernetip/list_identity/) | EIP ListIdentity and ListServices device discovery |
| [`ethernetip/server`](examples/ethernetip/server/) | CIP message router with custom tag object implementation |
| [`ethernetip/reconnecting`](examples/ethernetip/reconnecting/) | Manual transport build, CIP vs transport error handling |

### Cross-Protocol

| Example | Description |
|---------|-------------|
| [`monitor_polling`](examples/monitor_polling/) | Poll Modbus + EtherNet/IP through a unified Monitor with change detection |
| [`plc_interface`](examples/plc_interface/) | Protocol-agnostic code using the `plc.PLC` interface |

Run any example:

```bash
go run ./examples/modbus/server/ -port 5020
go run ./examples/modbus/read_registers/ -addr 127.0.0.1 -port 5020

go run ./examples/ethernetip/read_tag/ -addr 192.168.1.10:44818 -tag MyDINT
```

## Testing

```bash
go test ./... -count=1
```

See [TESTING.md](TESTING.md) for details.

## License

MIT. See [LICENSE.md](LICENSE.md).
