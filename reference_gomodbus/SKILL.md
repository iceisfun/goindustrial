# GoModbus - Project Skill Reference

## What This Is

A pure-Go Modbus TCP client/server library. Zero external dependencies. Thread-safe. Type-safe semantic types. Implements the Modbus Application Protocol V1.1b3 specification.

## Project Structure

```
gomodbus/
  common/          # Shared interfaces, types, errors, PDU, logger
    test/          # Mock implementations (transport, datastore, messages)
  protocol/        # PDU encoding/decoding (ProtocolHandler)
  transport/       # TCP transport, transactions, transaction pool
  client/          # TCPClient, BaseClient, transport abstraction
  server/          # TCPServer, MemoryStore, ConnectedClient, protocol handler
  logging/         # Logger and NoopLogger
  cmd/             # Example CLI programs
    client/        # 10 sample client programs (one per function)
    server/        # Sample server
    args/          # CLI argument parsing
  docs/            # Documentation
```

## Key Interfaces (all in `common/`)

- **`Client`** — `ReadCoils`, `ReadDiscreteInputs`, `ReadHoldingRegisters`, `ReadInputRegisters`, `WriteSingleCoil`, `WriteSingleRegister`, `WriteMultipleCoils`, `WriteMultipleRegisters`, `ReadWriteMultipleRegisters`, `ReadExceptionStatus`, `ReadDeviceIdentification`
- **`Server`** — `Start(ctx)`, `Stop(ctx)`, `IsRunning()`, `RegisterHandler(FunctionCode, HandlerFunc)`, `ConnectedClients()`
- **`Transport`** — `Send(ctx, Request) (Response, error)`, `Connect(ctx)`, `Close()`, `IsConnected()`
- **`DataStore`** — `ReadCoils`, `ReadDiscreteInputs`, `ReadHoldingRegisters`, `ReadInputRegisters`, `WriteSingleCoil`, `WriteSingleRegister`, `WriteMultipleCoils`, `WriteMultipleRegisters`
- **`Protocol`** — Request generation and response parsing for each function code
- **`LoggerInterface`** — `Trace`, `Debug`, `Info`, `Warn`, `Error`, `WithFields`, `GetLevel`, `SetLevel`

## Semantic Types (`common/types.go`)

All `uint16` wrappers: `Address`, `Quantity`, `TransactionID`, `UnitID`, `FunctionCode`, `ExceptionCode`, `CoilValue`, `RegisterValue`, `InputRegisterValue`, `DiscreteInputValue`, `ExceptionStatus`, `ReadDeviceIDCode`, `DeviceIDObjectCode`

Byte-sized types for Device Identification (FC 0x2B/0x0E): `ConformityLevel` (0x01–0x03 stream, 0x81–0x83 stream+individual), `MoreFollows` (0x00/0xFF), `MEIType`

## Supported Modbus Functions

| Code | Name | Client Method |
|------|------|---------------|
| 0x01 | Read Coils | `ReadCoils` |
| 0x02 | Read Discrete Inputs | `ReadDiscreteInputs` |
| 0x03 | Read Holding Registers | `ReadHoldingRegisters` |
| 0x04 | Read Input Registers | `ReadInputRegisters` |
| 0x05 | Write Single Coil | `WriteSingleCoil` |
| 0x06 | Write Single Register | `WriteSingleRegister` |
| 0x07 | Read Exception Status | `ReadExceptionStatus` |
| 0x0F | Write Multiple Coils | `WriteMultipleCoils` |
| 0x10 | Write Multiple Registers | `WriteMultipleRegisters` |
| 0x17 | Read/Write Multiple Registers | `ReadWriteMultipleRegisters` |
| 0x2B/0x0E | Read Device Identification | `ReadDeviceIdentification` |

## Client Architecture

**Creation patterns:**
```go
// Direct connection
c := client.NewTCPClient(host, transport.WithPort(502), ...).WithOptions(client.WithTCPUnitID(1), ...)

// With transport abstraction (reconnecting)
t := client.NewReconnectingTransport(host, logger, transportOpts, tcpOpts)
c := client.NewTCPClientFromTransport(t, tcpOption...)
```

**Layers:** `TCPClient` wraps `BaseClient` wraps `Transport` (interface in client pkg)

**Three transport strategies:**
1. **DirectTransport** — connect once at creation, no reconnect
2. **ReconnectingTransport** — lazy connect, auto-reconnect on failure
3. **TCPTransport** (low-level) — raw TCP with MBAP header encoding, transaction pool

**`transportBridge`** adapts `client.Transport` to `common.Transport` interface.

## Server Architecture

```go
s := server.NewTCPServer(addr,
    server.WithServerPort(port),
    server.WithServerLogger(logger),
    server.WithServerDataStore(store),
    server.WithServerListener(listener),
    server.WithOnClientConnect(fn),
    server.WithOnClientDisconnect(fn),
)
```

- `MemoryStore` — thread-safe in-memory `DataStore` with `sync.RWMutex`, sparse maps
- `ConnectedClient` — snapshot struct with `RemoteAddr`, `ConnectedAt`, `RxTransactions`, `TxTransactions`, `FunctionCodeStats`
- Internal `clientConn` uses `atomic.Uint64` for lockless statistics

## Transaction Pool (`transport/transaction_pool.go`)

- Pre-allocates 65,536 transaction IDs via buffered channel
- `Place(ctx, request)` assigns ID, `Get(txID)` retrieves, `Release(txID)` frees
- Timeout monitor goroutine (default 5s timeout, 1s check interval)
- `Transaction` struct has `ResponseCh` and `ErrCh` channels with non-blocking sends

## Protocol Layer (`protocol/protocol.go`)

MBAP Header: `TransactionID(2) + ProtocolID(2) + Length(2) + UnitID(1)` = 7 bytes before PDU.
Max PDU: 253 bytes. Max ADU: 260 bytes.

## Error Handling (`common/errors.go`)

Sentinel errors: `ErrNotConnected`, `ErrAlreadyConnected`, `ErrInvalidQuantity`, `ErrInvalidAddress`, `ErrTimeout`, `ErrTransactionTimeout`, `ErrTransportClosing`

`ModbusError` type with `FunctionCode` + `ExceptionCode`. Helpers: `IsModbusError(err)`, `IsExceptionError(err, code)`, `GetExceptionString(code)`

## Modbus Constraints

| Constraint | Value |
|---|---|
| Max coil read | 2,000 |
| Max register read | 125 |
| Max coil write | 1,968 |
| Max register write | 123 |
| Max R/W read | 125 |
| Max R/W write | 121 |
| Default TCP port | 502 |

## Configuration Pattern

Functional options (`With*` functions) throughout all packages. Each package has its own option type:
- `transport.TCPTransportOption` — `WithPort`, `WithTimeoutOption`, `WithReader`, `WithWriter`, `WithTransportLogger`
- `client.TCPOption` — `WithTCPLogger`, `WithTCPUnitID`
- `client.TransportOption` — `WithOnConnect`, `WithOnDisconnect`
- `server.TCPServerOption` — `WithServerPort`, `WithServerLogger`, `WithServerDataStore`, `WithServerListener`, `WithOnClientConnect`, `WithOnClientDisconnect`
- `transport.TransactionPoolOption` — timeout configuration
- `logging.Option` — logger configuration

## Concurrency

- `MemoryStore`: `sync.RWMutex`
- `TransactionPool`: `sync.Mutex`
- `ReconnectingTransport`: `sync.RWMutex` with double-check locking
- `TCPServer.clients`: `sync.RWMutex`
- `clientConn` stats: `atomic.Uint64`
- All client operations are safe for concurrent use — each gets a unique transaction ID

## Testing

```bash
go test ./...
```

- 11 test files, integration test at `transport/integration_test.go`
- Uses `common.FindFreePortTCP()` or pre-created listeners to avoid port races
- Mocks in `common/test/`: `MockTransport`, `MockDataStore`, `MockRequest`, `MockResponse`

## Conventions

- Spec references in comments: `// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section X.X`
- All I/O methods take `context.Context` as first argument
- Go version: 1.24.1
- No external dependencies
- Do not add Co-Authored-By lines to commit messages
