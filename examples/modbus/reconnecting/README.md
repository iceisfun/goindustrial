# Reconnecting Modbus TCP Client Example

This example demonstrates how to build a resilient Modbus TCP client that automatically reconnects after server restarts, network outages, and transient failures. It manually constructs the `ReconnectingTransport` layer for full control over connection lifecycle hooks and retry behaviour.

## What This Example Does

1. **Builds a `ReconnectingTransport` manually** rather than using the `modbus.Connect()` convenience function, showing each component: `TCPConnector`, `TCPCloser`, and `ReconnectingTransport`.
2. **Registers lifecycle hooks** (`OnConnect` and `OnDisconnect`) that log connection state changes.
3. **Configures client-level retries** (3 retries with 2-second delay between attempts).
4. **Runs a continuous polling loop** that reads holding registers at a configurable interval and survives server restarts.
5. **Classifies errors** into Modbus protocol errors (never retried) and transport errors (retried automatically).
6. **Uses `context.WithTimeout`** per poll cycle to prevent a single hung request from blocking the loop.

## How to Run

First, start a Modbus TCP server (for example, the server example in this repository):

```bash
# Terminal 1: start the server
go run ./examples/modbus/server -port 5020
```

Then, in a second terminal, start the reconnecting client:

```bash
# Terminal 2: start the reconnecting client
go run ./examples/modbus/reconnecting -addr 127.0.0.1 -port 5020

# Poll a different address range every 1 second
go run ./examples/modbus/reconnecting -addr 127.0.0.1 -port 5020 -address 5 -interval 1s

# Connect to unit ID 1 (for gateway setups)
go run ./examples/modbus/reconnecting -addr 127.0.0.1 -port 5020 -unit 1
```

### Command-Line Flags

| Flag        | Default       | Description                                    |
|-------------|---------------|------------------------------------------------|
| `-addr`     | `127.0.0.1`  | Modbus TCP server address                      |
| `-port`     | `5020`       | Modbus TCP server port                         |
| `-unit`     | `0`          | Modbus unit ID (slave address)                 |
| `-address`  | `0`          | Starting holding register address to read      |
| `-interval` | `2s`         | Polling interval (Go duration: 1s, 500ms, etc.)|

### Testing Reconnection

To test the reconnection behaviour, try this sequence:

```bash
# 1. Start the server
go run ./examples/modbus/server -port 5020

# 2. Start the reconnecting client (in another terminal)
go run ./examples/modbus/reconnecting -port 5020

# 3. Stop the server (Ctrl+C in terminal 1)
#    Watch the client log transport errors and keep retrying

# 4. Restart the server
go run ./examples/modbus/server -port 5020
#    Watch the client automatically reconnect and resume polling
```

## Expected Output

### Normal operation

```
[2026-03-26T10:00:00-05:00] INFO: Building reconnecting transport to 127.0.0.1:5020
[2026-03-26T10:00:00-05:00] INFO: Starting polling loop: reading 5 holding registers from address 0 every 2s
[2026-03-26T10:00:00-05:00] INFO: Press Ctrl+C to stop.
[2026-03-26T10:00:02-05:00] INFO: [Poll #1] Reading 5 holding registers from address 0...
[2026-03-26T10:00:02-05:00] INFO: TRANSPORT: Connection established to 127.0.0.1:5020
[Poll #1] Successfully read 5 holding registers:
  Register 0 = 1000 (0x03E8)
  Register 1 = 500 (0x01F4)
  Register 2 = 60 (0x003C)
  Register 3 = 100 (0x0064)
  Register 4 = 4000 (0x0FA0)

[2026-03-26T10:00:04-05:00] INFO: [Poll #2] Reading 5 holding registers from address 0...
[Poll #2] Successfully read 5 holding registers:
  Register 0 = 1000 (0x03E8)
  ...
```

### During server outage

```
[2026-03-26T10:00:10-05:00] INFO: [Poll #5] Reading 5 holding registers from address 0...
[2026-03-26T10:00:10-05:00] WARN: Transport error (attempt 1/4): dial tcp 127.0.0.1:5020: connect: connection refused
[2026-03-26T10:00:12-05:00] WARN: Transport error (attempt 2/4): dial tcp 127.0.0.1:5020: connect: connection refused
[2026-03-26T10:00:14-05:00] WARN: Transport error (attempt 3/4): dial tcp 127.0.0.1:5020: connect: connection refused
[2026-03-26T10:00:16-05:00] WARN: Transport error (attempt 4/4): dial tcp 127.0.0.1:5020: connect: connection refused
[2026-03-26T10:00:16-05:00] WARN: [Poll #5] Transport error after retries: modbus: all 4 attempts failed: ...
[2026-03-26T10:00:16-05:00] WARN: [Poll #5] Will retry on next poll cycle.
```

### After server restart

```
[2026-03-26T10:00:20-05:00] INFO: [Poll #7] Reading 5 holding registers from address 0...
[2026-03-26T10:00:20-05:00] INFO: TRANSPORT: Connection established to 127.0.0.1:5020
[Poll #7] Successfully read 5 holding registers:
  Register 0 = 1000 (0x03E8)
  ...
```

## Reconnection Strategy

### How ReconnectingTransport Works

The `ReconnectingTransport` implements a lazy-connect, auto-reconnect pattern:

1. **Lazy connection**: The transport does not dial the server when created. The first call to `Conn()` triggers the initial connection.
2. **Connection caching**: Once established, the connection is cached and returned to all concurrent callers via an `RWMutex` fast path.
3. **Failure detection**: When a send fails with a transport error, the client calls `transport.Reset(staleConn)`. This closes the stale connection and clears the cache.
4. **Transparent reconnection**: The next call to `Conn()` sees an empty cache and creates a new connection via the `Connector`.
5. **Clean shutdown**: `Close()` permanently shuts down the transport. Subsequent `Conn()` calls return an error.

### Retry Layers

There are two independent retry mechanisms:

| Layer       | Configured By                     | Retries                        | Scope               |
|-------------|-----------------------------------|-------------------------------|----------------------|
| **Client**  | `WithRetries(3)`, `WithRetryDelay(2s)` | Transport errors only          | Single request       |
| **Poll loop** | `ticker.C` interval              | All errors (just waits)        | Application level    |

The client retries happen inside `client.send()` for a single Modbus request. If all client retries are exhausted, the error bubbles up to the poll loop, which logs it and waits for the next tick to try again. This two-tier approach means:

- **Short-lived glitches** (a dropped packet, a momentary network blip) are handled by client retries without the application even noticing.
- **Extended outages** (server down for maintenance) are handled by the poll loop, which keeps retrying every interval until the server comes back.

## Error Classification

### ModbusError (Protocol Level)

A `ModbusError` means the server received the request, parsed it, and explicitly rejected it with an exception response. The Modbus specification defines these in Section 7:

| Exception Code | Name                          | Meaning                                                    |
|---------------|-------------------------------|------------------------------------------------------------|
| 0x01          | FunctionCodeNotSupported      | The server does not implement the requested function code  |
| 0x02          | DataAddressNotAvailable       | The requested register/coil address is out of range        |
| 0x03          | InvalidDataValue              | The request payload contains an illegal value              |
| 0x04          | ServerDeviceFailure           | An internal server error occurred                          |

**ModbusErrors are never retried** because the server will return the same exception on every attempt. The correct response is to fix the request (e.g., use a valid address) or alert an operator.

You can check for specific exceptions:

```go
if modbus.IsModbusError(err) {
    // Server rejected the request - do not retry
    if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
        // Address range is wrong
    }
}
```

### Transport Errors (Network Level)

Any error that is *not* a `ModbusError` is a transport error. These include:

- Connection refused (server not running)
- Connection reset by peer (server crashed)
- I/O timeout (network congestion, firewall)
- Context deadline exceeded (our timeout was too short)

**Transport errors are retried** by the client (up to `WithRetries` count) because they are usually transient. Between retries, the transport resets the stale connection and the next `Conn()` call attempts a fresh dial.

## When to Retry vs. When to Fail

| Scenario                         | Error Type     | Client Retries? | Action                              |
|----------------------------------|---------------|-----------------|-------------------------------------|
| Server down                      | Transport      | Yes             | Wait for server to restart          |
| Network timeout                  | Transport      | Yes             | Check network, increase timeout     |
| Invalid register address         | ModbusError    | No              | Fix the address in your config      |
| Function not supported           | ModbusError    | No              | Use a different function code       |
| Server returns device failure    | ModbusError    | No              | Check server hardware/software      |

## Modbus Specification References

- **Modbus Application Protocol Specification V1.1b3** (Modbus Organization, 2012)
  - Section 4.1: MBAP Header (TCP framing, transaction ID matching)
  - Section 6.3: Read Holding Registers (FC 03)
  - Section 7: Exception Responses and exception codes
- **Modbus Messaging on TCP/IP Implementation Guide V1.0b** (Modbus Organization, 2006)
  - Connection management recommendations for TCP clients
  - Guidance on reconnection strategies

## Architecture Notes

- **Transport is protocol-agnostic**: The `transport.ReconnectingTransport[C]` is a generic type that works with any connection type. For Modbus, `C` is `*modbus.TCPConn`. The same pattern works for EtherNet/IP with a different connection type.
- **Thread safety**: The `ReconnectingTransport` uses an `RWMutex` with double-check locking so that concurrent readers share the fast path while only one goroutine performs reconnection.
- **No background goroutines**: The transport does not run any background goroutines or health-check loops. Reconnection is demand-driven: it happens when a caller needs a connection and the current one is stale or absent.
- **Stale connection guard**: `Reset(stale)` only invalidates the connection if it matches the caller's stale reference. This prevents thundering-herd resets when multiple goroutines detect the same failure simultaneously.
