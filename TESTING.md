# Testing

## Running Tests

```bash
# All tests
go test ./... -count=1

# Verbose output
go test ./... -count=1 -v

# Specific package
go test ./protocol/modbus/ -count=1 -v
go test ./protocol/ethernetip/ -count=1 -v

# With race detector
go test ./... -race -count=1
```

## Test Architecture

### net.Pipe Testing

All protocol tests use `net.Pipe()` for deterministic, in-process testing with zero network overhead. This provides:

- No port conflicts or binding issues
- Deterministic timing (no network latency)
- Full control over both sides of the connection
- Works in CI without network access

**Pattern:**
```go
serverConn, clientConn := net.Pipe()
// Server wraps one end
// Client wraps the other
// Tests drive traffic through the pipe
```

### Test Summary (445 tests across 13 packages)

| Package | Tests | File(s) |
|---------|------:|---------|
| `protocol/ethernetip/cip` | 132 | `cip_test.go` |
| `protocol/modbus` | 96 | `modbus_test.go` |
| `protocol/ethernetip` | 48 | `ethernetip_test.go`, `io_test.go` |
| `monitor` | 47 | `monitor_test.go`, `cluster_test.go`, `subscriber_test.go` |
| `protocol/ethernetip/eip` | 37 | `eip_test.go` |
| `lua` | 18 | `lua_test.go` |
| `plc` | 17 | `plc_test.go` |
| `protocol/ethernetip/objects/connmgr` | 12 | `connmgr_test.go` |
| `transport` | 11 | `transport_test.go` |
| `protocol/ethernetip/objects/assembly` | 10 | `assembly_test.go` |
| `protocol/ethernetip/runtime` | 7 | `runtime_test.go` |
| `logging` | 7 | `logger_test.go` |
| (root) integration | 3 | `integration_test.go` |

### Modbus Tests (96 tests)

Located in `protocol/modbus/modbus_test.go`:

- Read/write holding registers, input registers, coils, discrete inputs
- All 11 function codes
- Exception handling (unsupported function, invalid data, invalid address)
- MBAP header encoding/decoding round-trips
- Request and response validation
- Concurrent read/write operations
- Server lifecycle (start, stop, connected clients)
- DataStore direct access and validation
- Device identification (FC 0x2B/0x0E)
- Transaction pool (place, release, timeout, collision)

### EtherNet/IP Tests (48 + 132 + 37 = 217 tests)

**`protocol/ethernetip/ethernetip_test.go` and `io_test.go`** (48 tests):
- EIP session register/unregister
- CIP ReadTag/WriteTag through server
- Generic `Read[T]`/`ReadSlice[T]` typed reads
- Timer and Counter decode
- ListIdentity and ListServices
- Tag listing via Symbol Object
- Full client-server integration via net.Pipe
- Server with pipeListener (full accept loop path)
- IOScanner Forward_Open/Close with cyclic UDP exchange

**`protocol/ethernetip/cip/cip_test.go`** (132 tests):
- CIP type marshal/unmarshal for all data types
- EPATH building (symbolic, class/instance/attribute, connection point)
- MessageRouter request/response encoding round-trips
- Service code dispatch
- Symbol instance decoding
- Timer and Counter struct decode/marshal
- Error formatting and status codes

**`protocol/ethernetip/eip/eip_test.go`** (37 tests):
- Encapsulation header encode/decode
- CPF encode/decode (connected and unconnected)
- ListIdentity/ListServices response parsing
- Real Wireshark capture parsing

### Monitor Tests (47 tests)

Located in `monitor/monitor_test.go`, `cluster_test.go`, `subscriber_test.go`:

- Subscribe, events, stop, change detection, handlers, close
- Adaptive read clustering (gap threshold, max block size, cache TTL, singleflight)
- Coil bit-level extraction
- Mixed data type clustering
- Subscriber fan-out and buffering
- Integration test with Modbus loopback server

### Foundation Tests

- **`logging/logger_test.go`** (7 tests): Log levels, formatting, fields, hexdump, NopLogger
- **`transport/transport_test.go`** (11 tests): Direct/reconnecting transport, reset, close, concurrency, callbacks, Peek
- **`plc/plc_test.go`** (17 tests): Value type conversions, DataPoint interface, Reader/Writer contracts
- **`lua/lua_test.go`** (18 tests): GoLua bindings for Modbus and EtherNet/IP operations

### CIP Object Tests

- **`objects/assembly/assembly_test.go`** (10 tests): Assembly Object read/write, instance management
- **`objects/connmgr/connmgr_test.go`** (12 tests): Forward_Open/Close, duplicate triad rejection, connection tracking, callbacks
- **`runtime/runtime_test.go`** (7 tests): UDP I/O runtime, connection add/remove, scheduler

### Cross-Protocol Integration Tests (3 tests)

Located in `integration_test.go`:

- **TestCrossProtocolMonitor**: Single Monitor polling both Modbus registers and EtherNet/IP tags, verifying events arrive on the shared channel with correct data
- **TestModbusClientPLCInterface**: Modbus Client as `plc.Reader` reading both registers and coils
- **TestEIPClientPLCInterface**: EtherNet/IP Session reading tags through `plc.Reader`

## Test Coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```
