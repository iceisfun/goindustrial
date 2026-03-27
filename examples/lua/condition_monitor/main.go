// Example: lua/condition_monitor
//
// Demonstrates using Lua to evaluate compound boolean conditions with
// per-signal time qualifications against live PLC data. The Go host provides
// PLC connectivity (via industrialLua.Open), a monotonic clock, and a sleep
// primitive. The Lua script defines which signals to read, how to compose
// them into conditions, and what hold times to require before a condition
// fires — preventing false triggers from signal bounce or transient states.
//
// This pattern separates deployment-specific logic (tag names, thresholds,
// hold times) from the compiled Go binary, making it easy to adjust
// condition rules without recompiling.
//
// The built-in demo monitors four boolean tags and evaluates three conditions:
//
//   safe_to_run:  !estop AND is_running AND estop_clear FOR 500ms AND is_running FOR 1s
//   drive_fault:  is_running AND !drive_ready FOR 200ms
//   estop_active: estop (immediate, no hold time)
//
// Usage:
//
//	go run . -addr 192.168.1.20
//	go run . -addr 192.168.1.10:44818 -poll 50 -cycles 500
//	go run . -addr 192.168.1.20 -script my_conditions.lua
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/iceisfun/golua/compiler"
	"github.com/iceisfun/golua/parser"
	"github.com/iceisfun/golua/stdlib"
	"github.com/iceisfun/golua/vm"

	industrialLua "github.com/iceisfun/goindustrial/lua"
)

func main() {
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address (host or host:port)")
	script := flag.String("script", "", "Path to a Lua script (if empty, runs the built-in demo)")
	poll := flag.Int("poll", 100, "Poll interval in milliseconds")
	cycles := flag.Int("cycles", 0, "Max scan cycles (0 = run until interrupted)")
	flag.Parse()

	var source string
	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			log.Fatalf("Failed to read script: %v", err)
		}
		source = string(data)
	} else {
		source = builtinScript
	}

	block, err := parser.Parse("condition_monitor", source)
	if err != nil {
		log.Fatalf("Lua parse error: %v", err)
	}

	proto, err := compiler.Compile("condition_monitor", block)
	if err != nil {
		log.Fatalf("Lua compile error: %v", err)
	}

	v := vm.New()
	stdlib.Open(v)
	industrialLua.Open(v)

	// Inject configuration globals for the Lua script.
	v.SetGlobal("PLC_ADDR", vm.NewString(*addr))
	v.SetGlobal("POLL_MS", vm.NewInt(int64(*poll)))
	v.SetGlobal("MAX_CYCLES", vm.NewInt(int64(*cycles)))

	// clock_ms() — monotonic milliseconds since program start.
	// Used by the Lua condition engine for time-qualified expressions.
	start := time.Now()
	v.SetGlobal("clock_ms", vm.NewNativeFunc(func(v *vm.VM) int {
		v.Set(0, vm.NewInt(time.Since(start).Milliseconds()))
		return 1
	}))

	// sleep_ms(n) — pause execution for n milliseconds.
	// Drives the Lua-side scan loop timing.
	v.SetGlobal("sleep_ms", vm.NewNativeFunc(func(v *vm.VM) int {
		ms := v.Get(1)
		if !ms.IsNumber() {
			panic(&vm.LuaError{Value: vm.NewString("bad argument #1 to 'sleep_ms' (number expected)")})
		}
		time.Sleep(time.Duration(ms.AsInt()) * time.Millisecond)
		return 0
	}))

	_, err = v.Run(proto)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lua runtime error: %v\n", err)
		os.Exit(1)
	}
}

// builtinScript is the default Lua condition monitor. It reads four boolean
// tags from an EtherNet/IP PLC and evaluates compound conditions with hold
// times. Change the tag names in the "tags" table to match your PLC program.
//
// For Modbus, replace the eip.connect / read_tag calls with:
//
//	local client = modbus.connect(MODBUS_ADDR, { port = 502, unit = 1 })
//	local coils = client:read_coils(0, 4)
//	sig:set("estop",       coils[1], now)
//	sig:set("is_running",  coils[2], now)
//	sig:set("estop_clear", coils[3], now)
//	sig:set("drive_ready", coils[4], now)
const builtinScript = `
-- Condition Monitor
--
-- Evaluates compound boolean expressions with per-signal hold times.
-- A condition fires only when its expression has been continuously
-- true for the required duration, preventing false triggers from
-- signal bounce or transient states.

----------------------------------------------------------------------
-- Signal tracker
----------------------------------------------------------------------
-- Tracks the current value of each signal and the timestamp of its
-- last state change. This is what makes time-qualified conditions
-- like "estop_clear FOR 500ms" possible.

local sig = { val = {}, since = {} }

function sig:set(name, new_val, now)
    if self.val[name] ~= new_val then
        self.since[name] = now
    end
    if not self.since[name] then
        self.since[name] = now
    end
    self.val[name] = new_val
end

function sig:get(name)
    return self.val[name]
end

-- Returns true only if the signal has been equal to 'expected'
-- for at least 'ms' milliseconds continuously.
function sig:held(name, expected, ms, now)
    if self.val[name] ~= expected then return false end
    return (now - (self.since[name] or now)) >= ms
end

----------------------------------------------------------------------
-- Condition engine
----------------------------------------------------------------------
-- Conditions are boolean expressions over signals. Each condition
-- tracks its own active/cleared state and fires callbacks on
-- transitions — not on every scan.

local conditions = {}
local cond_state = {}

local function define(name, opts)
    conditions[#conditions + 1] = {
        name      = name,
        eval      = opts.eval,
        on_active = opts.on_active or function() end,
        on_clear  = opts.on_clear or function() end,
    }
    cond_state[name] = { active = false }
end

local function evaluate(now)
    for _, c in ipairs(conditions) do
        local ok, result = pcall(c.eval, now)
        if not ok then result = false end

        local s = cond_state[c.name]
        if result and not s.active then
            s.active = true
            c.on_active()
        elseif not result and s.active then
            s.active = false
            c.on_clear()
        end
    end
end

----------------------------------------------------------------------
-- PLC connection
----------------------------------------------------------------------

local client = eip.connect(PLC_ADDR, { retries = 3, timeout = 10 })
print("Connected to " .. PLC_ADDR)

----------------------------------------------------------------------
-- Tag mapping
----------------------------------------------------------------------
-- Change these tag names to match your PLC program. Each entry maps
-- a friendly signal name (used in conditions) to a PLC tag.

local tags = {
    { name = "estop",       tag = "EStop" },
    { name = "is_running",  tag = "MachineRunning" },
    { name = "estop_clear", tag = "EStopReset" },
    { name = "drive_ready", tag = "DriveReady" },
}

----------------------------------------------------------------------
-- Condition definitions
----------------------------------------------------------------------

-- 1. Safe to run
--    Expression: !estop AND is_running AND estop_clear FOR 500ms AND is_running FOR 1s
--
--    The hold times ensure the machine has genuinely cleared the e-stop
--    sequence and the run signal is stable, not just a momentary flicker.

define("safe_to_run", {
    eval = function(now)
        return sig:get("estop") == false
           and sig:get("is_running") == true
           and sig:held("estop_clear", true, 500, now)
           and sig:held("is_running", true, 1000, now)
    end,
    on_active = function()
        print(string.format("[%8.1fs] ACTIVE  safe_to_run — machine cleared to operate", clock_ms() / 1000))
    end,
    on_clear = function()
        print(string.format("[%8.1fs] CLEARED safe_to_run", clock_ms() / 1000))
    end,
})

-- 2. Drive fault
--    Expression: is_running AND !drive_ready FOR 200ms
--
--    The 200ms hold debounces transient glitches on the drive ready
--    signal that are normal during acceleration/deceleration.

define("drive_fault", {
    eval = function(now)
        return sig:get("is_running") == true
           and sig:held("drive_ready", false, 200, now)
    end,
    on_active = function()
        print(string.format("[%8.1fs] ALARM   drive_fault — drive not ready while running", clock_ms() / 1000))
    end,
    on_clear = function()
        print(string.format("[%8.1fs] CLEARED drive_fault", clock_ms() / 1000))
    end,
})

-- 3. E-stop engaged (immediate — no hold time)

define("estop_active", {
    eval = function(now)
        return sig:get("estop") == true
    end,
    on_active = function()
        print(string.format("[%8.1fs] ESTOP   emergency stop engaged", clock_ms() / 1000))
    end,
    on_clear = function()
        print(string.format("[%8.1fs] CLEARED e-stop released", clock_ms() / 1000))
    end,
})

----------------------------------------------------------------------
-- Scan loop
----------------------------------------------------------------------

print(string.format("Monitoring %d signals, %d conditions — poll every %dms",
    #tags, #conditions, POLL_MS))
print(string.rep("-", 64))

local cycle = 0
while true do
    cycle = cycle + 1
    if MAX_CYCLES > 0 and cycle > MAX_CYCLES then break end

    local now = clock_ms()

    -- Read all signals from PLC.
    for _, t in ipairs(tags) do
        local ok, val = pcall(function() return client:read_tag(t.tag) end)
        if ok then
            sig:set(t.name, val, now)
        end
    end

    -- Evaluate all conditions against current signal state.
    evaluate(now)

    -- Periodic status dump every 50 cycles.
    if cycle % 50 == 0 then
        local parts = {}
        for _, t in ipairs(tags) do
            parts[#parts + 1] = string.format("%s=%s", t.name, tostring(sig:get(t.name)))
        end

        local active = {}
        for _, c in ipairs(conditions) do
            if cond_state[c.name].active then
                active[#active + 1] = c.name
            end
        end

        local active_str = #active > 0 and table.concat(active, ", ") or "none"
        print(string.format("[%8.1fs] scan #%d  %s  active=[%s]",
            now / 1000, cycle, table.concat(parts, "  "), active_str))
    end

    sleep_ms(POLL_MS)
end

client:close()
print("Done.")
`
