# Monitor Subscriber Example

This example demonstrates the **Subscriber** API for consuming monitor events.
Subscribers are independent consumers that each receive a copy of every event
through their own buffered channel. A slow subscriber never blocks the monitor
or other subscribers -- events are silently dropped when the buffer is full.

## What This Example Does

1. Connects to a **Modbus TCP server**
2. Creates a Monitor and subscribes to a holding register with change detection
3. Creates **two Subscribers** that each receive all events
4. Runs both subscribers concurrently using `for evt := range sub.All()`
5. Subscriber A logs every event; Subscriber B only prints changes
6. Shuts down cleanly on Ctrl+C

## Subscriber Architecture

The Subscriber API is a broadcast fan-out model. The monitor produces events
from all polling subscriptions, and each Subscriber independently receives a
copy:

```
  Monitor (polls PLC data points)
     │
     │  broadcast (non-blocking)
     │
     ├─── Subscriber A (buffer=128)  → logs all events
     ├─── Subscriber B (buffer=128)  → prints changes only
     └─── Events() channel           → legacy channel-based consumption
```

Each Subscriber has its own buffered channel. When the monitor emits an event,
it attempts a non-blocking send to each Subscriber. If a Subscriber's buffer is
full, the event is dropped for that Subscriber without affecting the monitor or
other Subscribers.

### Subscriber vs Events()

The original `Events()` channel is a single shared channel. If you have
multiple goroutines reading from it, each event goes to exactly one reader
(competing consumers). Subscribers solve this: each Subscriber gets every event
(broadcast).

| Approach | Pattern | Multiple consumers |
|----------|---------|--------------------|
| `Events()` | Single channel, competing readers | Each event goes to one reader |
| `NewSubscriber()` | Per-consumer channel, broadcast | Each subscriber gets every event |

## Subscriber API

### Creating a Subscriber

```go
sub, err := mon.NewSubscriber(128)  // 128-event buffer
if err != nil {
    log.Fatal(err)
}
defer sub.Done()
```

`NewSubscriber(bufferSize)` creates a Subscriber with a buffered channel of the
given size. Returns `ErrMonitorClosed` if the monitor has been closed.

### Consuming with for-range (iter.Seq)

Subscribers implement `iter.Seq[Event]` through the `All()` method, so you can
use them directly in a for-range loop:

```go
for evt := range sub.All() {
    if evt.Err != nil {
        log.Println("read error:", evt.Err)
        continue
    }
    if evt.Changed {
        fmt.Printf("changed: %s = %x\n", evt.Snapshot.Point, evt.Snapshot.Value.Raw)
    }
}
```

The iterator yields events until `Done()` is called or the monitor is closed.

### Consuming with the channel directly

If you prefer `select` or need to multiplex with other channels:

```go
for {
    select {
    case evt, ok := <-sub.Events():
        if !ok {
            return  // subscriber closed
        }
        process(evt)
    case <-ctx.Done():
        return
    }
}
```

### Done()

`Done()` unregisters the Subscriber from the monitor and closes its event
channel. It is idempotent (safe to call multiple times) and should be deferred
immediately after creating the Subscriber:

```go
sub, err := mon.NewSubscriber(128)
if err != nil {
    log.Fatal(err)
}
defer sub.Done()
```

After `Done()`:
- The Subscriber's channel is closed
- Any `for range sub.All()` loop terminates
- No more events are delivered to this Subscriber
- The monitor continues running for other Subscribers

### Monitor.Close()

When the monitor is closed, all Subscriber channels are closed automatically.
You do not need to call `Done()` before `Close()`, though it is harmless to do
so.

## Running the example

Start a Modbus server in one terminal:

```bash
go run ./examples/modbus/server/ -port 5020
```

Run the subscriber example in another:

```bash
go run ./examples/monitor_subscriber/ -modbus-addr 127.0.0.1 -modbus-port 5020
```

Use a write example in a third terminal to change register values and observe
both subscribers reacting:

```bash
go run ./examples/modbus/write_registers/ -addr 127.0.0.1 -port 5020
```

### Flags

| Flag            | Default     | Description                        |
|-----------------|-------------|------------------------------------|
| `-modbus-addr`  | `127.0.0.1` | Modbus TCP server address          |
| `-modbus-port`  | `502`       | Modbus TCP port                    |
| `-register`     | `0`         | Holding register address to monitor|
| `-interval`     | `1s`        | Poll interval                      |

## Expected output

```
Connecting to Modbus TCP server at 127.0.0.1:5020...
Connected.
Monitoring HoldingRegister(0, 1) with 2 subscribers — poll every 1s
Press Ctrl+C to stop.

[A] 14:32:01.003  #1  HoldingRegister(0, 1) = 0 (0x0000) *
[B] 14:32:01.003  CHANGED  HoldingRegister(0, 1) → 0 (0x0000)
[A] 14:32:02.012  #2  HoldingRegister(0, 1) = 0 (0x0000)
[A] 14:32:03.008  #3  HoldingRegister(0, 1) = 0 (0x0000)
[A] 14:32:04.015  #4  HoldingRegister(0, 1) = 1234 (0x04D2) *
[B] 14:32:04.015  CHANGED  HoldingRegister(0, 1) → 1234 (0x04D2)
[A] 14:32:05.010  #5  HoldingRegister(0, 1) = 1234 (0x04D2)

^C
Received interrupt, shutting down...
[A] subscriber done
[B] subscriber done
Done.
```

Key things to notice:

- **Subscriber A** logs every event with a sequence number. Changed events are
  marked with `*`.
- **Subscriber B** only prints when a value changes, filtering out unchanged
  polls.
- Both subscribers receive the same events -- they are independent consumers.
- On Ctrl+C, `Monitor.Close()` closes all subscriber channels, which causes
  both `for range sub.All()` loops to terminate.

## Common patterns

### Filtered subscriber

Process only events for a specific subscription ID:

```go
for evt := range sub.All() {
    if evt.SubscriptionID != mySubID {
        continue
    }
    // ...
}
```

### Timeout with subscriber

Use the channel directly if you need a timeout:

```go
select {
case evt, ok := <-sub.Events():
    if !ok {
        return
    }
    process(evt)
case <-time.After(10 * time.Second):
    log.Println("no events for 10 seconds")
}
```

### Multiple monitors, one subscriber each

```go
subModbus, _ := modbusMon.NewSubscriber(128)
defer subModbus.Done()

subEIP, _ := eipMon.NewSubscriber(128)
defer subEIP.Done()

// Use select to multiplex:
for {
    select {
    case evt, ok := <-subModbus.Events():
        if !ok { continue }
        handleModbus(evt)
    case evt, ok := <-subEIP.Events():
        if !ok { continue }
        handleEIP(evt)
    }
}
```
