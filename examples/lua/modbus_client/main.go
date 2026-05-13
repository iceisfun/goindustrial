// Example: lua/modbus_client
//
// Demonstrates using goindustrial's Lua bindings to read and write Modbus
// registers from a Lua script. The Go host creates a GoLua VM, opens the
// goindustrial module, and runs an embedded Lua script that connects to a
// Modbus TCP server and performs register operations.
//
// This is useful for:
//   - Scripting PLC data collection without recompiling Go code
//   - User-defined data transformation and alerting logic
//   - Rapid prototyping of Modbus integrations
//
// Usage:
//
//	go run . -addr 192.168.1.10 -port 502
//	go run . -addr 127.0.0.1 -port 5020
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/iceisfun/golua/v2/compiler"
	"github.com/iceisfun/golua/v2/parser"
	"github.com/iceisfun/golua/v2/stdlib"
	"github.com/iceisfun/golua/v2/vm"

	industrialLua "github.com/iceisfun/goindustrial/lua"
)

func main() {
	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")
	port := flag.Int("port", 502, "Modbus TCP port")
	script := flag.String("script", "", "Path to a Lua script file (if empty, runs the built-in demo)")
	flag.Parse()

	// The Lua source to run. If a script file is provided, read it; otherwise
	// use the embedded demo script.
	var source string
	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			log.Fatalf("Failed to read script: %v", err)
		}
		source = string(data)
	} else {
		source = builtinModbusScript
	}

	// --- GoLua setup ---

	// 1. Parse the Lua source into an AST.
	block, err := parser.Parse("modbus_client", source)
	if err != nil {
		log.Fatalf("Lua parse error: %v", err)
	}

	// 2. Compile the AST to bytecode.
	proto, err := compiler.Compile("modbus_client", block)
	if err != nil {
		log.Fatalf("Lua compile error: %v", err)
	}

	// 3. Create a VM and open standard + industrial modules.
	v := vm.New()
	stdlib.Open(v)
	industrialLua.Open(v)

	// 4. Inject configuration as globals so the Lua script can use them.
	v.SetGlobal("MODBUS_ADDR", vm.NewString(*addr))
	v.SetGlobal("MODBUS_PORT", vm.NewInt(int64(*port)))

	// 5. Run the script.
	_, err = v.Run(proto)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lua runtime error: %v\n", err)
		os.Exit(1)
	}
}

// builtinModbusScript is the default Lua script that demonstrates Modbus
// operations. It uses the MODBUS_ADDR and MODBUS_PORT globals set by the Go
// host.
const builtinModbusScript = `
-- Connect to the Modbus TCP server.
-- The modbus.connect() function takes an address and an options table.
local client = modbus.connect(MODBUS_ADDR, {
    port  = MODBUS_PORT,
    unit  = 1,         -- Modbus unit/slave ID
    retries = 2,       -- Retry transport errors up to 2 times
    timeout = 5,       -- Connection timeout in seconds
})

print("Connected to Modbus server at " .. MODBUS_ADDR .. ":" .. tostring(MODBUS_PORT))
print(string.rep("-", 60))

-- Read 10 holding registers starting at address 0.
-- Returns a 1-based Lua table of integer values.
print("\n[1] Reading 10 holding registers from address 0:")
local regs = client:read_holding_registers(0, 10)
for i = 1, #regs do
    print(string.format("  Register %d: %d (0x%04X)", i - 1, regs[i], regs[i]))
end

-- Read 8 coils starting at address 0.
-- Returns a 1-based Lua table of boolean values.
print("\n[2] Reading 8 coils from address 0:")
local coils = client:read_coils(0, 8)
for i = 1, #coils do
    local state = coils[i] and "ON" or "OFF"
    print(string.format("  Coil %d: %s", i - 1, state))
end

-- Read 5 input registers from address 0.
print("\n[3] Reading 5 input registers from address 0:")
local inputs = client:read_input_registers(0, 5)
for i = 1, #inputs do
    print(string.format("  Input Register %d: %d", i - 1, inputs[i]))
end

-- Demonstrate register conversion utilities.
-- Many industrial devices store 32-bit values across two consecutive registers.
print("\n[4] Register conversion utilities:")
local test_regs = client:read_holding_registers(0, 2)
if #test_regs >= 2 then
    local int32_val = client:to_int32(test_regs[1], test_regs[2])
    local float32_val = client:to_float32(test_regs[1], test_regs[2])
    print(string.format("  Registers [%d, %d] as INT32:   %d", test_regs[1], test_regs[2], int32_val))
    print(string.format("  Registers [%d, %d] as FLOAT32: %f", test_regs[1], test_regs[2], float32_val))
end

-- Error handling with pcall (protected call).
-- Modbus protocol errors (bad address, unsupported function) raise Lua errors.
print("\n[5] Error handling:")
local ok, err = pcall(function()
    -- Try to read from an address that may not exist.
    client:read_holding_registers(60000, 125)
end)
if not ok then
    print("  Expected error caught: " .. tostring(err))
else
    print("  Read succeeded (server has data at address 60000)")
end

-- Clean up.
client:close()
print("\nDone.")
`
