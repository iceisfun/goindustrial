# GoEIP — Project Skill Reference

## What This Is

A pure-Go EtherNet/IP (EIP) and Common Industrial Protocol (CIP) library for communicating with Rockwell Logix PLCs and other EIP devices. Supports explicit messaging (TCP, UCMM) and implicit messaging (UDP, Class 1 I/O). Zero external dependencies. Go 1.25.0.

Module: `github.com/iceisfun/goeip`

## Project Structure

```
goeip/
  pkg/
    client/              # High-level client API, transports, tag monitor, generics
    cip/                 # CIP types, encode/decode, services, paths, timer, counter, symbol
    eip/                 # EIP encapsulation header, commands, CPF, discovery
    session/             # EIP session registration & CIP messaging
    transport/           # TCP transport for EIP protocol
    runtime/             # UDP runtime for implicit messaging (Class 1 I/O)
    server/              # EIP server (adapter/target) TCP listener & dispatcher
    objects/
      assembly/          # Assembly Object (Class 0x04)
      connmgr/           # Connection Manager (Class 0x06, Forward_Open/Close)
  internal/
    logger.go            # Logger interface (Debugf/Infof/Warnf/Errorf) + ConsoleLogger, NopLogger
  cmd/                   # CLI tools and examples
    adapter/             # EIP target device simulator
    scanner/             # EIP originator (implicit I/O client)
    read_tag_single/     # Read single/array tags (raw bytes)
    read_tag_single_reconnecting/  # Auto-reconnect demo
    read_tag_struct/     # Read into Go types (int32, float32, timer, etc.)
    read_tag_timer/      # Read Timer structure
    write_tag_single/    # Write tags (BOOL/SINT/INT/DINT/LINT/USINT/UINT/UDINT/ULINT/REAL/LREAL/STRING)
    monitor_tag/         # Poll and monitor tags with change detection
    list_identity/       # Query device identity info
    list_tags/           # Enumerate all tags on a Logix controller (Symbol Object 0x6B)
  docs/                  # Markdown documentation
```

## Key Interfaces

### Transport (pkg/client/transport.go)
```go
type Transport interface {
    Session() (*session.Session, error)  // Get or create EIP session
    Reset(stale *session.Session) error  // Invalidate failed session
    Close() error
}
```

Two implementations:
- **directTransport** — connects immediately, no reconnect
- **reconnectingTransport** — lazy connect, auto-reconnect via RWMutex double-check locking

### Logger (internal/logger.go)
```go
type Logger interface {
    Debugf(format string, args ...interface{})
    Infof(format string, args ...interface{})
    Warnf(format string, args ...interface{})
    Errorf(format string, args ...interface{})
}
```

### CIP Marshaling (pkg/cip/)
```go
type Marshaler interface {
    MarshalCIP() ([]byte, error)
}
type Unmarshaler interface {
    UnmarshalCIP(data []byte) error
}
```

### TagMonitor interfaces (pkg/client/tag_monitor.go)
```go
type Refreshable interface {
    Refresh(snapshot TagSnapshot) (changed bool, err error)
}
type TagHandler func(snapshot TagSnapshot)
type TagReader interface {
    ReadTag(name string) ([]byte, error)
}
```

## Client API (pkg/client/client.go)

### Construction
```go
// Direct connection (connects immediately, no retry/reconnect)
c, err := client.Connect("192.168.1.10", logger)

// With reconnecting transport + retries
t := client.NewReconnectingTransport("192.168.1.10", logger,
    client.WithOnConnect(func() { ... }),
    client.WithOnDisconnect(func(err error) { ... }),
)
c := client.NewClient(t,
    client.WithRetries(5),              // 0=none, -1=infinite
    client.WithRetryDelay(2*time.Second), // default 1s
    client.WithLogger(logger),
)
defer c.Close()
```

### Tag Operations
```go
// Raw bytes (includes 2-byte type code prefix)
data, err := c.ReadTag("MyDINT")
data, err := c.ReadTagElements("MyArray", 21)

// Typed reads (strips type code, unmarshals into dst)
var val int32
err := c.ReadTagInto("MyDINT", &val)
var arr [10]int32
err := c.ReadTagElementsInto("MyArray", 10, &arr)

// Generic reads (pkg/client/generic.go)
val, err := client.Read[int32](c, "MyDINT")
slice, err := client.ReadSlice[int32](c, "MyArray", 10)

// Write (auto-detects CIP type from Go type)
err := c.WriteTag("MyTag", int32(42))
err := c.WriteTag("MyFloat", float32(3.14))
err := c.WriteTag("MyString", "Hello")

// Timer (special 14-byte Rockwell structure)
timer, err := c.ReadTimer("MyTimer")
// timer.PRE, timer.ACC, timer.EN, timer.TT, timer.DN
```

### Tag Monitor
```go
monitor, _ := client.NewTagMonitor(c,
    client.WithMonitorLogger(logger),
    client.WithEventBuffer(128),
)
defer monitor.Close()

monitor.AddTag("Position",
    client.WithFrequency(100*time.Millisecond),
    client.WithReadVariance(20*time.Millisecond), // ±jitter to spread load
    client.WithHandler(func(s client.TagSnapshot) { ... }),
    client.WithRefreshable(myStateObj),            // only emit on change
    client.WithInitialRead(true),
)

for evt := range monitor.Wait() {
    // evt.Err, evt.Changed, evt.Snapshot, evt.SubscriptionID
    var v int32
    evt.Snapshot.Into(&v)
}
```

## CIP Data Types (pkg/cip/types.go)

| CIP Type | Code | Go cip Type | Go Native |
|----------|------|-------------|-----------|
| BOOL | 0xC1 | `BOOL` | `bool` |
| SINT | 0xC2 | `SINT` | `int8` |
| INT | 0xC3 | `INT` | `int16` |
| DINT | 0xC4 | `DINT` | `int32` |
| LINT | 0xC5 | `LINT` | `int64` |
| USINT | 0xC6 | `USINT` | `uint8` |
| UINT | 0xC7 | `UINT` | `uint16` |
| UDINT | 0xC8 | `UDINT` | `uint32` |
| ULINT | 0xC9 | `ULINT` | `uint64` |
| REAL | 0xCA | `REAL` | `float32` |
| LREAL | 0xCB | `LREAL` | `float64` |
| STRING | 0xD0 | — | `string` |
| BYTE | 0xD1 | `BYTE` | `byte` |
| WORD | 0xD2 | `WORD` | `uint16` |
| DWORD | 0xD3 | `DWORD` | `uint32` |
| LWORD | 0xD4 | `LWORD` | `uint64` |

## Timer & Counter (pkg/cip/timer.go, counter.go)

14-byte binary layout: `Reserved(2) + StatusBits(4) + PRE(4) + ACC(4)`

**Timer** fields: PRE (int32 ms), ACC (int32 ms), EN (bit 31), TT (bit 30), DN (bit 29)
**Counter** fields: PRE (int32), ACC (int32), CU (bit 31), CD (bit 30), DN (bit 29), OV (bit 28), UN (bit 27)

Both implement `Unmarshaler`.

## Error Handling

- **cipError** (internal sentinel in client.go): wraps CIP protocol errors; these are NOT retried
- **Transport errors** (io.EOF, broken pipe, etc.): trigger `Reset()` + retry up to `retries` limit
- **cip.Error** (pkg/cip/types.go): `Status USINT` + `ExtStatus []UINT`, implements `error`
- Client retry loop: transport errors → reset session → retry; CIP errors → return immediately

## Implicit Messaging (UDP I/O)

### Server side (adapter)
```go
ao := assembly.NewAssemblyObject()
ao.RegisterAssembly(100, make([]byte, 32))  // Input assembly
ao.RegisterAssembly(150, make([]byte, 32))  // Output assembly

cm := connmgr.NewConnectionManager()
router := cip.NewMessageRouter()
router.RegisterObject(cip.ClassConnectionMgr, cm)

srv := server.NewServer(ao, router, logger)
srv.Start(":44818")

rt := runtime.NewRuntime(ao)
rt.Start(":2222")
```

### Client side (scanner)
```go
conn := &runtime.IOConnection{
    ConnectionID:  0x12345678,
    RPI:           100 * time.Millisecond,
    RemoteAddr:    targetUDPAddr,
    Assembly:      targetAssembly,
    IsProducer:    true,
    RunIdleHeader: true,
}
rt.AddConnection(conn)
```

## Session Layer (pkg/session/)

`session.Session` wraps `transport.Transport` (TCP). Handles:
- EIP RegisterSession / UnRegisterSession
- SendCIPMessage: encapsulates CIP in EIP SendRRData command
- Session handle tracking

## Naming Disambiguation

- `transport.Transport` (pkg/transport/) = raw TCP connection for EIP
- `client.Transport` (pkg/client/) = session lifecycle strategy interface (Session/Reset/Close)
- In `transport_reconnecting.go`, use `pkgtransport` alias for `pkg/transport`

## Test Patterns

- **Integration** (`client_test.go`): real TCP mock server, sync channels for handshaking
- **Unit** (`reconnect_test.go`): `mockTransport` implementing `client.Transport`
- **Stubs** (`tag_monitor_test.go`): `stubTagReader` implementing `TagReader`
- Run: `go test ./...`

## CLI Tools Quick Reference

| Tool | Usage |
|------|-------|
| `read_tag_single` | `go run ./cmd/read_tag_single -addr IP -tag NAME [-count N]` |
| `read_tag_struct` | `go run ./cmd/read_tag_struct -addr IP -tag NAME -type TYPE` |
| `read_tag_timer` | `go run ./cmd/read_tag_timer -addr IP -tag NAME` |
| `read_tag_single_reconnecting` | `go run ./cmd/read_tag_single_reconnecting -addr IP -tag NAME` |
| `write_tag_single` | `go run ./cmd/write_tag_single -addr IP -tag NAME -type TYPE -value VAL` |
| `monitor_tag` | `go run ./cmd/monitor_tag -addr IP -tag NAME -type TYPE` |
| `list_tags` | `go run ./cmd/list_tags -addr IP` |
| `list_identity` | `go run ./cmd/list_identity -addr IP` |
| `adapter` | `go run ./cmd/adapter [--addr :44818] [--udp-addr :2222] --input-assembly ID --output-assembly ID` |
| `scanner` | `go run ./cmd/scanner --addr IP:44818 --input-assembly ID --output-assembly ID [--rpi 100ms]` |

Default EIP TCP port: **44818**. Default I/O UDP port: **2222**.

## Conventions

- Functional options (`WithXXX`) pattern throughout
- All encoding is little-endian
- ReadTag response includes 2-byte type code prefix; ReadTagInto strips it automatically
- `WithRetries(-1)` = infinite retries; counter resets at MaxInt32 to prevent overflow
- Logger defaults to NopLogger when nil
- Reconnecting transport constructor never errors (lazy connect)
- Do not add Co-Authored-By lines to commit messages
