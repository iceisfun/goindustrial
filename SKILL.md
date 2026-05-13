---
name: goindustrial
description: Communicate with industrial PLCs using Modbus TCP and EtherNet/IP (CIP) via github.com/iceisfun/goindustrial. Covers client/server setup, register and tag operations, reconnection, monitoring, protocol-agnostic PLC access, and optional GoLua scripting bindings.
license: MIT
compatibility: claude-code, opencode
metadata:
  language: go
  domain: industrial-automation
---

# GoIndustrial Skill

Use this when helping someone who imported `github.com/iceisfun/goindustrial` and wants to communicate with Modbus TCP or EtherNet/IP devices.

## SKILLS

Copy-paste block for an AI assistant:

```text
SKILLS:
- GoIndustrial is a pure-Go library for Modbus TCP and EtherNet/IP (CIP) industrial protocols. Zero external dependencies.
- Two protocol clients, both implementing plc.PLC: modbus.Client and ethernetip.Client.
- Modbus quick path: modbus.Connect(ctx, host, opts...) -> client.ReadHoldingRegisters / WriteSingleRegister / etc.
- EtherNet/IP quick path: ethernetip.Connect(ctx, addr, opts...) -> client.ReadTag / WriteTag / ReadTagInto / etc.
- Both protocols support reconnecting transports: transport.NewReconnectingTransport[C](connector, closer, opts...).
- Functional options everywhere: modbus.WithRetries(3), modbus.WithUnitID(1), ethernetip.WithRetryDelay(2*time.Second), etc.
- Wire-level hex dump tracing: modbus.WithHexDump(os.Stdout) or ethernetip.WithHexDump(w) dumps all TCP traffic in hexdump -C format. Uses hexdump.Dumper which wraps io.Reader/io.Writer. Use io.MultiWriter to write to both stdout and a file.
- Monitor polls any plc.Reader: monitor.NewMonitor(reader) -> m.Subscribe(dataPoint, monitor.WithFrequency(100*ms)).
- Adaptive read clustering: monitor.NewClusteringReader(modbusClient, monitor.WithGapThreshold(32)) wraps a reader to coalesce nearby Modbus addresses into block reads. Reduces N subscriptions from N requests to ~1 per cluster. Supports cache TTL for cross-goroutine sharing, singleflight dedup, and WithClusteringEnabled(false) to force OFF.
- Clusterable interface: Modbus data point types (HoldingRegister, InputRegister, Coil, DiscreteInput) implement ClusterKey/ClusterAddr/ClusterQty/ClusterMerge/ClusterExtract for protocol-agnostic clustering.
- Modbus data areas: Coils (bool R/W), Discrete Inputs (bool R), Holding Registers (uint16 R/W), Input Registers (uint16 R).
- CIP data types: BOOL(0xC1), SINT(int8), INT(int16), DINT(int32), LINT(int64), USINT(uint8), UINT(uint16), UDINT(uint32), ULINT(uint64), REAL(float32), LREAL(float64), STRING(0xD0).
- Custom CIP types: implement cip.TypeCodec (MarshalCIP + UnmarshalCIP + CIPType()) and call cip.RegisterType(code, factory) at init() time. Optional fmt.Stringer for display name. cip.LookupType(code) returns codec; DataType.String() resolves registered names.
- Vendor packages: vendor/rockwell provides TypeCodec wrappers for Timer, Counter, PID, Control. Call rockwell.RegisterTimer(dt), etc. with controller-specific type codes from ListTags.
- Error classification: Modbus protocol errors (IsModbusError) are not retried; transport errors trigger Reset + retry. Same pattern for EIP: cipError not retried, transport errors retried.
- Servers: modbus.NewServer(addr, opts...) and ethernetip.NewServer(router, opts...). Both support net.Pipe injection for testing.
- EIP server has session management (unique handles, validation), ListIdentity/ListServices, client tracking (ConnectedClients()), connect/disconnect callbacks.
- Implicit I/O (cyclic messaging): IOScanner (client) sends Forward_Open, exchanges assembly data over UDP at RPI. Adapter (server) wires ConnMgr callbacks to Runtime for automatic I/O connection setup.
- IOScanner: NewIOScanner(session, udpAddr) -> scanner.Open(ctx, cfg) -> conn.SetOutput(data) / conn.Input() / conn.IsTimedOut() -> scanner.Close(ctx, conn).
- Adapter pattern: AssemblyObject + Runtime (UDP) + Scheduler (5ms tick) + ConnectionManager with WithOnOpen/WithOnClose callbacks wired to Runtime.AddConnection/RemoveConnection.
- Monitor subscribers: mon.NewSubscriber(bufferSize) returns Subscriber with buffered channel. sub.All() is iter.Seq[Event] for for-range. sub.Done() to unregister. Never blocks the monitor.
- plc.Value carries Type (DataType enum) and ByteOrder. Helper methods: val.Bool(), val.Int(), val.Uint(), val.Float32(), val.Float64() decode Raw using the value's byte order.
- Device probe: examples/ethernetip/probe scans identity, TCP/IP interface, Ethernet link, assembly instances, CIP object classes, connection manager stats, and optionally Logix tags.
- Optional Lua bindings (lua/ package, requires github.com/iceisfun/golua/v2 — Lua 5.5.0): industrialLua.Open(v) registers "modbus" and "eip" Lua globals.
- Lua modbus API: modbus.connect(addr, opts) -> client, client:read_holding_registers(addr, qty), client:write_register(addr, val), etc.
- Lua eip API: eip.connect(addr, opts) -> client, client:read_tag(name) -> auto-typed value, client:write_tag(name, val), client:list_tags(), etc.
- Lua methods use colon syntax (client:method(args)); the self parameter is handled automatically.
```

## What You Usually Need To Know

Most users want one of these:

1. Read/write Modbus registers or coils from a TCP device.
2. Read/write tags on a Rockwell Logix PLC over EtherNet/IP.
3. Monitor data points from either protocol with change detection.
4. Establish implicit I/O (cyclic) connections for real-time assembly exchange.
5. Probe an EtherNet/IP device to discover identity, assemblies, and capabilities.
6. Build a server/simulator/adapter for testing.

## Module and Imports

Module: `github.com/iceisfun/goindustrial`

```go
import (
    "github.com/iceisfun/goindustrial/logging"
    "github.com/iceisfun/goindustrial/transport"
    "github.com/iceisfun/goindustrial/plc"
    "github.com/iceisfun/goindustrial/monitor"
    "github.com/iceisfun/goindustrial/protocol/modbus"
    "github.com/iceisfun/goindustrial/protocol/ethernetip"
    "github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
    "github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)
```

## Modbus TCP Client

### Connect and Read

```go
ctx := context.Background()

client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithPort(502),
    modbus.WithUnitID(1),
    modbus.WithRetries(3),
    modbus.WithRetryDelay(500*time.Millisecond),
    modbus.WithLogger(logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Read 10 holding registers starting at address 0
regs, err := client.ReadHoldingRegisters(ctx, 0, 10)

// Read input registers (read-only sensor data)
inputs, err := client.ReadInputRegisters(ctx, 0, 5)

// Read coils
coils, err := client.ReadCoils(ctx, 0, 16)

// Read discrete inputs (read-only digital inputs)
dis, err := client.ReadDiscreteInputs(ctx, 0, 8)
```

### Write

```go
// Write a single register
err := client.WriteSingleRegister(ctx, 100, 0x1234)

// Write multiple registers
err = client.WriteMultipleRegisters(ctx, 100, []uint16{0x1111, 0x2222, 0x3333})

// Write a single coil
err = client.WriteSingleCoil(ctx, 0, true)

// Write multiple coils
err = client.WriteMultipleCoils(ctx, 0, []bool{true, false, true, true})

// Atomic read+write (FC 0x17)
regs, err := client.ReadWriteMultipleRegisters(ctx, 0, 5, 100, []uint16{42})
```

### Device Identification

```go
devID, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDBasicStream, modbus.DeviceIDVendorName)
// devID.Objects is map[DeviceIDObjectCode]string
// devID.ConformityLevel, devID.MoreFollows, devID.NextObjectID
```

### Modbus Types and Constants

```go
// Types (all type aliases)
modbus.Address     // uint16, register/coil address (0-65535)
modbus.Quantity    // uint16, number of registers/coils to read
modbus.UnitID      // byte, slave address (0-247)
modbus.CoilValue   // = bool
modbus.RegisterValue // = uint16

// Limits
modbus.MaxCoilCount          // 2000
modbus.MaxRegisterCount      // 125
modbus.MaxWriteCoilCount     // 1968
modbus.MaxWriteRegisterCount // 123
modbus.DefaultTCPPort        // 502

// Exception codes
modbus.ExceptionFunctionCodeNotSupported // 0x01
modbus.ExceptionDataAddressNotAvailable  // 0x02
modbus.ExceptionInvalidDataValue         // 0x03
modbus.ExceptionServerDeviceFailure      // 0x04

// Error helpers
modbus.IsModbusError(err)                          // true if protocol error
modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) // specific check
```

### Modbus DataPoints (for plc.PLC / monitor)

```go
modbus.HoldingRegister{Addr: 0, Qty: 10}
modbus.InputRegister{Addr: 0, Qty: 5}
modbus.Coil{Addr: 0, Qty: 16}
modbus.DiscreteInput{Addr: 0, Qty: 8}
```

## EtherNet/IP Client

### Connect and Read Tags

```go
ctx := context.Background()

client, err := ethernetip.Connect(ctx, "192.168.1.20",
    ethernetip.WithRetries(3),
    ethernetip.WithRetryDelay(1*time.Second),
    ethernetip.WithLogger(logger),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Raw byte read (includes 2-byte type code prefix)
data, err := client.ReadTag(ctx, "MyDINT")

// Read into a Go type (strips type code, unmarshals)
var val int32
err = client.ReadTagInto(ctx, "MyDINT", &val)

// Generic typed read
val, err := ethernetip.Read[int32](client, ctx, "MyDINT")

// Read array elements
slice, err := ethernetip.ReadSlice[float32](client, ctx, "MyArray", 10)

// Read multiple elements raw
data, err = client.ReadTagElements(ctx, "MyArray", 10)
```

### Write Tags

```go
// Auto-detects CIP type from Go type
err := client.WriteTag(ctx, "MyDINT", int32(42))
err = client.WriteTag(ctx, "MyREAL", float32(3.14))
err = client.WriteTag(ctx, "MyString", "Hello")
err = client.WriteTag(ctx, "MyBool", true)
```

### Timer and Counter

```go
// Timer: PRE (int32 ms), ACC (int32 ms), EN, TT, DN (bools)
timer, err := client.ReadTimer(ctx, "MyTimer")

// Counter: PRE, ACC (int32), CU, CD, DN, OV, UN (bools)
var counter cip.Counter
err = client.ReadTagInto(ctx, "MyCounter", &counter)
```

### Discovery

```go
// List all tags on a Logix controller
tags, err := client.ListTags(ctx)
// tags[i].InstanceID, tags[i].Name, tags[i].Type

// Device identity
items, err := client.ListIdentity(ctx)

// Service capabilities
services, err := client.ListServices(ctx)
```

### CIP Data Types

| CIP Type | Code | Go Type |
|----------|------|---------|
| BOOL | 0xC1 | `bool` |
| SINT | 0xC2 | `int8` |
| INT | 0xC3 | `int16` |
| DINT | 0xC4 | `int32` |
| LINT | 0xC5 | `int64` |
| USINT | 0xC6 | `uint8` |
| UINT | 0xC7 | `uint16` |
| UDINT | 0xC8 | `uint32` |
| ULINT | 0xC9 | `uint64` |
| REAL | 0xCA | `float32` |
| LREAL | 0xCB | `float64` |
| STRING | 0xD0 | `string` |

### Custom CIP Types (TypeCodec Registry)

Vendor-specific and site-specific struct types (UDTs, AOIs) can be registered
so they decode, encode, and display by name instead of `UNKNOWN(0x…)`.

```go
// 1. Define a struct with MarshalCIP, UnmarshalCIP, CIPType, and optional String.
type SetOn3Timer struct {
    PRE int32
    ACC int32
    EN, EN2, EN3, TT, DN bool
}

func (s *SetOn3Timer) CIPType() cip.DataType         { return 0x2F83 }
func (s *SetOn3Timer) String() string                  { return "SET_ON_3_TMR" }
func (s *SetOn3Timer) UnmarshalCIP(data []byte) error  { /* decode wire layout */ }
func (s *SetOn3Timer) MarshalCIP() ([]byte, error)     { /* encode wire layout */ }

// 2. Register at init() time.
func init() {
    cip.RegisterType(0x2F83, func() cip.TypeCodec { return new(SetOn3Timer) })
}

// 3. Now works automatically:
cip.DataType(0x2F83).String()    // "SET_ON_3_TMR"
cip.LookupType(0x2F83)           // returns *SetOn3Timer codec
cip.GoTypeToCIPType(&myStruct)   // returns 0x2F83
ethernetip.Read[SetOn3Timer](client, ctx, "MyTag") // typed read
```

For well-known Rockwell types, use the `vendor/rockwell` package:

```go
import "github.com/iceisfun/goindustrial/protocol/ethernetip/vendor/rockwell"

func init() {
    // Type codes are controller-specific — discover via ListTags
    rockwell.RegisterTimer(0x02B3)
    rockwell.RegisterCounter(0x02B4)
    rockwell.RegisterPID(0x02B5)
    rockwell.RegisterControl(0x02B6)
}
```

Available Rockwell types: `Timer`, `Counter`, `PID`, `Control`.

### EtherNet/IP DataPoints (for plc.PLC / monitor)

```go
ethernetip.Tag{Name: "MyDINT", Elements: 1}
ethernetip.Tag{Name: "MyArray", Elements: 10}
```

## Reconnecting Transport

Both protocols support the same reconnection pattern via the generic transport layer.

### Modbus Reconnecting Client

```go
connector := modbus.NewTCPConnector(host, modbus.WithPort(502))
closer := modbus.NewTCPCloser()

tp := transport.NewReconnectingTransport[*modbus.TCPConn](connector, closer,
    transport.WithOnConnect(func() { log.Println("connected") }),
    transport.WithOnDisconnect(func(err error) { log.Printf("disconnected: %v", err) }),
    // Multiple callbacks may be registered; they fire in registration order.
)

client := modbus.NewClient(tp,
    modbus.WithRetries(3),
    modbus.WithRetryDelay(2*time.Second),
    modbus.WithLogger(logger),
)
defer client.Close()
```

### EtherNet/IP Reconnecting Client

```go
// Convenience constructor (never fails, connects lazily)
client := ethernetip.NewReconnectingClient("192.168.1.20",
    ethernetip.WithRetries(-1),          // infinite retries
    ethernetip.WithRetryDelay(2*time.Second),
    ethernetip.WithLogger(logger),
)
defer client.Close()

// Or build manually for lifecycle hooks
connector := ethernetip.NewSessionConnector("192.168.1.20", logger)
closer := ethernetip.SessionCloser{}

tp := transport.NewReconnectingTransport[*ethernetip.Session](connector, closer,
    transport.WithOnConnect(func() { log.Println("session registered") }),
    transport.WithOnDisconnect(func(err error) { log.Printf("session lost: %v", err) }),
)

client := ethernetip.NewClient(tp,
    ethernetip.WithRetries(-1),
    ethernetip.WithLogger(logger),
)
```

### Error Classification

Both protocols distinguish protocol errors from transport errors:

- **Protocol errors** (Modbus exceptions, CIP status errors) are NOT retried -- the device understood the request and rejected it.
- **Transport errors** (TCP broken pipe, timeout, EOF) trigger `Reset()` + reconnect + retry.

```go
// Modbus
if modbus.IsModbusError(err) {
    // Device rejected the request (bad address, unsupported FC, etc.)
} else {
    // Transport error -- will be retried by the client
}

// EtherNet/IP: CIP errors return directly, transport errors are retried internally
```

## Monitor

The monitor polls any `plc.Reader` (both clients implement this) and emits events.

```go
// Optional: gate polling until connected (channel closed by OnConnect hook)
connected := make(chan struct{})
tp := transport.NewReconnectingTransport[*modbus.TCPConn](connector, closer,
    transport.WithOnConnect(func() { close(connected) }),
)

m, err := monitor.NewMonitor(client,
    monitor.WithLogger(logger),
    monitor.WithEventBuffer(128),
    monitor.WithConnected(connected), // wait for PLC connection before polling
)
defer m.Close()

// Subscribe with options
sub, err := m.Subscribe(modbus.HoldingRegister{Addr: 0, Qty: 10},
    monitor.WithFrequency(100*time.Millisecond),
    monitor.WithReadVariance(10*time.Millisecond),      // jitter to spread load
    monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
    monitor.WithHandler(func(snap monitor.Snapshot) {
        fmt.Printf("callback: %s = %x\n", snap.Point, snap.Value.Raw)
    }),
    monitor.WithInitialRead(0), // 0 = immediate; default is 50ms delay
)

// Consume the unified event channel
for evt := range m.Events() {
    if evt.Err != nil {
        log.Printf("poll error: %v", evt.Err)
        continue
    }
    if evt.Changed {
        fmt.Printf("[%s] %s: %x\n", evt.Snapshot.Timestamp.Format(time.RFC3339),
            evt.Snapshot.Point, evt.Snapshot.Value.Raw)
    }
}
```

### Monitor Subscribers (Broadcast Fan-Out)

```go
sub, err := mon.NewSubscriber(128)
if err != nil {
    log.Fatal(err)
}
defer sub.Done()

// iter.Seq[Event] — use in for-range
for evt := range sub.All() {
    if evt.Err != nil {
        continue
    }
    if evt.Changed {
        fmt.Printf("%s = %x\n", evt.Snapshot.Point, evt.Snapshot.Value.Raw)
    }
}
```

Multiple subscribers each receive all events (broadcast). Each has its own buffered channel — a slow subscriber never blocks the monitor or others.

### Adaptive Read Clustering

Wrap a Modbus client in `ClusteringReader` to coalesce nearby addresses into block reads:

```go
clustered := monitor.NewClusteringReader(modbusClient,
    monitor.WithGapThreshold(32),         // merge addresses within 32-register gap
    monitor.WithMaxRegistersPerRead(120),  // cap block size (Modbus limit: 125)
    monitor.WithMaxCoilsPerRead(2000),     // cap coil block size
    monitor.WithCacheTTL(200*time.Millisecond), // optional: share reads across goroutines
    // monitor.WithClusteringEnabled(false), // force OFF — passthrough mode
)

mon, _ := monitor.NewMonitor(clustered)
// Subscribe auto-registers points with the ClusteringReader.
// 10 adjacent registers → 1 Modbus request instead of 10.
```

The `Clusterable` interface allows any data point type to participate in clustering. All four Modbus types (`HoldingRegister`, `InputRegister`, `Coil`, `DiscreteInput`) implement it. Coil extraction handles bit-level packing correctly.

Singleflight deduplication prevents concurrent subscription goroutines from issuing duplicate reads for the same cluster.

## Implicit I/O (Cyclic Messaging)

### IOScanner (Client Side)

```go
// Use an existing TCP session for Forward_Open/Close
scanner, err := ethernetip.NewIOScanner(session, ":0") // ephemeral UDP port
if err != nil {
    log.Fatal(err)
}

conn, err := scanner.Open(ctx, ethernetip.IOConnectionConfig{
    OTConnectionPoint: 2,              // O→T assembly instance on target
    TOConnectionPoint: 1,              // T→O assembly instance on target
    OTSize:            12,             // bytes
    TOSize:            12,             // bytes
    RPI:               10 * time.Millisecond,
    TimeoutMultiplier: 3,
    TargetAddr:        targetUDPAddr,  // target IP:2222
})
if err != nil {
    log.Fatal(err)
}

// Write output assembly (sent to target each RPI)
conn.SetOutput([]byte{0x02, 0x00, 0xE8, 0x03, ...})

// Read input assembly (received from target each RPI)
input := conn.Input()
age := conn.InputAge()

// Clean shutdown
scanner.Close(ctx, conn) // sends Forward_Close
scanner.Shutdown(ctx)
```

### Adapter (Server Side)

```go
ao := assembly.NewAssemblyObject()
ao.RegisterAssembly(100, make([]byte, 12)) // consume
ao.RegisterAssembly(101, make([]byte, 12)) // produce

rt := runtime.NewRuntime(ao)
rt.Start(":2222")

sched := runtime.NewScheduler(rt)
sched.Start()

cm := connmgr.NewConnectionManager(
    connmgr.WithOnOpen(func(c *connmgr.Connection, req *connmgr.ForwardOpenRequest) {
        rpi := time.Duration(req.OTRPI) * time.Microsecond
        rt.AddConnection(&runtime.IOConnection{
            ConnectionID: c.OTConnectionID, RPI: rpi,
            Assembly: ao.GetInstance(100), IsConsumer: true,
        })
        rt.AddConnection(&runtime.IOConnection{
            ConnectionID: c.TOConnectionID, RPI: rpi,
            Assembly: ao.GetInstance(101), IsProducer: true,
        })
    }),
    connmgr.WithOnClose(func(c *connmgr.Connection) {
        rt.RemoveConnection(c.OTConnectionID)
        rt.RemoveConnection(c.TOConnectionID)
    }),
)

router := cip.NewMessageRouter()
router.RegisterObject(cip.ClassConnectionMgr, cm)

srv := ethernetip.NewServer(router)
srv.Start(ctx, ":44818")
```

## Modbus Server

```go
store := modbus.NewMemoryStore()

// Pre-populate data
store.SetHoldingRegister(0, 1234)
store.SetCoil(0, true)
store.SetInputRegister(0, 5678)
store.SetDiscreteInput(0, true)

srv := modbus.NewServer("0.0.0.0",
    modbus.WithServerPort(502),
    modbus.WithServerLogger(logger),
    modbus.WithServerDataStore(store),
    modbus.WithOnClientConnect(func(c modbus.ConnectedClient) {
        log.Printf("client connected: %s", c.RemoteAddr)
    }),
    modbus.WithOnClientDisconnect(func(c modbus.ConnectedClient) {
        log.Printf("client disconnected: %s (rx=%d tx=%d)", c.RemoteAddr, c.RxTransactions, c.TxTransactions)
    }),
)

srv.Start(ctx)
defer srv.Stop(ctx)

// Access data store at runtime
ds := srv.GetDataStore()
```

## EtherNet/IP Server

```go
router := cip.NewMessageRouter()
router.RegisterObject(cip.UINT(0x04), myCustomObject) // implements cip.Object

srv := ethernetip.NewServer(router,
    ethernetip.WithServerLogger(logger),
    ethernetip.WithIdentity(eip.ListIdentityItem{
        TypeID: eip.ItemIDListIdentity, EncapsVersion: 1,
        VendorID: 1, ProductName: "My Device",
    }),
    ethernetip.WithOnClientConnect(func(c ethernetip.ConnectedClient) {
        log.Printf("client connected: %s", c.RemoteAddr)
    }),
    ethernetip.WithOnClientDisconnect(func(c ethernetip.ConnectedClient) {
        log.Printf("client disconnected: %s", c.RemoteAddr)
    }),
)

srv.Start(ctx, ":44818")
defer srv.Stop()

// Query active clients
clients := srv.ConnectedClients()
```

A `cip.Object` must implement:

```go
type Object interface {
    HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error)
}
```

Return `cip.Error{Status: cip.StatusPathDestinationUnknown}` for unsupported requests.

## plc.PLC Interface

Both clients implement this protocol-agnostic interface:

```go
type PLC interface {
    Read(ctx context.Context, points ...DataPoint) ([]Value, error)
    Write(ctx context.Context, point DataPoint, data []byte) error
    Connect(ctx context.Context) error
    Disconnect(ctx context.Context) error
    IsConnected() bool
}
```

Use it when writing code that should work with any protocol:

```go
func pollAndLog(ctx context.Context, p plc.PLC, dp plc.DataPoint) {
    vals, err := p.Read(ctx, dp)
    if err != nil {
        log.Printf("read error: %v", err)
        return
    }
    log.Printf("%s = %x", dp, vals[0].Raw)
}
```

## Logging

```go
// Default logger (writes to stdout)
logger := logging.NewDefaultLogger(
    logging.WithLevel(logging.LevelDebug),
)

// Silent logger
logger := logging.NewNopLogger()

// Levels: LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelNone
```

All log methods take `context.Context` as first parameter:

```go
logger.Info(ctx, "connected to %s", addr)
logger.WithFields(map[string]any{"unit": 1}).Debug(ctx, "reading registers")
```

## Hex Dump Tracing

Both protocols support wire-level hex dump tracing via `WithHexDump(io.Writer)`:

```go
// Modbus: hex dump to stdout
client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithHexDump(os.Stdout),
)

// EtherNet/IP: hex dump to a file
f, _ := os.Create("trace.hex")
client, err := ethernetip.Connect(ctx, "192.168.1.20",
    ethernetip.WithHexDump(f),
)

// Both stdout and file simultaneously
client, err := modbus.Connect(ctx, "192.168.1.10",
    modbus.WithHexDump(io.MultiWriter(os.Stdout, f)),
)
```

Output format (hexdump -C style, short lines padded for column alignment):

```
>>> WRITE 12 bytes
00000000  00 00 00 00 00 06 01 03  00 00 00 03              |............    |
<<< READ 15 bytes
00000000  00 00 00 00 00 09 01 03  06 00 01 00 02 00 03     |...............  |
```

The `hexdump.Dumper` type can also be used directly to wrap any `io.Reader` or `io.Writer`:

```go
d := hexdump.NewDumper(os.Stdout)
wrappedReader := d.WrapReader(someReader)
wrappedWriter := d.WrapWriter(someWriter)
```

## Testing with net.Pipe

Both protocols support injecting a `net.Conn` for in-process testing:

```go
serverConn, clientConn := net.Pipe()

// Modbus
srv := modbus.NewServer("", modbus.WithServerConn(serverConn))
srv.Start(ctx)

client, _ := modbus.Connect(ctx, "", modbus.WithConn(clientConn))

// EtherNet/IP
router := cip.NewMessageRouter()
srv := ethernetip.NewServer(router, ethernetip.WithServerConn(serverConn))
srv.Start(ctx, "")

conn, _ := ethernetip.NewTCPConn("", ethernetip.WithConn(clientConn))
```

## Lua Scripting Bindings (Optional)

The `lua/` package provides [GoLua v2 (Lua 5.5.0)](https://github.com/iceisfun/golua) bindings. Import it separately — the core library remains zero-dependency.

```go
import (
    "github.com/iceisfun/golua/v2/vm"
    "github.com/iceisfun/golua/v2/stdlib"
    industrialLua "github.com/iceisfun/goindustrial/lua"
)

v := vm.New()
stdlib.Open(v)
industrialLua.Open(v) // registers "modbus" and "eip" Lua globals
```

### Lua Modbus API

```lua
local client = modbus.connect("192.168.1.10", {
    port = 502, unit = 1, retries = 2, timeout = 5
})

local regs = client:read_holding_registers(0, 10)  -- table of ints
local coils = client:read_coils(0, 8)               -- table of bools
local inputs = client:read_input_registers(0, 5)
local discrete = client:read_discrete_inputs(0, 8)

client:write_register(100, 0x1234)
client:write_registers(100, {10, 20, 30})
client:write_coil(0, true)
client:write_coils(0, {true, false, true})

local result = client:read_write_registers(0, 5, 100, {42})

-- Convert two registers to 32-bit values (big-endian)
local int32_val = client:to_int32(regs[1], regs[2])
local float32_val = client:to_float32(regs[1], regs[2])

local dev = client:read_device_id()
print(dev.vendor_name, dev.product_code, dev.revision)

client:close()
```

### Lua EtherNet/IP API

```lua
local client = eip.connect("192.168.1.20:44818", {
    retries = 2, timeout = 10
})

-- Auto-typed reads: returns int, float, bool, or string based on CIP type
local val = client:read_tag("MyDINT")           -- returns integer
local fval = client:read_tag("MyREAL")          -- returns float

-- Batch read
local values = client:read_tags({"Tag1", "Tag2", "Tag3"})

-- Write with auto-detect or explicit type
client:write_tag("MyDINT", 42)                  -- auto: DINT
client:write_tag("MyREAL", 3.14)                -- auto: REAL
client:write_tag("MyINT", 100, "INT")           -- explicit type

-- Structured types
local timer = client:read_timer("MyTimer")
print(timer.pre, timer.acc, timer.en, timer.tt, timer.dn)

local counter = client:read_counter("MyCounter")
print(counter.pre, counter.acc, counter.cu, counter.dn)

-- Tag discovery
local tags = client:list_tags()
for i = 1, #tags do
    print(tags[i].id, tags[i].name, tags[i].type)
end

client:close()
```

### Error Handling in Lua

All protocol errors raise Lua errors. Use `pcall` for safe calls:

```lua
local ok, err = pcall(function()
    client:read_tag("NONEXISTENT_TAG")
end)
if not ok then
    print("Error:", err)
end
```

## Examples

Runnable examples under `examples/` covering every operation, each with its own README:

**Modbus:** read_registers, write_registers, read_coils, write_coils, read_write_registers, device_identification, server, reconnecting, all_data_types, hexdump

**EtherNet/IP:** read_tag, write_tag, read_tag_typed, timer_counter, list_tags, list_identity, server, reconnecting, probe, adapter, io_scanner, hexdump, custom_type

**Cross-protocol:** monitor_polling, monitor_subscriber, plc_interface

**Lua scripting:** lua/modbus_client, lua/ethernetip_client, lua/monitor_tags, lua/condition_monitor

Run any example:

```bash
go run ./examples/modbus/server/ -port 5020
go run ./examples/modbus/read_registers/ -addr 127.0.0.1 -port 5020
go run ./examples/ethernetip/read_tag/ -addr 192.168.1.10:44818 -tag MyDINT
```

## Guidance For AI Assistants

Good defaults:

- Use `modbus.Connect` or `ethernetip.Connect` for simple scripts; build the transport manually only when you need lifecycle hooks or custom reconnection behavior.
- Use protocol-specific methods (ReadHoldingRegisters, ReadTag) for protocol-aware code; use plc.PLC for protocol-agnostic code.
- Always pass `context.Context` and handle timeouts with `context.WithTimeout`.
- Check `modbus.IsModbusError(err)` before assuming a transport failure.
- Use `ethernetip.Read[T]` generics for typed reads; use `ReadTagInto` when you have a pointer to unmarshal into.
- For EIP, `ReadTag` returns raw bytes with a 2-byte type code prefix; `ReadTagInto` strips it automatically.
- Timer and Counter are 14-byte Rockwell structures: `Reserved(2) + StatusBits(4) + PRE(4) + ACC(4)`.
- For vendor-specific struct types (UDTs, AOIs), implement `cip.TypeCodec` and call `cip.RegisterType` at init() time. Use `vendor/rockwell` for known Rockwell types. Type codes are controller-specific — discover them via `ListTags`.
- `WithRetries(-1)` means infinite retries for long-running applications.
- Both servers support `WithServerConn(net.Conn)` for deterministic in-process testing with `net.Pipe`.
- The monitor works with any `plc.Reader` -- you can poll Modbus and EtherNet/IP through the same monitor.

- For implicit I/O, use IOScanner (client) or the adapter pattern (server). The adapter wires ConnMgr callbacks to Runtime — Forward_Open automatically creates I/O connections.
- plc.Value now carries Type and ByteOrder metadata. Use val.Int(), val.Float32(), etc. instead of manual binary decoding.
- Monitor subscribers (NewSubscriber) use iter.Seq[Event] for for-range loops. Each subscriber gets all events independently (broadcast).
- Use the probe example to discover what a device supports before building integrations.
- For Lua bindings, `industrialLua.Open(v)` registers both `modbus` and `eip` globals. Lua methods use colon syntax (`client:read_tag("name")`); the self parameter is handled automatically.
- Lua errors from protocol operations are raised via panic and caught by pcall in Lua.

The goal of this skill is to help an assistant build correct integrations quickly using the public API.
