# Modbus TCP Client - Lua Scripting Example

This example demonstrates how to use Lua scripts to interact with a Modbus TCP
server through goindustrial's GoLua bindings. A small Go host program creates a
Lua virtual machine, loads the `modbus` module provided by the
`github.com/iceisfun/goindustrial/lua` package, and then executes a Lua script
that performs register and coil reads, writes, and data conversions -- all
without any Go recompilation.

## Why Lua scripting for industrial protocols?

Modbus TCP is the most common protocol for communicating with PLCs, power
meters, VFDs, and other industrial devices. In many real-world deployments you
need to:

- **Change data collection logic without rebuilding** -- operators or
  integrators can edit a `.lua` file and restart the process.
- **Prototype quickly** -- Lua scripts let you experiment with different
  register addresses and data interpretations interactively.
- **Deploy site-specific logic** -- each site may have different devices with
  different register maps. Lua scripts act as lightweight, portable
  configuration that includes logic.

The `modbus` Lua module gives scripts full access to the Modbus TCP function
codes supported by goindustrial, while the Go host handles connection
management, retries, and binary framing.

## How it works

The Go host (`main.go`) performs five steps:

1. **Parse flags** -- reads `-addr`, `-port`, and `-script` from the command
   line.
2. **Load the Lua source** -- either from a file (when `-script` is given) or
   from a built-in demo script embedded in the Go binary.
3. **Create a GoLua VM** -- `vm.New()` creates the virtual machine,
   `stdlib.Open(v)` loads standard Lua libraries (string, math, table, etc.),
   and `industrialLua.Open(v)` registers the `modbus` and `eip` global modules.
4. **Inject globals** -- the Go host sets `MODBUS_ADDR` (string) and
   `MODBUS_PORT` (integer) as Lua globals so the script knows where to connect.
5. **Run the script** -- the compiled bytecode executes inside the VM. Any
   errors raised by protocol operations propagate as Lua runtime errors.

## Running the example

Connect to a Modbus TCP server at a known address and port:

```
go run ./examples/lua/modbus_client/ -addr 10.2.150.23 -port 502
```

Connect to a local simulator on a non-standard port:

```
go run ./examples/lua/modbus_client/ -addr 127.0.0.1 -port 5020
```

Run a custom Lua script instead of the built-in demo:

```
go run ./examples/lua/modbus_client/ -addr 10.2.150.23 -port 502 -script my_registers.lua
```

### Flags

| Flag      | Default       | Description                                       |
|-----------|---------------|---------------------------------------------------|
| `-addr`   | `127.0.0.1`   | Modbus TCP server IP address or hostname          |
| `-port`   | `502`         | Modbus TCP port number                            |
| `-script` | *(empty)*     | Path to a `.lua` file; if omitted the built-in demo runs |

## Lua API reference: the `modbus` module

The `modbus` global table is available to every script run by this host. It
exposes one top-level function and a set of client methods.

### Connecting

```lua
local client = modbus.connect(addr, opts)
```

- `addr` -- string, the IP address or hostname of the Modbus TCP server.
- `opts` -- optional table with the following fields:

| Field         | Type    | Default | Description                              |
|---------------|---------|---------|------------------------------------------|
| `port`        | integer | `502`   | TCP port number                          |
| `unit`        | integer | `1`     | Modbus unit / slave ID                   |
| `retries`     | integer | `0`     | Number of times to retry on transport errors |
| `retry_delay` | number  | `0.5`   | Seconds to wait between retries          |
| `timeout`     | number  | `10`    | Connection timeout in seconds            |

Returns a client object (a Lua table with methods). On failure, raises a Lua
error.

### Reading data

All read methods use Lua's colon syntax (`client:method(...)`) and return
1-based Lua tables.

```lua
-- Read holding registers (function code 0x03).
-- Returns a table of integer values.
local regs = client:read_holding_registers(address, quantity)

-- Read input registers (function code 0x04).
local inputs = client:read_input_registers(address, quantity)

-- Read coils (function code 0x01).
-- Returns a table of boolean values.
local coils = client:read_coils(address, quantity)

-- Read discrete inputs (function code 0x02).
local discretes = client:read_discrete_inputs(address, quantity)
```

- `address` -- integer, the zero-based starting address.
- `quantity` -- integer, how many registers or coils to read.

### Writing data

```lua
-- Write a single holding register (function code 0x06).
client:write_register(address, value)

-- Write multiple holding registers (function code 0x10).
client:write_registers(address, {v1, v2, v3})

-- Write a single coil (function code 0x05).
client:write_coil(address, true)

-- Write multiple coils (function code 0x0F).
client:write_coils(address, {true, false, true})
```

### Atomic read-write

```lua
-- Read holding registers and write holding registers in a single transaction
-- (function code 0x17).
local results = client:read_write_registers(read_addr, read_qty, write_addr, {v1, v2})
```

### Device identification

```lua
-- Read device identification objects (function code 0x2B/0x0E).
-- Returns a table with vendor_name, product_code, and revision fields.
local dev = client:read_device_id()
print(dev.vendor_name, dev.product_code, dev.revision)
```

### Register conversion utilities

Many industrial devices store 32-bit values across two consecutive 16-bit Modbus
registers. The client provides helper methods to reassemble them:

```lua
-- Interpret two registers as a signed 32-bit integer (big-endian).
local int_val = client:to_int32(high_register, low_register)

-- Interpret two registers as an IEEE 754 float (big-endian).
local float_val = client:to_float32(high_register, low_register)
```

These are useful when a device stores a temperature as a FLOAT32 across
registers 100 and 101, for example:

```lua
local regs = client:read_holding_registers(100, 2)
local temp = client:to_float32(regs[1], regs[2])
print("Temperature: " .. temp)
```

### Closing the connection

```lua
client:close()
```

Always close the client when you are done to release the TCP connection.

## Error handling with pcall

All `modbus` methods raise Lua errors when something goes wrong -- for example,
if the server returns a Modbus exception code (illegal address, illegal
function) or a network timeout occurs. In Lua, you handle these with `pcall`
(protected call):

```lua
local ok, err = pcall(function()
    client:read_holding_registers(60000, 125)
end)

if not ok then
    print("Error: " .. tostring(err))
else
    print("Read succeeded")
end
```

`pcall` calls the function in protected mode. If the function raises an error,
`pcall` returns `false` and the error message as a string. If the function
succeeds, `pcall` returns `true` followed by the function's return values.

This is the Lua equivalent of Go's `if err != nil` pattern. Use it any time you
want your script to handle errors gracefully rather than terminating.

## Expected output

The built-in demo script produces output similar to:

```
Connected to Modbus server at 10.2.150.23:502
------------------------------------------------------------

[1] Reading 10 holding registers from address 0:
  Register 0: 1234 (0x04D2)
  Register 1: 0 (0x0000)
  Register 2: 5678 (0x162E)
  ...

[2] Reading 8 coils from address 0:
  Coil 0: ON
  Coil 1: OFF
  Coil 2: ON
  ...

[3] Reading 5 input registers from address 0:
  Input Register 0: 42
  Input Register 1: 0
  ...

[4] Register conversion utilities:
  Registers [1234, 0] as INT32:   80871424
  Registers [1234, 0] as FLOAT32: 0.000000

[5] Error handling:
  Expected error caught: read_holding_registers: ...

Done.
```

The actual values depend entirely on the device you connect to.

## Writing custom scripts

Create a `.lua` file and pass it with `-script`. Your script has access to the
`MODBUS_ADDR` and `MODBUS_PORT` globals injected by the Go host, plus the full
`modbus` module and Lua standard library.

Example custom script (`my_registers.lua`):

```lua
local client = modbus.connect(MODBUS_ADDR, {
    port    = MODBUS_PORT,
    unit    = 1,
    timeout = 5,
})

-- Read temperature from registers 100-101 (FLOAT32).
local regs = client:read_holding_registers(100, 2)
local temp = client:to_float32(regs[1], regs[2])

if temp > 85.0 then
    print("WARNING: temperature is " .. temp .. " degrees")
else
    print("Temperature normal: " .. temp)
end

client:close()
```

Run it:

```
go run ./examples/lua/modbus_client/ -addr 10.2.150.23 -port 502 -script my_registers.lua
```
