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

## Testing

```bash
go test ./... -count=1
```

See [TESTING.md](TESTING.md) for details.

## License

MIT. See [LICENSE.md](LICENSE.md).
