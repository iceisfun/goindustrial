# pccc_monitor — poll PCCC data-table addresses for changes

Wraps `pccc.Client` in the protocol-agnostic `monitor` package. Subscribes
to one or more SLC addresses and prints every value change.

## Usage

```bash
# Poll a single integer at 500 ms:
go run . -addr 10.30.40.71:44818 -tags N7:0

# Poll several addresses, all at 200 ms:
go run . -addr 10.30.40.71:44818 \
  -tags N7:0,N7:1,F8:5,B3:0/2,T4:0.ACC \
  -freq 200ms
```

## Why this works

`pccc.Client` implements `plc.Reader`, so it slots into
`monitor.NewMonitor` exactly like `modbus.Client` or `ethernetip.Client`.
Subscribed `pccc.File` data points are read on the configured cadence; the
monitor compares each result to the previous one and emits an `Event` with
`Changed = true` whenever the value differs.
