// Example: lua/ethernetip_client
//
// Demonstrates using goindustrial's Lua bindings to read tags from a Rockwell
// Logix PLC over EtherNet/IP. The Go host creates a GoLua VM, opens the
// goindustrial module, and runs a Lua script that connects to the PLC and
// reads tags of various CIP data types.
//
// This is useful for:
//   - Scripting tag data collection from PLCs without recompiling
//   - User-defined alerting, calculations, or data export logic in Lua
//   - Dynamic tag reading based on configuration or discovery
//
// Usage:
//
//	go run . -addr 10.0.10.70
//	go run . -addr 192.168.1.10:44818 -script my_script.lua
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/iceisfun/golua/compiler"
	"github.com/iceisfun/golua/parser"
	"github.com/iceisfun/golua/stdlib"
	"github.com/iceisfun/golua/vm"

	industrialLua "github.com/iceisfun/goindustrial/lua"
)

func main() {
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address (host or host:port)")
	script := flag.String("script", "", "Path to a Lua script file (if empty, runs the built-in demo)")
	flag.Parse()

	var source string
	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			log.Fatalf("Failed to read script: %v", err)
		}
		source = string(data)
	} else {
		source = builtinEIPScript
	}

	block, err := parser.Parse("eip_client", source)
	if err != nil {
		log.Fatalf("Lua parse error: %v", err)
	}

	proto, err := compiler.Compile("eip_client", block)
	if err != nil {
		log.Fatalf("Lua compile error: %v", err)
	}

	v := vm.New()
	stdlib.Open(v)
	industrialLua.Open(v)

	v.SetGlobal("EIP_ADDR", vm.NewString(*addr))

	_, err = v.Run(proto)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lua runtime error: %v\n", err)
		os.Exit(1)
	}
}

const builtinEIPScript = `
-- Connect to a Rockwell Logix PLC over EtherNet/IP.
-- The eip.connect() function handles TCP connection + EIP session registration.
local client = eip.connect(EIP_ADDR, {
    retries = 2,
    timeout = 10,
})

print("Connected to EtherNet/IP device at " .. EIP_ADDR)
print(string.rep("-", 60))

-- Discover all tags on the PLC.
-- list_tags() queries the Symbol Object (CIP Class 0x6B) to enumerate tags.
print("\n[1] Discovering tags on PLC...")
local tags = client:list_tags()
print(string.format("  Found %d tags", #tags))

-- Print the first 20 tags as a sample.
local show = math.min(#tags, 20)
print(string.format("\n  First %d tags:", show))
for i = 1, show do
    print(string.format("    %4d  %-40s  type=0x%04X", tags[i].id, tags[i].name, tags[i].type))
end
if #tags > show then
    print(string.format("    ... and %d more", #tags - show))
end

-- Find and read some DINT tags (type code 0x00C4).
print("\n[2] Reading DINT tags:")
local dint_count = 0
for i = 1, #tags do
    -- 0x00C4 is DINT, but array types have the high bit set (0x8000).
    -- Only read scalar DINTs (non-array).
    if tags[i].type == 0x00C4 and dint_count < 5 then
        local ok, val = pcall(function()
            return client:read_tag(tags[i].name)
        end)
        if ok then
            print(string.format("  %-40s = %s", tags[i].name, tostring(val)))
            dint_count = dint_count + 1
        end
    end
end
if dint_count == 0 then
    print("  (no readable DINT tags found)")
end

-- Demonstrate batch reading.
-- read_tags() reads multiple tags in sequence and returns a table of values.
print("\n[3] Batch tag read:")
local batch_names = {}
local batch_count = 0
for i = 1, #tags do
    if tags[i].type == 0x00C4 and batch_count < 3 then
        batch_names[#batch_names + 1] = tags[i].name
        batch_count = batch_count + 1
    end
end
if #batch_names > 0 then
    local ok, values = pcall(function()
        return client:read_tags(batch_names)
    end)
    if ok then
        for i = 1, #batch_names do
            print(string.format("  %-40s = %s", batch_names[i], tostring(values[i])))
        end
    else
        print("  Batch read error: " .. tostring(values))
    end
else
    print("  (no DINT tags available for batch read)")
end

-- Error handling: try to read a tag that doesn't exist.
print("\n[4] Error handling:")
local ok, err = pcall(function()
    client:read_tag("THIS_TAG_DOES_NOT_EXIST_12345")
end)
if not ok then
    print("  Expected error: " .. tostring(err))
else
    print("  Surprisingly, the tag exists!")
end

-- CIP type constants are available for explicit writes.
print("\n[5] Available CIP type constants:")
for name, val in pairs(eip.types) do
    print(string.format("  eip.types.%-8s = %q", name, val))
end

client:close()
print("\nDone.")
`
