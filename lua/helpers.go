package lua

import (
	"fmt"

	"github.com/iceisfun/golua/v2/vm"
)

// luaErrorf raises a Lua-visible error from a native function.
func luaErrorf(format string, args ...any) {
	panic(&vm.LuaError{Value: vm.NewString(fmt.Sprintf(format, args...))})
}

// getString returns the string at stack index idx or panics with a Lua error.
func getString(v *vm.VM, idx int, fname string) string {
	val := v.Get(idx)
	if val.IsString() {
		return val.AsString()
	}
	luaErrorf("bad argument #%d to '%s' (string expected, got %s)", idx, fname, val.Type())
	return ""
}

// getInt returns the integer at stack index idx or panics with a Lua error.
func getInt(v *vm.VM, idx int, fname string) int64 {
	val := v.Get(idx)
	if val.IsNumber() {
		return val.AsInt()
	}
	luaErrorf("bad argument #%d to '%s' (number expected, got %s)", idx, fname, val.Type())
	return 0
}

// getOptInt returns the integer at stack index idx, or defaultVal if nil.
func getOptInt(v *vm.VM, idx int, defaultVal int64) int64 {
	val := v.Get(idx)
	if val.IsNil() {
		return defaultVal
	}
	if val.IsNumber() {
		return val.AsInt()
	}
	return defaultVal
}

// getOptString returns the string at stack index idx, or defaultVal if nil.
func getOptString(v *vm.VM, idx int, defaultVal string) string {
	val := v.Get(idx)
	if val.IsNil() {
		return defaultVal
	}
	if val.IsString() {
		return val.AsString()
	}
	return defaultVal
}

// getTable returns the table at stack index idx or panics with a Lua error.
func getTable(v *vm.VM, idx int, fname string) vm.LuaTable {
	val := v.Get(idx)
	if val.IsTable() {
		return val.AsTable()
	}
	luaErrorf("bad argument #%d to '%s' (table expected, got %s)", idx, fname, val.Type())
	return nil
}

// tableGetString reads a string field from a table, returning "" if missing.
func tableGetString(tbl vm.LuaTable, key string) string {
	val := tbl.Get(vm.NewString(key))
	if val.IsString() {
		return val.AsString()
	}
	return ""
}

// tableGetInt reads an integer field from a table, returning defaultVal if missing.
func tableGetInt(tbl vm.LuaTable, key string, defaultVal int64) int64 {
	val := tbl.Get(vm.NewString(key))
	if val.IsNumber() {
		return val.AsInt()
	}
	return defaultVal
}

// tableGetFloat reads a float field from a table, returning defaultVal if missing.
func tableGetFloat(tbl vm.LuaTable, key string, defaultVal float64) float64 {
	val := tbl.Get(vm.NewString(key))
	if val.IsNumber() {
		return val.AsFloat()
	}
	return defaultVal
}

// tableGetBool reads a boolean field from a table, returning defaultVal if missing.
func tableGetBool(tbl vm.LuaTable, key string, defaultVal bool) bool {
	val := tbl.Get(vm.NewString(key))
	if val.IsBool() {
		return val.AsBool()
	}
	return defaultVal
}

// registersToLuaTable converts a slice of uint16 register values to a Lua table
// with 1-based integer keys.
func registersToLuaTable(regs []uint16) *vm.Table {
	t := vm.NewEmptyTable()
	for i, r := range regs {
		t.Set(vm.NewInt(int64(i+1)), vm.NewInt(int64(r)))
	}
	return t
}

// boolsToLuaTable converts a slice of bools to a Lua table with 1-based keys.
func boolsToLuaTable(vals []bool) *vm.Table {
	t := vm.NewEmptyTable()
	for i, b := range vals {
		t.Set(vm.NewInt(int64(i+1)), vm.NewBool(b))
	}
	return t
}
