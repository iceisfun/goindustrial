# Condition Monitor - Lua Scripting Example

This example demonstrates using Lua to evaluate **compound boolean conditions
with per-signal time qualifications** against live PLC data. The Go host
provides PLC connectivity and timing primitives; the Lua script defines which
signals to read, how to compose them into conditions, and what hold times to
require before a condition fires.

## Why Lua for condition monitoring?

Industrial machines often need to evaluate logic like "the E-stop must be clear
**and** the run signal must have been stable for at least one second before the
machine is allowed to operate." These rules vary by site, machine, and safety
requirements. Embedding the condition logic in Lua means:

- **Condition rules can change without recompiling** -- an integrator edits a
  `.lua` file and restarts the process.
- **Time qualifications are explicit** -- each signal can specify its own hold
  time to debounce transient states and prevent false triggers.
- **Callbacks fire on transitions** -- `on_active` and `on_clear` are called
  when a condition becomes true or false, not every scan cycle.
- **Go handles the hard parts** -- PLC connectivity, timing, and the scan loop
  are provided by the Go host as native functions.

## How it works

### Go host (`main.go`)

The Go program creates a GoLua VM with the goindustrial modules and two
additional native functions:

| Global        | Type     | Description                                    |
|---------------|----------|------------------------------------------------|
| `PLC_ADDR`    | string   | PLC address from the `-addr` flag              |
| `POLL_MS`     | integer  | Poll interval from the `-poll` flag            |
| `MAX_CYCLES`  | integer  | Cycle limit from the `-cycles` flag (0 = unlimited) |
| `clock_ms()`  | function | Monotonic milliseconds since program start     |
| `sleep_ms(n)` | function | Pause execution for `n` milliseconds           |

The `industrialLua.Open(v)` call registers the `modbus` and `eip` globals for
PLC communication.

### Lua script

The built-in script implements three components:

**1. Signal tracker** -- records each signal's current value and when it last
changed. This is what makes time-qualified conditions possible.

```lua
local sig = { val = {}, since = {} }

function sig:set(name, new_val, now)
    if self.val[name] ~= new_val then
        self.since[name] = now
    end
    self.val[name] = new_val
end

function sig:held(name, expected, ms, now)
    if self.val[name] ~= expected then return false end
    return (now - (self.since[name] or now)) >= ms
end
```

`sig:held("estop_clear", true, 500, now)` returns `true` only if
`estop_clear` has been continuously `true` for at least 500 milliseconds.

**2. Condition engine** -- conditions are boolean expressions over signals.
Each condition tracks its own active/cleared state and fires callbacks only on
transitions:

```lua
define("safe_to_run", {
    eval = function(now)
        return sig:get("estop") == false
           and sig:get("is_running") == true
           and sig:held("estop_clear", true, 500, now)
           and sig:held("is_running", true, 1000, now)
    end,
    on_active = function() print("ACTIVE  safe_to_run") end,
    on_clear  = function() print("CLEARED safe_to_run") end,
})
```

**3. Scan loop** -- reads PLC signals, updates the tracker, evaluates all
conditions, and sleeps for `POLL_MS` between cycles.

### Built-in conditions

| Condition      | Expression                                                           | Purpose                         |
|---------------|----------------------------------------------------------------------|---------------------------------|
| `safe_to_run`  | `!estop AND is_running AND estop_clear FOR 500ms AND is_running FOR 1s` | Machine cleared to operate      |
| `drive_fault`  | `is_running AND !drive_ready FOR 200ms`                              | Debounced drive fault detection |
| `estop_active` | `estop == true`                                                      | Immediate E-stop detection      |

## Running the example

Connect to an EtherNet/IP PLC:

```bash
go run ./examples/lua/condition_monitor/ -addr 192.168.1.20
```

Faster polling with a cycle limit:

```bash
go run ./examples/lua/condition_monitor/ -addr 192.168.1.20 -poll 50 -cycles 500
```

Run a custom condition script:

```bash
go run ./examples/lua/condition_monitor/ -addr 192.168.1.20 -script my_conditions.lua
```

### Flags

| Flag      | Default              | Description                                              |
|-----------|----------------------|----------------------------------------------------------|
| `-addr`   | `192.168.1.10:44818` | PLC address as `host` or `host:port`                     |
| `-poll`   | `100`                | Poll interval in milliseconds                            |
| `-cycles` | `0`                  | Max scan cycles (0 = run until interrupted)              |
| `-script` | *(empty)*            | Path to a `.lua` file; if omitted the built-in demo runs |

## Adapting the tag names

The built-in script reads four boolean tags. Change the `tags` table to match
your PLC program:

```lua
local tags = {
    { name = "estop",       tag = "EStop" },
    { name = "is_running",  tag = "MachineRunning" },
    { name = "estop_clear", tag = "EStopReset" },
    { name = "drive_ready", tag = "DriveReady" },
}
```

The `name` field is the friendly name used in conditions. The `tag` field is the
actual PLC tag name passed to `client:read_tag()`.

## Using Modbus instead of EtherNet/IP

Replace the EtherNet/IP connection and tag reads with Modbus coil reads. The
condition engine and signal tracker work identically:

```lua
local client = modbus.connect(MODBUS_ADDR, { port = 502, unit = 1 })

-- In the scan loop, read coils instead of tags:
local coils = client:read_coils(0, 4)
sig:set("estop",       coils[1], now)
sig:set("is_running",  coils[2], now)
sig:set("estop_clear", coils[3], now)
sig:set("drive_ready", coils[4], now)
```

## Expected output

```
Connected to 192.168.1.20
Monitoring 4 signals, 3 conditions — poll every 100ms
----------------------------------------------------------------
[     0.5s] ESTOP   emergency stop engaged
[     3.2s] CLEARED e-stop released
[     3.7s] ACTIVE  safe_to_run — machine cleared to operate
[     5.0s] scan #50  estop=false  is_running=true  estop_clear=true  drive_ready=true  active=[safe_to_run]
[     8.1s] ALARM   drive_fault — drive not ready while running
[     8.3s] CLEARED safe_to_run
[     9.5s] CLEARED drive_fault
[    10.0s] scan #100  estop=false  is_running=true  estop_clear=true  drive_ready=true  active=[safe_to_run]
```

Key things to notice:

- `safe_to_run` does not fire immediately when conditions are met -- it waits
  for the hold times (500ms for estop_clear, 1000ms for is_running).
- `drive_fault` is debounced at 200ms, so momentary drive-ready glitches during
  acceleration do not trigger the alarm.
- `estop_active` fires immediately with no hold time.
- The periodic status line shows raw signal values and which conditions are
  currently active.

## Writing custom condition scripts

Create a `.lua` file with your own signal definitions and conditions. The Go
host provides `clock_ms()`, `sleep_ms()`, `PLC_ADDR`, `POLL_MS`, `MAX_CYCLES`,
and the full `eip` and `modbus` modules.

You can copy the signal tracker and condition engine from the built-in script
and add your own conditions:

```lua
define("high_pressure", {
    eval = function(now)
        local psi = sig:get("pressure")
        return type(psi) == "number" and psi > 150.0
           and sig:held("pressure_high", true, 3000, now)
    end,
    on_active = function()
        print("ALARM: sustained high pressure for 3 seconds")
    end,
    on_clear = function()
        print("CLEARED: pressure returned to normal")
    end,
})
```

For analog thresholds, compute a derived boolean signal in the scan loop and
track it like any other signal:

```lua
-- In the scan loop, after reading the analog tag:
local psi = sig:get("pressure")
local was_high = sig:get("pressure_high")
local is_high = (type(psi) == "number" and psi > 150.0)
sig:set("pressure_high", is_high, now)
```

This lets `sig:held("pressure_high", true, 3000, now)` work correctly for
analog values.
