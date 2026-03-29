# goindustrial

[![Go Reference](https://pkg.go.dev/badge/github.com/iceisfun/goindustrial/.svg)](https://pkg.go.dev/github.com/iceisfun/goindustrial/)

A unified Go library for industrial communication protocols. Consolidates **Modbus TCP** and **EtherNet/IP (CIP)** under common abstractions for transport, logging, monitoring, and PLC access.

Zero external dependencies for core protocols. Pure Go. Go 1.25+. The optional `lua/` package requires [GoLua](https://github.com/iceisfun/golua).

## Package Structure

```
goindustrial/
    logging/                     Unified structured logging
    transport/                   Generic transport lifecycle (reconnect, retry)
    hexdump/                     Wire-level hex dump tracing
    plc/                         Protocol-agnostic PLC interface
    monitor/                     Polling engine with change detection

    protocol/modbus/             Modbus TCP client, server, and protocol
    protocol/ethernetip/         EtherNet/IP client, server, IOScanner, and protocol
    protocol/ethernetip/cip/     Common Industrial Protocol
    protocol/ethernetip/eip/     EIP encapsulation layer
    protocol/ethernetip/objects/ CIP objects (Assembly, Connection Manager)
    protocol/ethernetip/runtime/ UDP I/O runtime and scheduler for implicit messaging

    lua/                         Optional GoLua bindings (requires github.com/iceisfun/golua)
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

### Adaptive Read Clustering

Wrap a Modbus client in a `ClusteringReader` to coalesce nearby register reads into block reads, reducing Modbus TCP requests:

```go
modbusClient, _ := modbus.Connect(ctx, "192.168.1.100")

// Wrap with clustering — nearby addresses are merged into single reads.
clustered := monitor.NewClusteringReader(modbusClient,
    monitor.WithGapThreshold(32),        // merge if gap ≤ 32 registers
    monitor.WithMaxRegistersPerRead(120), // protocol-safe block size
    // monitor.WithClusteringEnabled(false), // force OFF
)

mon, _ := monitor.NewMonitor(clustered)

// These 3 subscriptions produce 1 Modbus request instead of 3.
mon.Subscribe(modbus.HoldingRegister{Addr: 100, Qty: 1}, ...)
mon.Subscribe(modbus.HoldingRegister{Addr: 101, Qty: 1}, ...)
mon.Subscribe(modbus.HoldingRegister{Addr: 102, Qty: 1}, ...)
```

### Hex Dump Tracing

Both protocols support wire-level hex dump tracing via `WithHexDump`. Pass any `io.Writer` to see every byte on the wire in traditional `hexdump -C` format:

```go
// Modbus: hex dump to stdout
client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithHexDump(os.Stdout),
)

// EtherNet/IP: hex dump to a file
f, _ := os.Create("trace.hex")
defer f.Close()
client, err := ethernetip.Connect(ctx, "192.168.1.20",
    ethernetip.WithHexDump(f),
)

// Both stdout and file simultaneously
client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithHexDump(io.MultiWriter(os.Stdout, f)),
)
```

Output:

```
>>> WRITE 12 bytes
00000000  00 00 00 00 00 06 01 03  00 00 00 03              |............    |
<<< READ 15 bytes
00000000  00 00 00 00 00 09 01 03  06 00 01 00 02 00 03     |............... |
```

### Lua Scripting (Optional)

The `lua/` package provides [GoLua](https://github.com/iceisfun/golua) bindings so Lua scripts can drive Modbus and EtherNet/IP operations. This is useful for user-configurable data collection, alerting, and transformation logic without recompiling Go code.

```go
import (
    "github.com/iceisfun/golua/vm"
    "github.com/iceisfun/golua/stdlib"
    industrialLua "github.com/iceisfun/goindustrial/lua"
)

v := vm.New()
stdlib.Open(v)
industrialLua.Open(v) // registers "modbus" and "eip" globals
```

```lua
-- Read Modbus registers
local client = modbus.connect("192.168.1.10", {port = 502, unit = 1})
local regs = client:read_holding_registers(0, 10)
for i = 1, #regs do print(regs[i]) end
client:close()

-- Read EtherNet/IP tags
local plc = eip.connect("192.168.1.20:44818")
local val = plc:read_tag("MyDINT")
print("MyDINT =", val)
plc:close()
```

## Architecture

### Shared Infrastructure

- **`logging.Logger`** -- Context-aware, leveled, structured fields. Pluggable: supply your own or use the default.
- **`transport.Transport[C]`** -- Generic connection lifecycle with `DirectTransport` and `ReconnectingTransport` (RWMutex double-check locking, lifecycle hooks).
- **`hexdump.Dumper`** -- Wire-level hex dump tracing for any `io.Reader`/`io.Writer`. Both protocols accept `WithHexDump(io.Writer)` to capture all TCP traffic.
- **`plc.PLC`** -- Protocol-agnostic interface (`Reader`, `Writer`, `Connect`, `Disconnect`). Both protocol clients implement this.
- **`monitor.Monitor`** -- Subscription-per-goroutine polling engine with frequency control, read variance (jitter), change detection, and handler callbacks.

### Protocol Implementations

**Modbus TCP** (`protocol/modbus/`)
- All 11 function codes (read/write coils, registers, device identification)
- MBAP header framing with atomic transaction ID allocation
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
| [`modbus/hexdump`](examples/modbus/hexdump/) | Wire-level hex dump tracing to stdout or file |

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
| [`ethernetip/probe`](examples/ethernetip/probe/) | Full device probe: identity, network, assemblies, CIP objects, tags |
| [`ethernetip/adapter`](examples/ethernetip/adapter/) | Implicit I/O adapter: accepts Forward_Open, cyclic UDP exchange |
| [`ethernetip/io_scanner`](examples/ethernetip/io_scanner/) | Implicit I/O scanner: sends Forward_Open, cyclic UDP exchange |
| [`ethernetip/hexdump`](examples/ethernetip/hexdump/) | Wire-level hex dump tracing to stdout or file |

### Cross-Protocol

| Example | Description |
|---------|-------------|
| [`monitor_polling`](examples/monitor_polling/) | Poll Modbus + EtherNet/IP through a unified Monitor with change detection |
| [`monitor_subscriber`](examples/monitor_subscriber/) | Broadcast fan-out with buffered Subscribers and iter.Seq for-range |
| [`plc_interface`](examples/plc_interface/) | Protocol-agnostic code using the `plc.PLC` interface |

### Lua Scripting

| Example | Description |
|---------|-------------|
| [`lua/modbus_client`](examples/lua/modbus_client/) | Read/write Modbus registers from Lua scripts |
| [`lua/ethernetip_client`](examples/lua/ethernetip_client/) | Read/write EtherNet/IP tags from Lua with tag discovery |
| [`lua/monitor_tags`](examples/lua/monitor_tags/) | Lua-driven tag polling with change detection |
| [`lua/condition_monitor`](examples/lua/condition_monitor/) | Compound boolean conditions with per-signal hold times |

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
