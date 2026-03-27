// Example: lua/monitor_tags
//
// Demonstrates using Lua scripts to define which tags to monitor and how to
// process the data. The Go host provides the industrial protocol bindings,
// while the Lua script controls the logic — which tags to read, how often,
// and what to do with the values.
//
// This pattern is powerful for deployments where:
//   - Different sites need different tag configurations
//   - Alerting/transformation rules change frequently
//   - Non-Go-developers need to modify data collection behavior
//
// Usage:
//
//	go run . -addr 10.0.10.70
//	go run . -addr 192.168.1.10:44818 -script custom_monitor.lua
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
	cycles := flag.Int("cycles", 5, "Number of poll cycles to run (built-in demo only)")
	flag.Parse()

	var source string
	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			log.Fatalf("Failed to read script: %v", err)
		}
		source = string(data)
	} else {
		source = builtinMonitorScript
	}

	block, err := parser.Parse("monitor_tags", source)
	if err != nil {
		log.Fatalf("Lua parse error: %v", err)
	}

	proto, err := compiler.Compile("monitor_tags", block)
	if err != nil {
		log.Fatalf("Lua compile error: %v", err)
	}

	v := vm.New()
	stdlib.Open(v)
	industrialLua.Open(v)

	v.SetGlobal("EIP_ADDR", vm.NewString(*addr))
	v.SetGlobal("POLL_CYCLES", vm.NewInt(int64(*cycles)))

	_, err = v.Run(proto)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lua runtime error: %v\n", err)
		os.Exit(1)
	}
}

const builtinMonitorScript = `
-- Monitor Tags Example
--
-- This script discovers DINT tags on a PLC and polls a few of them
-- in a loop, printing value changes.

local client = eip.connect(EIP_ADDR, {
    retries = 3,
    timeout = 10,
})

print("Connected to " .. EIP_ADDR)

-- Discover tags and pick up to 3 scalar DINTs to monitor.
local tags = client:list_tags()
local monitor_tags = {}
for i = 1, #tags do
    if tags[i].type == 0x00C4 and #monitor_tags < 3 then
        monitor_tags[#monitor_tags + 1] = tags[i].name
    end
end

if #monitor_tags == 0 then
    print("No DINT tags found to monitor.")
    client:close()
    return
end

print(string.format("Monitoring %d tags for %d cycles:", #monitor_tags, POLL_CYCLES))
for i, name in ipairs(monitor_tags) do
    print(string.format("  [%d] %s", i, name))
end
print(string.rep("-", 60))

-- Track previous values for change detection.
local prev_values = {}

for cycle = 1, POLL_CYCLES do
    -- Read all monitored tags.
    local ok, values = pcall(function()
        return client:read_tags(monitor_tags)
    end)

    if ok then
        local line = string.format("Cycle %d:", cycle)
        for i, name in ipairs(monitor_tags) do
            local val = values[i]
            local changed = (prev_values[name] ~= nil and prev_values[name] ~= val)
            local marker = changed and " *" or ""
            line = line .. string.format("  %s=%s%s", name, tostring(val), marker)
            prev_values[name] = val
        end
        print(line)
    else
        print(string.format("Cycle %d: ERROR - %s", cycle, tostring(values)))
    end
end

client:close()
print("\nDone.")
`
