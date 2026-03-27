# EtherNet/IP Reconnecting Client Example

This example demonstrates how to build a **fault-tolerant EtherNet/IP client** that automatically reconnects after network failures. It manually constructs the transport layer to show the full architecture and to attach lifecycle hooks (OnConnect/OnDisconnect).

## What This Example Does

1. **Manually builds a reconnecting transport** using `SessionConnector`, `SessionCloser`, and `ReconnectingTransport`
2. **Registers lifecycle hooks** that fire on connection and disconnection events
3. **Polls a tag continuously** at a configurable interval
4. **Survives server restarts** -- when the connection drops, the transport automatically reconnects
5. **Distinguishes CIP errors from transport errors** and handles each appropriately

## Transport Architecture

The transport layer in `goindustrial` is a generic abstraction that manages the lifecycle of a protocol-specific connection. Here is how the pieces fit together:

```
ethernetip.Client
   Uses the transport to get an active *Session, then sends CIP requests.
   If a transport error occurs, it resets the session and retries.
        |
        v
transport.ReconnectingTransport[*ethernetip.Session]
   - Conn(): returns an active session, creating one lazily if needed
   - Reset(stale): tears down a failed session so the next Conn() reconnects
   - Fires OnConnect/OnDisconnect hooks on lifecycle transitions
        |
        v
ethernetip.SessionConnector              ethernetip.SessionCloser
   - Connect(): dials TCP + registers      - Close(): unregisters + closes TCP
     an EIP session                          session
```

### Why Build the Transport Manually?

The convenience function `ethernetip.NewReconnectingClient()` creates all of this internally, but you cannot attach lifecycle hooks. Building manually gives you:

- **OnConnect hook**: fires after every successful session establishment (including reconnections)
- **OnDisconnect hook**: fires whenever a session is torn down (due to errors or explicit close)
- **Full control** over connector options (e.g., custom dial timeouts)
- **Testability**: you can inject mock connectors for unit testing

## CIP Errors vs Transport Errors

This is one of the most important concepts when building resilient industrial applications:

### Transport Errors (Retried Automatically)

Transport errors indicate that the TCP connection was lost or the request could not be delivered:

- Connection refused (server not running)
- Connection reset (server crashed mid-request)
- Read/write timeout (network failure)
- EOF (server closed the connection)

These errors are **transient** -- once the connection is re-established, the request may succeed. The client's retry loop handles these automatically:

1. Detects the transport error
2. Calls `transport.Reset(staleSession)` to invalidate the broken session
3. Waits `retryDelay` (2 seconds in this example)
4. Calls `transport.Conn()` which triggers a reconnect
5. Retries the CIP request on the new session

### CIP Errors (NOT Retried)

CIP errors are protocol-level responses from the PLC. The PLC received your request, processed it, and deliberately returned an error:

- `StatusPathDestinationUnknown` (0x05): tag does not exist
- `StatusServiceNotSupported` (0x08): the object doesn't support the requested service
- `StatusObjectDoesNotExist` (0x16): the target CIP class doesn't exist
- `StatusInvalidAttributeValue` (0x09): the write value is out of range

These errors are **deterministic** -- retrying the same request will produce the same result. The client returns them immediately to the caller without retrying.

### How to Tell Them Apart in Code

```go
data, err := client.ReadTag(ctx, tagName)
if err != nil {
    var cipErr cip.Error
    if errors.As(err, &cipErr) {
        // CIP error: the PLC rejected your request
        fmt.Printf("CIP error: status=0x%02X\n", cipErr.Status)
    } else {
        // Transport error: connection problem (should have been retried)
        fmt.Printf("Transport error: %v\n", err)
    }
}
```

## Retry Semantics

| Setting | Value | Meaning |
|---------|-------|---------|
| `WithRetries(-1)` | Infinite | Never give up on transport errors |
| `WithRetries(0)` | No retries | Fail immediately on any error |
| `WithRetries(3)` | 3 retries | Try up to 4 times total (1 initial + 3 retries) |
| `WithRetryDelay(2s)` | 2 seconds | Pause between retry attempts |

The retry loop interacts with the reconnecting transport:

1. **First attempt**: uses the existing session (or creates one lazily)
2. **On transport error**: resets the session, waits `retryDelay`, gets a new session
3. **On CIP error**: returns immediately (no retry)
4. **On context cancellation**: returns immediately

## Usage

```bash
# Poll a tag every 2 seconds (default settings)
go run . -addr 192.168.1.100:44818 -tag MyDINT

# Poll every 500ms with a custom tag
go run . -addr 192.168.1.100:44818 -tag Temperature -interval 500ms

# Test with the server example (in another terminal)
go run ../server -addr :44818
# Then in this terminal:
go run . -addr 127.0.0.1:44818 -tag MyDINT -interval 1s
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `127.0.0.1:44818` | EtherNet/IP server address (`host:port`) |
| `-tag` | `MyDINT` | Tag name to read |
| `-interval` | `2s` | Polling interval (Go duration format) |

### Testing Reconnection

To verify the reconnection behavior:

1. Start the server example: `go run ../server -addr :44818`
2. Start this client: `go run . -addr 127.0.0.1:44818 -tag MyDINT -interval 1s`
3. Observe successful reads
4. Stop the server (Ctrl+C)
5. Observe the client logging retry/reconnection attempts
6. Restart the server
7. Observe the client automatically reconnecting and resuming reads

## Lifecycle Hooks

The `OnConnect` and `OnDisconnect` hooks are called by the transport layer:

- **OnConnect**: called inside `Conn()` after a successful `connector.Connect()`. This happens on the first connection and every reconnection.
- **OnDisconnect**: called inside `Reset()` or `Close()` after `closer.Close()`. The error parameter is the result of closing the old session (may be nil).

Common uses for lifecycle hooks:

| Hook | Use Case |
|------|----------|
| OnConnect | Log reconnection events, reset watchdogs, re-initialize state |
| OnDisconnect | Set outputs to safe state, trigger alarms, start failover timers |
