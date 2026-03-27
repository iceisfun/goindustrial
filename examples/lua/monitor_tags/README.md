# Monitor Tags - Lua Scripting Example

This example demonstrates how to use a Lua script to continuously monitor PLC
tags over EtherNet/IP and detect value changes between poll cycles. It combines
tag discovery, batch reading, and change tracking in a compact script -- showing
the kind of runtime-configurable monitoring logic that Lua scripting makes
possible without recompiling Go code.

## Why Lua-driven tag monitoring?

In industrial data collection, a common requirement is to poll a set of PLC tags
at regular intervals and react when values change. Different sites, machines, or
production lines may need to monitor entirely different tags with different
alerting rules. Embedding the monitoring logic in Lua means:

- **Site-specific monitoring** -- each deployment can have its own `.lua` script
  that selects which tags to watch, without rebuilding the Go binary.
- **Dynamic tag selection** -- the script can discover tags at startup using
  `list_tags()` and choose which ones to monitor based on name patterns, data
  types, or any other criteria.
- **Change detection and alerting** -- Lua logic tracks previous values and can
  print, log, or flag changes however you need.
- **Non-developer configuration** -- an integrator who understands the PLC
  program can write a Lua script without Go knowledge.

## How it works

The Go host (`main.go`) creates a GoLua VM with the goindustrial modules loaded
and sets two globals:

- `EIP_ADDR` -- the PLC address from the `-addr` flag.
- `POLL_CYCLES` -- the number of poll iterations from the `-cycles` flag.

The built-in Lua script then:

1. Connects to the PLC using `eip.connect()`.
2. Calls `client:list_tags()` to discover all tags.
3. Selects up to 3 scalar DINT tags (type code `0x00C4`) to monitor.
4. Enters a loop for `POLL_CYCLES` iterations. Each iteration:
   - Calls `client:read_tags()` to batch-read all monitored tags.
   - Compares each value to its previous value.
   - Prints a summary line. Tags whose values changed since the last cycle are
     marked with `*`.
5. Closes the connection.

## Running the example

Monitor tags on a PLC for 5 cycles (the default):

```
go run ./examples/lua/monitor_tags/ -addr 192.168.1.20
```

Specify a different number of poll cycles:

```
go run ./examples/lua/monitor_tags/ -addr 192.168.1.20 -cycles 20
```

Run a custom monitoring script:

```
go run ./examples/lua/monitor_tags/ -addr 192.168.1.20 -script custom_monitor.lua
```

### Flags

| Flag      | Default                 | Description                                        |
|-----------|-------------------------|----------------------------------------------------|
| `-addr`   | `192.168.1.10:44818`    | PLC address as `host` or `host:port`               |
| `-cycles` | `5`                     | Number of poll cycles (built-in script only)        |
| `-script` | *(empty)*               | Path to a `.lua` file; if omitted the built-in demo runs |

Note: the `-cycles` flag sets the `POLL_CYCLES` global, which the built-in
script reads. Custom scripts can use this global or ignore it.

## Lua API used in this example

This example uses the `eip` module. Here is a summary of the methods the
built-in script calls. See the `ethernetip_client` example for the full API
reference.

### Connecting

```lua
local client = eip.connect(EIP_ADDR, {
    retries = 3,
    timeout = 10,
})
```

### Discovering tags

```lua
local tags = client:list_tags()
```

Returns a 1-based table where each entry has `.id` (integer), `.name` (string),
and `.type` (integer CIP type code). The built-in script filters for
`tags[i].type == 0x00C4` (DINT) and collects up to 3 tag names.

### Batch reading

```lua
local values = client:read_tags({"TagA", "TagB", "TagC"})
```

Reads multiple tags and returns a table of values in the same order. Each value
is automatically converted to the appropriate Lua type (integer, float, boolean,
or string) based on the CIP type code in the PLC response.

### Closing

```lua
client:close()
```

## Error handling with pcall

The built-in script wraps the `read_tags` call in `pcall` so that a transient
read failure does not terminate the entire monitoring loop:

```lua
local ok, values = pcall(function()
    return client:read_tags(monitor_tags)
end)

if ok then
    -- process values
else
    print("ERROR - " .. tostring(values))
end
```

`pcall` (protected call) is Lua's error handling mechanism. It calls the given
function in protected mode: if the function raises an error, `pcall` returns
`false` and the error message. If the function succeeds, `pcall` returns `true`
followed by any return values. This is the Lua equivalent of Go's
`if err != nil` pattern.

Using `pcall` around reads in a monitoring loop is important because industrial
networks can be unreliable -- a single timeout or CIP error should not crash
your monitoring script.

## Expected output

The built-in script produces output similar to:

```
Connected to 192.168.1.20
Monitoring 3 tags for 5 cycles:
  [1] Program:MainProgram.RunCount
  [2] Program:MainProgram.MotorSpeed
  [3] Program:MainProgram.BatchCount
------------------------------------------------------------
Cycle 1:  Program:MainProgram.RunCount=4821  Program:MainProgram.MotorSpeed=1750  Program:MainProgram.BatchCount=12
Cycle 2:  Program:MainProgram.RunCount=4822 *  Program:MainProgram.MotorSpeed=1750  Program:MainProgram.BatchCount=12
Cycle 3:  Program:MainProgram.RunCount=4823 *  Program:MainProgram.MotorSpeed=1750  Program:MainProgram.BatchCount=12
Cycle 4:  Program:MainProgram.RunCount=4824 *  Program:MainProgram.MotorSpeed=1748 *  Program:MainProgram.BatchCount=12
Cycle 5:  Program:MainProgram.RunCount=4825 *  Program:MainProgram.MotorSpeed=1748  Program:MainProgram.BatchCount=13 *

Done.
```

Key things to notice:

- The first cycle never shows `*` markers because there is no previous value to
  compare against.
- Subsequent cycles mark changed values with `*`. In this example, `RunCount`
  increments every cycle, `MotorSpeed` changes occasionally, and `BatchCount`
  changes once.
- If a read error occurs, the cycle prints `ERROR` and the error message instead
  of values.

The actual tag names, values, and change patterns depend entirely on the PLC
program running on the target controller.

## Writing custom monitoring scripts

Create a `.lua` file and pass it with `-script`. Your script has access to the
`EIP_ADDR` and `POLL_CYCLES` globals injected by the Go host, plus the full
`eip` module (and `modbus` module, if needed) and the Lua standard library.

Example custom script (`custom_monitor.lua`):

```lua
local client = eip.connect(EIP_ADDR, {
    retries = 3,
    timeout = 10,
})

-- Monitor specific tags by name rather than discovering them.
local watch_list = {
    "Program:MainProgram.Temperature",
    "Program:MainProgram.Pressure",
    "Program:MainProgram.FlowRate",
}

local prev = {}

for cycle = 1, POLL_CYCLES do
    local ok, values = pcall(function()
        return client:read_tags(watch_list)
    end)

    if ok then
        for i, name in ipairs(watch_list) do
            local val = values[i]
            if prev[name] ~= nil and prev[name] ~= val then
                print(string.format("CHANGE: %s  %s -> %s",
                    name, tostring(prev[name]), tostring(val)))
            end
            prev[name] = val
        end
    else
        print("Read error: " .. tostring(values))
    end
end

client:close()
```

Run it:

```
go run ./examples/lua/monitor_tags/ -addr 192.168.1.20 -cycles 100 -script custom_monitor.lua
```

This script only prints output when a value changes, which is useful for
long-running monitors where you only care about state transitions.
