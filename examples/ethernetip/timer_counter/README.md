# Timer and Counter Example

Read Timer (TON/TOF/RTO) and Counter (CTU/CTD/CTUD) structured tags from a
Rockwell Logix PLC over EtherNet/IP.

## What This Example Does

This program connects to a Logix controller and reads one or both of:

- A **Timer** tag (created by TON, TOF, or RTO ladder instructions)
- A **Counter** tag (created by CTU, CTD, or CTUD ladder instructions)

It decodes the 14-byte Rockwell structure and displays every field, including
the individual status bits packed inside a DINT.

## Rockwell Timer/Counter Memory Layout

Both Timer and Counter tags share an identical 14-byte binary structure. This
is the canonical Rockwell Logix memory layout as returned by CIP Read Tag:

```
Byte Offset   Size    Field
-----------   ------  ------------------------------------------
0-1           INT     Reserved (2 bytes, typically 0x0000)
2-5           DINT    Status Bits (boolean flags packed as bits)
6-9           DINT    PRE (Preset value)
10-13         DINT    ACC (Accumulated value)
-----------   ------  ------------------------------------------
Total: 14 bytes
```

### Timer Status Bits (offset 2-5, interpreted as uint32)

| Bit | Name | Description |
|-----|------|-------------|
| 31  | EN   | **Enable** -- true while the timer instruction's rung-in condition is true |
| 30  | TT   | **Timer Timing** -- true while EN is true and ACC < PRE (timer is actively counting) |
| 29  | DN   | **Done** -- true when ACC >= PRE (the timer has completed) |

Timer instructions:
- **TON** (Timer On-Delay): Starts counting when rung-in goes true. DN sets after PRE milliseconds.
- **TOF** (Timer Off-Delay): Starts counting when rung-in goes false. DN clears after PRE milliseconds.
- **RTO** (Retentive Timer On): Like TON but ACC is retained when rung-in goes false. Must be explicitly reset.

PRE and ACC are in **milliseconds**. A 5-second timer has PRE = 5000.

### Counter Status Bits (offset 2-5, interpreted as uint32)

| Bit | Name | Description |
|-----|------|-------------|
| 31  | CU   | **Count Up** -- set on the rising edge of a CTU instruction's rung-in |
| 30  | CD   | **Count Down** -- set on the rising edge of a CTD instruction's rung-in |
| 29  | DN   | **Done** -- true when ACC >= PRE |
| 28  | OV   | **Overflow** -- set when ACC increments past +2,147,483,647 (max int32) |
| 27  | UN   | **Underflow** -- set when ACC decrements past -2,147,483,648 (min int32) |

Counter instructions:
- **CTU** (Count Up): Increments ACC on each false-to-true rung transition.
- **CTD** (Count Down): Decrements ACC on each false-to-true rung transition.
- **CTUD** (Count Up/Down): Supports both directions with separate enable inputs.

PRE and ACC are **dimensionless integer counts**.

### CIP Response Format for Structs

When you read a structured tag, the CIP response has a 4-byte header instead of
the usual 2-byte header for atomic types:

```
Bytes 0-1:  CIP type code (>= 0x02A0 for STRUCT)
Bytes 2-3:  Structure handle (identifies the specific struct definition)
Bytes 4+:   Structure data (14 bytes for Timer/Counter)
```

The structure handle is assigned by the controller firmware and varies between
controller models and firmware revisions. The goindustrial library handles this
automatically.

## How to Run

```bash
# Read both a Timer and a Counter
go run ./examples/ethernetip/timer_counter \
  -addr 192.168.1.10:44818 \
  -timer-tag Timer_1 \
  -counter-tag Counter_1

# Read only a Timer
go run ./examples/ethernetip/timer_counter \
  -addr 192.168.1.10:44818 \
  -timer-tag MyTON

# Read only a Counter
go run ./examples/ethernetip/timer_counter \
  -addr 192.168.1.10:44818 \
  -counter-tag PartCount
```

## Expected Output

```
Connected to 192.168.1.10:44818

=== Timer: Timer_1 ===

  PRE (Preset):       5000 ms  (5.0 seconds)
  ACC (Accumulated):  3200 ms  (3.2 seconds)
  Progress:           64.0%

  EN  (Enable):       true
  TT  (Timer Timing): true
  DN  (Done):         false

  State: Timing (actively counting up)

=== Counter: Counter_1 ===

  PRE (Preset):       100
  ACC (Accumulated):  42
  Progress:           42.0%

  CU  (Count Up):     true
  CD  (Count Down):   false
  DN  (Done):         false
  OV  (Overflow):     false
  UN  (Underflow):    false

  State: Counting

Done.
```

## Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `insufficient data for Timer: expected at least 14 bytes, got N` | The tag is not a Timer/Counter structure | Verify the tag name points to a TON/TOF/RTO/CTU/CTD instruction output |
| `CIP Error: Status=0x05` | Tag not found | Check the tag name in Studio 5000 |
| `response too short for type header` | Response did not contain the expected 4-byte struct header | May indicate a firmware incompatibility |
| `dial tcp ...: connection refused` | PLC unreachable | Verify IP address, port 44818, and firewall |

## Monitoring Timers in Real Time

To continuously monitor a timer's progress, you can wrap the read in a loop:

```go
for {
    timer, err := client.ReadTimer(ctx, "Timer_1")
    if err != nil {
        log.Printf("read error: %v", err)
        continue
    }
    fmt.Printf("\rACC=%5d / PRE=%5d  EN=%v TT=%v DN=%v",
        timer.ACC, timer.PRE, timer.EN, timer.TT, timer.DN)
    time.Sleep(100 * time.Millisecond)
}
```

This polls at 10 Hz, which is typical for HMI-style monitoring. For
production use, consider the `monitor` package which provides a polling
loop with configurable intervals and error handling.

## Tag Naming Conventions

In a Logix program, Timer and Counter tags are usually named after their
instruction. For example, a rung with `TON Timer_1 5000 0` creates a tag
called `Timer_1` of type `TIMER`. You can also access individual members:

- `Timer_1.PRE` -- just the preset (DINT)
- `Timer_1.ACC` -- just the accumulated (DINT)
- `Timer_1.EN` -- just the enable bit (BOOL)
- `Timer_1.DN` -- just the done bit (BOOL)

Reading the full structure tag (`Timer_1` without a member) returns all 14
bytes in a single CIP transaction, which is more efficient than reading
each member individually.
