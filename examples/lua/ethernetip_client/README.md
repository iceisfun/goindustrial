# EtherNet/IP Client - Lua Scripting Example

This example demonstrates how to use Lua scripts to read and write tags on a
Rockwell / Allen-Bradley Logix PLC over EtherNet/IP (CIP). A small Go host
program creates a Lua virtual machine, loads the `eip` module provided by the
`github.com/iceisfun/goindustrial/lua` package, and then executes a Lua script
that connects to the PLC, discovers tags, and reads their values -- all without
recompiling Go code.

## Why Lua scripting for EtherNet/IP?

EtherNet/IP with CIP (Common Industrial Protocol) is the standard protocol for
Rockwell Logix controllers (ControlLogix, CompactLogix). These PLCs organize
their data as named tags rather than numbered registers, and each tag has a CIP
data type (BOOL, SINT, INT, DINT, REAL, STRING, etc.). In practice you often
need to:

- **Discover tags dynamically** -- rather than hard-coding tag names, scripts
  can call `list_tags()` and decide at runtime which tags to read.
- **Adjust collection logic per site** -- different PLCs have different tag
  databases. A Lua script is a portable, editable configuration that includes
  logic.
- **Prototype integrations** -- test which tags are available and what types
  they return before committing to a compiled solution.
- **Let non-Go-developers modify behavior** -- operators or integrators can
  edit a `.lua` file without setting up a Go toolchain.

The `eip` Lua module gives scripts access to tag reads, writes, discovery, and
structured data types (Timers, Counters), while the Go host manages EIP session
registration, CIP routing, and binary encoding.

## How it works

The Go host (`main.go`) performs five steps:

1. **Parse flags** -- reads `-addr` and `-script` from the command line.
2. **Load the Lua source** -- either from a file (when `-script` is given) or
   from a built-in demo script embedded in the Go binary.
3. **Create a GoLua VM** -- `vm.New()` creates the virtual machine,
   `stdlib.Open(v)` loads standard Lua libraries, and
   `industrialLua.Open(v)` registers the `modbus` and `eip` global modules.
4. **Inject globals** -- the Go host sets `EIP_ADDR` (string) as a Lua global
   so the script knows where to connect.
5. **Run the script** -- the compiled bytecode executes inside the VM.

## Running the example

Connect to a Logix PLC:

```
go run ./examples/lua/ethernetip_client/ -addr 10.0.10.70
```

Specify a non-default port (the EtherNet/IP default is 44818):

```
go run ./examples/lua/ethernetip_client/ -addr 192.168.1.10:44818
```

Run a custom Lua script:

```
go run ./examples/lua/ethernetip_client/ -addr 10.0.10.70 -script my_tags.lua
```

### Flags

| Flag      | Default                 | Description                                        |
|-----------|-------------------------|----------------------------------------------------|
| `-addr`   | `192.168.1.10:44818`    | PLC address as `host` or `host:port`               |
| `-script` | *(empty)*               | Path to a `.lua` file; if omitted the built-in demo runs |

## Lua API reference: the `eip` module

The `eip` global table is available to every script run by this host. It
provides a connection function, client methods, and CIP type constants.

### Connecting

```lua
local client = eip.connect(addr, opts)
```

- `addr` -- string, the PLC address in `"host"` or `"host:port"` format. If no
  port is specified, 44818 is used.
- `opts` -- optional table with the following fields:

| Field         | Type    | Default | Description                                          |
|---------------|---------|---------|------------------------------------------------------|
| `retries`     | integer | `0`     | Number of times to retry on transport errors (-1 = infinite) |
| `retry_delay` | number  | `1`     | Seconds to wait between retries                      |
| `timeout`     | number  | `10`    | Connection timeout in seconds                        |

Returns a client object. On failure, raises a Lua error.

The connection process handles the full EtherNet/IP handshake: TCP connect, EIP
RegisterSession, and CIP Forward Open. All of this is transparent to the Lua
script.

### Reading tags

```lua
-- Read a single tag. The return value is automatically typed based on the CIP
-- data type reported by the PLC:
--   BOOL       -> Lua boolean
--   SINT/INT/DINT/LINT/USINT/UINT/UDINT/ULINT -> Lua integer
--   REAL/LREAL -> Lua float
--   STRING     -> Lua string
local value = client:read_tag("MyDintTag")

-- Read raw bytes (useful for structures or arrays). Returns a Lua string
-- containing the raw CIP response bytes. count defaults to 1.
local raw = client:read_tag_raw("ArrayTag", count)
```

### Batch reading

```lua
-- Read multiple tags in sequence. Pass a table of tag name strings.
-- Returns a table of values in the same order.
local values = client:read_tags({"Tag1", "Tag2", "Tag3"})
print(values[1], values[2], values[3])
```

This is more convenient than calling `read_tag` in a loop, although under the
hood each tag is still read individually. Errors during the batch cause the
entire call to raise a Lua error.

### Writing tags

```lua
-- Write a tag. The value type is inferred from the Lua type:
--   Lua boolean  -> CIP BOOL
--   Lua integer  -> CIP DINT (32-bit signed)
--   Lua float    -> CIP REAL (32-bit float)
--   Lua string   -> CIP STRING
client:write_tag("MyDintTag", 42)

-- Force a specific CIP type by passing a third argument.
-- This is necessary when the default mapping is not what you need.
client:write_tag("MySintTag", 7, eip.types.SINT)
client:write_tag("MyRealTag", 3.14, eip.types.REAL)
client:write_tag("MyLintTag", 123456789, eip.types.LINT)
```

### CIP type constants

The `eip.types` table provides string constants for every supported CIP data
type. These are used as the optional third argument to `write_tag`:

| Constant          | Value     | Description                           |
|-------------------|-----------|---------------------------------------|
| `eip.types.BOOL`  | `"BOOL"`  | Boolean                              |
| `eip.types.SINT`  | `"SINT"`  | Signed 8-bit integer                 |
| `eip.types.INT`   | `"INT"`   | Signed 16-bit integer                |
| `eip.types.DINT`  | `"DINT"`  | Signed 32-bit integer                |
| `eip.types.LINT`  | `"LINT"`  | Signed 64-bit integer                |
| `eip.types.USINT` | `"USINT"` | Unsigned 8-bit integer               |
| `eip.types.UINT`  | `"UINT"`  | Unsigned 16-bit integer              |
| `eip.types.UDINT` | `"UDINT"` | Unsigned 32-bit integer              |
| `eip.types.ULINT` | `"ULINT"` | Unsigned 64-bit integer              |
| `eip.types.REAL`  | `"REAL"`  | 32-bit IEEE 754 float                |
| `eip.types.LREAL` | `"LREAL"` | 64-bit IEEE 754 float                |
| `eip.types.STRING`| `"STRING"`| CIP STRING (UINT length + bytes)     |

### Tag discovery

```lua
-- Enumerate all tags defined in the PLC program.
-- Returns a 1-based table of tag entries, each with:
--   .id   -- integer, the CIP Symbol Instance ID
--   .name -- string, the tag name
--   .type -- integer, the CIP type code
local tags = client:list_tags()

for i = 1, #tags do
    print(tags[i].id, tags[i].name, string.format("0x%04X", tags[i].type))
end
```

Some useful type code values to know:

- `0x00C1` -- BOOL
- `0x00C2` -- SINT
- `0x00C3` -- INT
- `0x00C4` -- DINT
- `0x00CA` -- REAL
- Tags with the high bit set (`0x8000`) are arrays.

### Reading structured types

```lua
-- Read a Timer tag. Returns a table with fields:
--   .pre (integer) -- preset value
--   .acc (integer) -- accumulated value
--   .en  (boolean) -- enable bit
--   .tt  (boolean) -- timer timing bit
--   .dn  (boolean) -- done bit
local timer = client:read_timer("MyTimer")
print("Preset:", timer.pre, "Accumulated:", timer.acc, "Done:", timer.dn)

-- Read a Counter tag. Returns a table with fields:
--   .pre (integer) -- preset value
--   .acc (integer) -- accumulated value
--   .cu  (boolean) -- count up enable
--   .cd  (boolean) -- count down enable
--   .dn  (boolean) -- done bit
--   .ov  (boolean) -- overflow
--   .un  (boolean) -- underflow
local ctr = client:read_counter("MyCounter")
print("Count:", ctr.acc, "Done:", ctr.dn)
```

### Closing the connection

```lua
client:close()
```

Always close the client when you are done. This sends a CIP Forward Close and
EIP UnregisterSession to cleanly tear down the session.

## Error handling with pcall

All `eip` methods raise Lua errors on failure -- for example, if you read a tag
that does not exist, or the PLC returns a CIP error status. In Lua, you handle
errors with `pcall` (protected call):

```lua
local ok, err = pcall(function()
    client:read_tag("THIS_TAG_DOES_NOT_EXIST")
end)

if not ok then
    print("Error: " .. tostring(err))
end
```

`pcall` calls the function in protected mode. If the function raises an error,
`pcall` returns `false` and the error message string. If the function succeeds,
`pcall` returns `true` followed by any return values. This lets your script
continue running even when individual operations fail.

You can also capture return values on success:

```lua
local ok, value = pcall(function()
    return client:read_tag("MaybeExists")
end)

if ok then
    print("Value: " .. tostring(value))
else
    print("Could not read tag: " .. tostring(value))
end
```

## Expected output

The built-in demo script produces output similar to:

```
Connected to EtherNet/IP device at 10.0.10.70
------------------------------------------------------------

[1] Discovering tags on PLC...
  Found 147 tags

  First 20 tags:
       1  Program:MainProgram.RunCount              type=0x00C4
       2  Program:MainProgram.Temperature            type=0x00CA
       3  Program:MainProgram.MotorSpeed             type=0x00C4
    ...
    ... and 127 more

[2] Reading DINT tags:
  Program:MainProgram.RunCount              = 4821
  Program:MainProgram.MotorSpeed            = 1750
  ...

[3] Batch tag read:
  Program:MainProgram.RunCount              = 4821
  Program:MainProgram.MotorSpeed            = 1750
  Program:MainProgram.BatchCount            = 12

[4] Error handling:
  Expected error: read_tag("THIS_TAG_DOES_NOT_EXIST_12345"): ...

[5] Available CIP type constants:
  eip.types.BOOL     = "BOOL"
  eip.types.SINT     = "SINT"
  eip.types.INT      = "INT"
  eip.types.DINT     = "DINT"
  ...

Done.
```

The actual tag names and values depend entirely on the PLC program.

## Writing custom scripts

Create a `.lua` file and pass it with `-script`. Your script has access to the
`EIP_ADDR` global injected by the Go host, plus the full `eip` module and Lua
standard library.

Example custom script (`my_tags.lua`):

```lua
local client = eip.connect(EIP_ADDR, {
    retries = 2,
    timeout = 10,
})

-- Discover all tags and read every REAL (float) tag.
local tags = client:list_tags()
for i = 1, #tags do
    if tags[i].type == 0x00CA then  -- 0x00CA = REAL
        local ok, val = pcall(function()
            return client:read_tag(tags[i].name)
        end)
        if ok then
            print(string.format("%-40s = %.2f", tags[i].name, val))
        end
    end
end

-- Write a value to a known tag.
client:write_tag("Program:MainProgram.Setpoint", 75.0, eip.types.REAL)

client:close()
```

Run it:

```
go run ./examples/lua/ethernetip_client/ -addr 10.0.10.70 -script my_tags.lua
```
