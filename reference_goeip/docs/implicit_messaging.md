# Implicit Messaging (UDP I/O)

Implicit Messaging (also known as Class 1 I/O) provides real-time, cyclic data exchange between a Scanner and an Adapter over UDP.

## Architecture

The `pkg/runtime` package implements the engine for Implicit Messaging. It handles:

1. **UDP Listener**: Listens on port 2222 (default) for incoming I/O packets.
2. **Scheduler**: Manages the transmission of cyclic packets based on the Requested Packet Interval (RPI).
3. **Watchdog**: Monitors incoming packets and times out connections if data is not received within the specified window (RPI * Multiplier).

## Packet Format

`goeip` implements the standard EtherNet/IP I/O packet format:

- **Item Count**: 2
- **Address Item**: Connected Address Item (0xA1) containing the Connection ID.
- **Data Item**: Connected Data Item (0xB1) containing:
  - **Sequence Count**: 16-bit incrementing counter.
  - **Run/Idle Header**: 32-bit header (optional, usually present for O->T).
  - **Payload**: The actual Assembly data.

## Runtime Usage

The `Runtime` struct coordinates the UDP socket and the connections.

```go
import "github.com/iceisfun/goeip/pkg/runtime"

// Initialize Runtime with reference to Assembly Object
rt := runtime.NewRuntime(ao)

// Start listening on UDP port 2222
err := rt.Start(":2222")
```

### Adding Connections

Connections are typically added automatically by the Connection Manager when a `Forward_Open` succeeds, or manually by a Scanner application.

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

### Run/Idle Header

The 32-bit Run/Idle header is supported.
- **Bit 0**: Run Mode (1 = Run, 0 = Idle).
- **Bits 1-31**: Reserved (0).

When acting as a Scanner (Producer O->T), `goeip` injects this header. When acting as an Adapter (Consumer O->T), `goeip` parses and validates it.
