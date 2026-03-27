# Audit Findings

## Scope

Reviewed the shared infrastructure, Modbus stack, EtherNet/IP stack, root integration tests, and current test strategy. I also ran:

- `go test ./...`
- `go test ./... -cover`
- `go test ./... -race`

## Executive Summary

- The project is in solid shape on happy-path protocol behavior, but cancellation and deadline behavior are not yet first-class in several core paths.
- Test coverage is materially thinner than the README/TESTING language suggests for connected EtherNet/IP runtime behavior, Modbus client lifecycle behavior, and shutdown/cancellation semantics.
- There is at least one concrete correctness gap (`ReadExceptionStatus` is claimed but not implemented by the default Modbus server) and one concrete concurrency issue (`go test -race` fails in Modbus server shutdown).
- The shared abstractions are usable, but some public APIs are still too loose (`opts ...any`, raw `[]byte`, context-free closers) for a library that aims to be protocol-safe and production-friendly.

## Findings

### High

1. Modbus README capability claim does not match the default server behavior.
   - Refs: `README.md:117`, `protocol/modbus/server.go:140`, `protocol/modbus/modbus_test.go:439`
   - `README.md` says Modbus supports all 11 function codes, but the default server does not register `FuncReadExceptionStatus` (`0x07`), and the test suite currently expects that request to fail as unsupported.

2. Modbus client-side cancellation is not fully aligned with Go I/O cancellation expectations.
   - Refs: `protocol/modbus/conn.go:177`, `protocol/modbus/conn.go:341`, `protocol/modbus/client.go:228`
   - `TCPConn.Send` observes `ctx.Done()`, but a blocked `writer.Write` in `writeLoop` has no write deadline and is not interruptible by context. A slow peer or `net.Pipe` stall can leave goroutines blocked past cancellation.

3. EtherNet/IP context/deadline support is mostly API-shaped, not behavior-shaped.
   - Refs: `protocol/ethernetip/session.go:34`, `protocol/ethernetip/session.go:79`, `protocol/ethernetip/conn.go:47`, `protocol/ethernetip/client.go:441`
   - Public methods accept `context.Context`, but the TCP send/receive path does not use deadlines, and retry backoff in `Client.do` uses plain `time.Sleep`, so canceled contexts do not reliably bound operation time.

4. EtherNet/IP server session handling is not protocol-safe yet.
   - Refs: `protocol/ethernetip/server.go:124`
   - `HandleConn` uses a fixed session handle (`0x01020304`), does not track per-connection registration state, and does not validate the incoming session handle before processing `SendRRData` or `SendUnitData`.

5. The monitor can hang shutdown on a blocked read.
   - Refs: `monitor/monitor.go:86`, `monitor/monitor.go:253`, `monitor/monitor.go:260`, `monitor/options.go:38`
   - Subscriptions poll with `context.Background()` and expose no per-subscription timeout/cancel option, so `Close()` can wait indefinitely if a `plc.Reader` blocks.

6. `go test -race` currently fails in the Modbus server.
   - Refs: `protocol/modbus/server.go:257`, `protocol/modbus/server.go:316`
   - The race detector reports a concurrent read/write on `s.listener` between `Stop` and `acceptLoop`, so the package is not yet race-clean even though the normal test suite passes.

### Medium

7. Major protocol areas have little or no automated coverage.
   - Refs: `protocol/ethernetip/runtime/runtime.go:1`, `protocol/ethernetip/runtime/scheduler.go:1`, `protocol/ethernetip/objects/assembly/assembly.go:1`, `protocol/ethernetip/objects/connmgr/connmgr.go:1`
   - `go test -cover ./...` reports `0.0%` coverage for EtherNet/IP runtime and object packages, despite these being protocol-critical paths for connected I/O and Forward Open/Close behavior.

8. Modbus coverage is still thin for client lifecycle and transaction semantics.
   - Refs: `protocol/modbus/client.go:28`, `protocol/modbus/transaction.go:1`, `protocol/modbus/modbus_test.go`
   - Coverage is only `34.8%` for `protocol/modbus`; I did not find targeted tests for client reconnect behavior, transaction timeout behavior, out-of-order responses, or canceled requests during send/wait.

9. EtherNet/IP coverage is still thin for discovery and enumeration features.
   - Refs: `protocol/ethernetip/client.go:295`, `protocol/ethernetip/session.go:156`, `protocol/ethernetip/eip/discovery.go:35`
   - Coverage is `40.3%` for `protocol/ethernetip`, `40.9%` for `protocol/ethernetip/eip`, and there are no end-to-end tests for `ListIdentity`, `ListServices`, or `ListTags` against a realistic server implementation.

10. Connected EtherNet/IP runtime is architecturally incomplete.
    - Refs: `protocol/ethernetip/objects/connmgr/connmgr.go:35`, `protocol/ethernetip/runtime/runtime.go:73`
    - `Forward_Open`/`Forward_Close` only maintain an internal map. They do not create runtime I/O connections, bind assemblies, validate addressing/state deeply, or exercise watchdog/scheduler semantics through integration tests.

11. The transport shutdown contract is weaker than the connect contract.
    - Refs: `transport/transport.go:13`, `transport/direct.go:67`, `transport/reconnecting.go:81`
    - `Connector.Connect` is context-aware, but `Closer.Close` is not. That makes reset/close paths impossible to bound with deadlines or cancellation, which is a poor fit for industrial communications and orderly shutdown.

12. The shared PLC abstraction is still too raw for strong cross-protocol use.
    - Refs: `plc/plc.go:5`, `integration_test.go:75`
    - `plc.DataPoint` only requires `String()`, `plc.Value` only carries raw bytes, and `Writer` takes raw bytes. Even the integration helper must switch on concrete protocol types, which suggests missing codec/metadata interfaces.

13. Modbus option handling gives up compile-time safety.
    - Refs: `protocol/modbus/client.go:30`
    - `Connect(ctx, host, opts ...any)` silently ignores unsupported option types. For a public API, a typed option envelope or split constructors would make misuse much easier to catch.

14. Modbus value aliases lose domain separation.
    - Refs: `protocol/modbus/types.go:34`
    - `CoilValue`, `DiscreteInputValue`, `RegisterValue`, and `InputRegisterValue` are aliases, not distinct named types, so the type system cannot help prevent mixing logically different data classes.

15. Monitor event delivery semantics need sharper documentation.
    - Refs: `monitor/monitor.go:156`, `monitor/options.go:25`
    - Events are dropped when the buffer is full, but public comments do not make it clear that delivery is best-effort rather than backpressured or lossless.

16. Several exported behaviors need clearer comments.
    - Refs: `protocol/modbus/client.go:99`, `protocol/modbus/client.go:464`, `transport/direct.go:46`, `transport/reconnecting.go:34`, `logging/default.go:63`
    - Examples: `Client.IsConnected()` may trigger transport work via `Conn(context.Background())`; `ReadDeviceIdentification` does not explain multi-part enumeration; and multiple exported transport/logger methods have no method-level doc comments.

### Low

17. EtherNet/IP runtime packet parsing is still very manual and underexplained.
    - Refs: `protocol/ethernetip/runtime/runtime.go:149`, `protocol/ethernetip/eip/cpf.go:1`, `protocol/ethernetip/objects/connmgr/types.go:1`
    - The code relies on byte slicing with minimal validation and limited field documentation for protocol-sensitive structures such as CPF items, connection parameters, and socket-address-oriented fields.

18. EtherNet/IP server/client internals are tightly coupled to concrete types.
    - Refs: `protocol/ethernetip/session.go:14`, `protocol/ethernetip/server.go:53`, `protocol/ethernetip/runtime/runtime.go:29`
    - There are likely useful missing interfaces around transport/session behavior, runtime assembly access, and connection-manager/runtime integration that would make testing and future extension easier.

## Recommended Test Additions

1. Cancellation/deadline tests
   - Add `net.Pipe` tests that cancel while blocked in Modbus write, Modbus read wait, EtherNet/IP receive, monitor polling, and reconnect retry backoff.

2. Race and shutdown tests
   - Add focused tests around Modbus server `Start`/`Stop`, accept-loop shutdown, monitor `Close`, and transport `Reset`/`Close` while operations are in flight.

3. Modbus protocol completeness tests
   - Add explicit coverage for `ReadExceptionStatus`, multi-part device identification enumeration, transaction timeout cleanup, and out-of-order response matching.

4. EtherNet/IP discovery and connected-I/O tests
   - Add end-to-end tests for `ListIdentity`, `ListServices`, `ListTags`, session validation, invalid session reuse, `Forward_Open`/`Forward_Close`, watchdog expiry, and runtime scheduler packet generation.

5. Shared abstraction tests
   - Add tests for event dropping semantics, context-bounded monitor shutdown, and protocol-agnostic reader/writer behavior with richer typed metadata.

## Commands / Evidence

- `go test ./...` passed.
- `go test ./... -cover` passed, but reported low coverage in key protocol packages:
  - `protocol/modbus`: `34.8%`
  - `protocol/ethernetip`: `40.3%`
  - `protocol/ethernetip/eip`: `40.9%`
  - `protocol/ethernetip/runtime`: `0.0%`
  - `protocol/ethernetip/objects/assembly`: `0.0%`
  - `protocol/ethernetip/objects/connmgr`: `0.0%`
- `go test ./... -race` failed in `protocol/modbus` due to a race between `Server.Stop` and `acceptLoop` on `s.listener`.
