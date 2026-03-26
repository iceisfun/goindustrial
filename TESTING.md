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

# Integration tests only
go test -run 'TestCrossProtocol|TestModbusClient|TestEIPClient' -v

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

### Modbus Tests (19 tests)

Located in `protocol/modbus/modbus_test.go`:

- Read/write holding registers, coils, discrete inputs
- Exception handling (unsupported function, invalid data)
- MBAP header encoding/decoding round-trips
- Concurrent read/write operations
- Server lifecycle (start, stop, connected clients)
- DataStore direct access and validation

### EtherNet/IP Tests (18 tests)

Located in `protocol/ethernetip/ethernetip_test.go`:

- EIP session register/unregister
- CIP ReadTag/WriteTag through server
- CIP encoding round-trips
- EPATH building (symbolic, class/instance/attribute, 16-bit)
- Timer and Counter decode/marshal
- Assembly Object and Connection Manager
- CPF encode/decode (connected and unconnected)
- Full client-server integration via net.Pipe
- Server with pipeListener (full accept loop path)

### Cross-Protocol Integration Tests (3 tests)

Located in `integration_test.go`:

- **TestCrossProtocolMonitor**: Single Monitor polling both Modbus registers and EtherNet/IP tags, verifying events arrive on the shared channel with correct data
- **TestModbusClientPLCInterface**: Modbus Client as `plc.Reader` reading both registers and coils
- **TestEIPClientPLCInterface**: EtherNet/IP Session reading tags through `plc.Reader`

### Foundation Tests

- `logging/logger_test.go`: Log levels, formatting, fields, hexdump, NopLogger
- `transport/transport_test.go`: Direct/reconnecting transport, reset, close, concurrency, callbacks
- `monitor/monitor_test.go`: Subscribe, events, stop, change detection, handlers, close

## Test Coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```
