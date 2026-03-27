# Cross-Protocol Monitor Example

This example demonstrates the **monitor** package, which provides a unified way to poll data points from industrial controllers and detect value changes. It shows monitoring both **Modbus TCP** and **EtherNet/IP** data simultaneously using the same Monitor API.

## What This Example Does

1. Connects to a **Modbus TCP server** and an **EtherNet/IP server**
2. Creates **two Monitor instances** (one per protocol, since each monitor wraps a single `plc.Reader`)
3. Subscribes to a Modbus holding register with change detection and a handler callback
4. Subscribes to an EtherNet/IP tag with change detection and a handler callback
5. Consumes events from **both** monitors in a unified event loop
6. Handles **graceful shutdown** via OS signals

## Monitor Architecture

The monitor package is protocol-agnostic. It works with any type that implements the `plc.Reader` interface:

```go
type Reader interface {
    Read(ctx context.Context, points ...DataPoint) ([]Value, error)
}
```

Both `modbus.Client` and `ethernetip.Client` implement this interface, so they can be used interchangeably with the monitor.

```
                   Your Application
                  /                \
    monitor.Monitor              monitor.Monitor
    (Modbus reader)              (EtherNet/IP reader)
         |                            |
    modbus.Client                ethernetip.Client
    (plc.Reader)                 (plc.Reader)
         |                            |
    Modbus TCP Server           EtherNet/IP Server
```

### How the Monitor Works Internally

When you call `monitor.Subscribe()`, the monitor creates a dedicated goroutine for that subscription. The goroutine runs a poll loop:

1. Wait for the configured frequency (plus optional random variance)
2. Call `reader.Read(ctx, point)` to get the current value
3. Compare with the previous value using the change detector (if configured)
4. Call the per-subscription handler (if configured)
5. Emit an Event to the monitor's event channel
6. Go back to step 1

Each subscription runs independently, so they can have different frequencies and do not block each other.

### Key Components

| Component | Purpose |
|-----------|---------|
| `monitor.Monitor` | Manages subscriptions and the event channel |
| `monitor.Subscription` | A handle to a running poll goroutine |
| `monitor.Event` | Emitted on each poll cycle (success or error) |
| `monitor.Snapshot` | Contains the data point, raw value, and timestamp |
| `monitor.ChangeDetector` | Interface for detecting value changes |
| `monitor.ByteChangeDetector` | Simple byte comparison change detector |
| `monitor.Handler` | Per-subscription callback function |

## Change Detection

The monitor supports pluggable change detection through the `ChangeDetector` interface:

```go
type ChangeDetector interface {
    Detect(prev, curr Snapshot) bool
}
```

The built-in `ByteChangeDetector` compares raw byte slices using `bytes.Equal()`. This works well for discrete values (integers, booleans) but may produce excessive "changed" events for floating-point values that fluctuate slightly.

For floating-point values, you could implement a custom deadband detector:

```go
type DeadbandDetector struct {
    Threshold float64
}

func (d DeadbandDetector) Detect(prev, curr monitor.Snapshot) bool {
    prevVal := parseFloat(prev.Value.Raw)
    currVal := parseFloat(curr.Value.Raw)
    return math.Abs(currVal - prevVal) > d.Threshold
}
```

### Event.Changed Field

The `Changed` field in the Event struct reflects what the change detector reported:

- If **no change detector** is configured, `Changed` is always `true` (every event is treated as a change)
- If a change detector is configured, `Changed` is `true` only when `Detect()` returns `true`
- On the **first read** (no previous value to compare), `Changed` is always `true`
- On **read errors**, `Changed` is `false` (there is no new value to compare)

## Two Ways to Consume Events

### 1. Per-Subscription Handlers (Callback Style)

Use `monitor.WithHandler(fn)` when subscribing. The handler is called synchronously in the subscription's poll goroutine after every successful read:

```go
mon.Subscribe(point,
    monitor.WithHandler(func(snap monitor.Snapshot) {
        fmt.Printf("Value: %v\n", snap.Value.Raw)
    }),
)
```

**Pros**: Simple, per-subscription logic without checking IDs
**Cons**: Runs in the poll goroutine, so slow handlers delay subsequent polls

### 2. Unified Event Channel

Use `mon.Events()` to get a receive-only channel of all events from all subscriptions:

```go
for event := range mon.Events() {
    if event.Err != nil {
        log.Printf("Error: %v", event.Err)
        continue
    }
    if event.Changed {
        log.Printf("Changed: sub=%d value=%v", event.SubscriptionID, event.Snapshot.Value.Raw)
    }
}
```

**Pros**: Centralized processing, can batch events, decouple from poll timing
**Cons**: Need to check `SubscriptionID` to identify the source

You can use **both** approaches simultaneously. The handler fires first, then the event is emitted to the channel.

## Subscription Options

| Option | Description |
|--------|-------------|
| `WithFrequency(d)` | Poll interval (e.g., `100*time.Millisecond`). Default: 500ms |
| `WithReadVariance(d)` | Random jitter added to poll timing to prevent thundering herd |
| `WithChangeDetector(d)` | Attach a change detector (e.g., `ByteChangeDetector{}`) |
| `WithHandler(fn)` | Per-subscription callback fired after each successful read |
| `WithInitialRead(d)` | Delay before the first read (0 = immediate). Default: 50ms |

## Monitor Options

| Option | Description |
|--------|-------------|
| `WithLogger(l)` | Attach a logger for diagnostics |
| `WithEventBuffer(n)` | Size of the internal event channel buffer. Default: 64 |

## Usage

```bash
# Monitor both Modbus and EtherNet/IP (requires both servers running)
go run . \
  -modbus-addr 127.0.0.1 -modbus-port 5020 -modbus-register 0 \
  -eip-addr 127.0.0.1:44818 -eip-tag MyDINT \
  -interval 1s
```

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-modbus-addr` | `127.0.0.1` | Modbus TCP server address |
| `-modbus-port` | `502` | Modbus TCP port |
| `-modbus-register` | `0` | Holding register address to monitor |
| `-eip-addr` | `127.0.0.1:44818` | EtherNet/IP server address |
| `-eip-tag` | `MyDINT` | EtherNet/IP tag name to monitor |
| `-interval` | `1s` | Poll interval for all subscriptions |

### Running the Full Demo

Open three terminals:

```bash
# Terminal 1: Start the Modbus server
cd examples/modbus/server && go run .

# Terminal 2: Start the EtherNet/IP server
cd examples/ethernetip/server && go run .

# Terminal 3: Run the monitor
cd examples/monitor_polling && go run . \
  -modbus-addr 127.0.0.1 -modbus-port 502 \
  -eip-addr 127.0.0.1:44818 \
  -interval 1s
```

Then use write examples in additional terminals to change values and observe the monitor detecting the changes.

## Graceful Shutdown

The monitor example handles `SIGINT` (Ctrl+C) and `SIGTERM`:

1. The signal handler cancels the context
2. The event loop exits
3. `modbusMon.Close()` and `eipMon.Close()` are called, which:
   - Signal all subscription goroutines to stop
   - Wait for them to finish (`sync.WaitGroup`)
   - Close the event channels
4. Client connections are closed via defer

## When to Use the Monitor

The monitor is ideal for:

- **Dashboard/HMI applications** that need to display live values
- **Alarm systems** that watch for out-of-range conditions
- **Data logging** where you want periodic snapshots of process values
- **Change-driven workflows** where actions should only trigger when values change

For high-frequency implicit I/O (sub-millisecond), use EtherNet/IP's native connected messaging instead of the monitor's polling approach.
