package lua

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/iceisfun/golua/v2/vm"
	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// openEIP registers the "eip" global module in the VM.
//
// Lua API:
//
//	client = eip.connect(addr, opts?)     -- connect to an EtherNet/IP device
//	client:read_tag(name) -> value        -- read a tag, auto-typed
//	client:read_tag_raw(name) -> string   -- read raw bytes
//	client:write_tag(name, value, type?)  -- write a tag
//	client:read_timer(name) -> table      -- read Timer structure
//	client:read_counter(name) -> table    -- read Counter structure
//	client:list_tags() -> table           -- enumerate all tags
//	client:close()
func openEIP(v *vm.VM) {
	mod := vm.NewEmptyTable()

	mod.SetString("connect", vm.NewNativeFunc(eipConnect))

	// Expose CIP type constants so Lua scripts can use them with write_tag.
	types := vm.NewEmptyTable()
	types.SetString("BOOL", vm.NewString("BOOL"))
	types.SetString("SINT", vm.NewString("SINT"))
	types.SetString("INT", vm.NewString("INT"))
	types.SetString("DINT", vm.NewString("DINT"))
	types.SetString("LINT", vm.NewString("LINT"))
	types.SetString("USINT", vm.NewString("USINT"))
	types.SetString("UINT", vm.NewString("UINT"))
	types.SetString("UDINT", vm.NewString("UDINT"))
	types.SetString("ULINT", vm.NewString("ULINT"))
	types.SetString("REAL", vm.NewString("REAL"))
	types.SetString("LREAL", vm.NewString("LREAL"))
	types.SetString("STRING", vm.NewString("STRING"))
	mod.SetString("types", vm.NewTable(types))

	v.SetGlobal("eip", vm.NewTable(mod))
}

// eipConnect implements eip.connect(addr, opts?)
//
// addr is the device address in "host" or "host:port" format (default port 44818).
// opts is an optional table with fields:
//   - retries (int, default 0, -1 for infinite)
//   - retry_delay (number, seconds, default 1)
//   - timeout (number, seconds, default 10)
//
// Returns a client table with tag read/write methods.
func eipConnect(v *vm.VM) int {
	addr := getString(v, 1, "eip.connect")

	retries := int64(0)
	retryDelay := 1.0
	timeout := 10.0

	if v.Get(2).IsTable() {
		opts := v.Get(2).AsTable()
		retries = tableGetInt(opts, "retries", retries)
		retryDelay = tableGetFloat(opts, "retry_delay", retryDelay)
		timeout = tableGetFloat(opts, "timeout", timeout)
	}

	ctx := v.Context()
	connCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
	defer cancel()

	client, err := ethernetip.Connect(connCtx, addr,
		ethernetip.WithRetries(int(retries)),
		ethernetip.WithRetryDelay(time.Duration(retryDelay*float64(time.Second))),
		ethernetip.WithLogger(logging.NewNopLogger()),
	)
	if err != nil {
		luaErrorf("eip.connect: %s", err.Error())
	}

	v.Set(0, vm.NewTable(eipClientToLua(client, ctx)))
	return 1
}

// eipClientToLua wraps an *ethernetip.Client in a Lua table with methods.
// All methods expect to be called with colon syntax (client:method(args)),
// so the first stack argument (index 1) is the self table, and real
// arguments start at index 2.
func eipClientToLua(client *ethernetip.Client, ctx context.Context) *vm.Table {
	t := vm.NewEmptyTable()

	// read_tag(name) -> typed value (int, float, bool, or string)
	t.SetString("read_tag", vm.NewNativeFunc(func(v *vm.VM) int {
		tagName := getString(v, 2, "client:read_tag")

		data, err := client.ReadTag(ctx, tagName)
		if err != nil {
			luaErrorf("read_tag(%q): %s", tagName, err.Error())
		}

		v.Set(0, cipDataToLuaValue(data))
		return 1
	}))

	// read_tag_raw(name, count?) -> string of raw bytes
	t.SetString("read_tag_raw", vm.NewNativeFunc(func(v *vm.VM) int {
		tagName := getString(v, 2, "client:read_tag_raw")
		count := uint16(getOptInt(v, 3, 1))

		data, err := client.ReadTagElements(ctx, tagName, count)
		if err != nil {
			luaErrorf("read_tag_raw(%q): %s", tagName, err.Error())
		}

		v.Set(0, vm.NewString(string(data)))
		return 1
	}))

	// read_tags({name1, name2, ...}) -> {val1, val2, ...}
	t.SetString("read_tags", vm.NewNativeFunc(func(v *vm.VM) int {
		tbl := getTable(v, 2, "client:read_tags")

		result := vm.NewEmptyTable()
		length := tbl.Len()
		for i := 1; i <= length; i++ {
			tagVal := tbl.Get(vm.NewInt(int64(i)))
			if !tagVal.IsString() {
				luaErrorf("read_tags: element %d must be a string tag name", i)
			}
			tagName := tagVal.AsString()

			data, err := client.ReadTag(ctx, tagName)
			if err != nil {
				luaErrorf("read_tags(%q): %s", tagName, err.Error())
			}

			result.Set(vm.NewInt(int64(i)), cipDataToLuaValue(data))
		}

		v.Set(0, vm.NewTable(result))
		return 1
	}))

	// write_tag(name, value, type?)
	t.SetString("write_tag", vm.NewNativeFunc(func(v *vm.VM) int {
		tagName := getString(v, 2, "client:write_tag")
		luaVal := v.Get(3)
		typeHint := getOptString(v, 4, "")

		goVal := luaValueToGoForWrite(luaVal, typeHint)

		if err := client.WriteTag(ctx, tagName, goVal); err != nil {
			luaErrorf("write_tag(%q): %s", tagName, err.Error())
		}

		return 0
	}))

	// read_timer(name) -> table with pre, acc, en, tt, dn
	t.SetString("read_timer", vm.NewNativeFunc(func(v *vm.VM) int {
		tagName := getString(v, 2, "client:read_timer")

		timer, err := client.ReadTimer(ctx, tagName)
		if err != nil {
			luaErrorf("read_timer(%q): %s", tagName, err.Error())
		}

		result := vm.NewEmptyTable()
		result.SetString("pre", vm.NewInt(int64(timer.PRE)))
		result.SetString("acc", vm.NewInt(int64(timer.ACC)))
		result.SetString("en", vm.NewBool(timer.EN))
		result.SetString("tt", vm.NewBool(timer.TT))
		result.SetString("dn", vm.NewBool(timer.DN))

		v.Set(0, vm.NewTable(result))
		return 1
	}))

	// read_counter(name) -> table with pre, acc, cu, cd, dn, ov, un
	t.SetString("read_counter", vm.NewNativeFunc(func(v *vm.VM) int {
		tagName := getString(v, 2, "client:read_counter")

		data, err := client.ReadTag(ctx, tagName)
		if err != nil {
			luaErrorf("read_counter(%q): %s", tagName, err.Error())
		}

		if len(data) < 2 {
			luaErrorf("read_counter(%q): response too short", tagName)
		}
		typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
		hdrLen := 2
		if typeCode >= cip.TypeSTRUCT {
			hdrLen = 4
		}
		if len(data) < hdrLen {
			luaErrorf("read_counter(%q): response too short for header", tagName)
		}

		counter, err := cip.DecodeCounter(data[hdrLen:])
		if err != nil {
			luaErrorf("read_counter(%q): %s", tagName, err.Error())
		}

		result := vm.NewEmptyTable()
		result.SetString("pre", vm.NewInt(int64(counter.PRE)))
		result.SetString("acc", vm.NewInt(int64(counter.ACC)))
		result.SetString("cu", vm.NewBool(counter.CU))
		result.SetString("cd", vm.NewBool(counter.CD))
		result.SetString("dn", vm.NewBool(counter.DN))
		result.SetString("ov", vm.NewBool(counter.OV))
		result.SetString("un", vm.NewBool(counter.UN))

		v.Set(0, vm.NewTable(result))
		return 1
	}))

	// list_tags() -> table of {id=N, name="...", type=N}
	t.SetString("list_tags", vm.NewNativeFunc(func(v *vm.VM) int {
		// self at index 1, no other args
		tags, err := client.ListTags(ctx)
		if err != nil {
			luaErrorf("list_tags: %s", err.Error())
		}

		result := vm.NewEmptyTable()
		for i, tag := range tags {
			entry := vm.NewEmptyTable()
			entry.SetString("id", vm.NewInt(int64(tag.InstanceID)))
			entry.SetString("name", vm.NewString(tag.Name))
			entry.SetString("type", vm.NewInt(int64(tag.Type)))
			result.Set(vm.NewInt(int64(i+1)), vm.NewTable(entry))
		}

		v.Set(0, vm.NewTable(result))
		return 1
	}))

	// close()
	t.SetString("close", vm.NewNativeFunc(func(v *vm.VM) int {
		// self at index 1
		if err := client.Close(); err != nil {
			luaErrorf("close: %s", err.Error())
		}
		return 0
	}))

	return t
}

// cipDataToLuaValue converts raw CIP response bytes (with 2-byte type code
// prefix) to an appropriate Lua value.
func cipDataToLuaValue(data []byte) vm.Value {
	if len(data) < 2 {
		return vm.NewString(string(data))
	}

	typeCode := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	hdrLen := 2
	if typeCode >= cip.TypeSTRUCT {
		hdrLen = 4
	}
	if len(data) < hdrLen {
		return vm.NewString(string(data))
	}
	payload := data[hdrLen:]

	switch typeCode {
	case cip.TypeBOOL:
		if len(payload) >= 1 {
			return vm.NewBool(payload[0] != 0)
		}
	case cip.TypeSINT:
		if len(payload) >= 1 {
			return vm.NewInt(int64(int8(payload[0])))
		}
	case cip.TypeINT:
		if len(payload) >= 2 {
			return vm.NewInt(int64(int16(binary.LittleEndian.Uint16(payload))))
		}
	case cip.TypeDINT:
		if len(payload) >= 4 {
			return vm.NewInt(int64(int32(binary.LittleEndian.Uint32(payload))))
		}
	case cip.TypeLINT:
		if len(payload) >= 8 {
			return vm.NewInt(int64(binary.LittleEndian.Uint64(payload)))
		}
	case cip.TypeUSINT, cip.TypeBYTE:
		if len(payload) >= 1 {
			return vm.NewInt(int64(payload[0]))
		}
	case cip.TypeUINT, cip.TypeWORD:
		if len(payload) >= 2 {
			return vm.NewInt(int64(binary.LittleEndian.Uint16(payload)))
		}
	case cip.TypeUDINT, cip.TypeDWORD:
		if len(payload) >= 4 {
			return vm.NewInt(int64(binary.LittleEndian.Uint32(payload)))
		}
	case cip.TypeULINT, cip.TypeLWORD:
		if len(payload) >= 8 {
			// Lua integers are int64; large uint64 may lose precision
			return vm.NewInt(int64(binary.LittleEndian.Uint64(payload)))
		}
	case cip.TypeREAL:
		if len(payload) >= 4 {
			bits := binary.LittleEndian.Uint32(payload)
			return vm.NewFloat(float64(math.Float32frombits(bits)))
		}
	case cip.TypeLREAL:
		if len(payload) >= 8 {
			bits := binary.LittleEndian.Uint64(payload)
			return vm.NewFloat(math.Float64frombits(bits))
		}
	case cip.TypeSTRING:
		// CIP STRING: UINT length + bytes
		if len(payload) >= 2 {
			strLen := int(binary.LittleEndian.Uint16(payload[0:2]))
			if len(payload) >= 2+strLen {
				return vm.NewString(string(payload[2 : 2+strLen]))
			}
		}
	}

	// Fallback: return raw hex representation for unknown types.
	return vm.NewString(fmt.Sprintf("raw[0x%04X](%d bytes)", typeCode, len(payload)))
}

// luaValueToGoForWrite converts a Lua value to the appropriate Go type for
// WriteTag. An optional typeHint string can force the CIP type.
func luaValueToGoForWrite(val vm.Value, typeHint string) any {
	if typeHint != "" {
		return luaValueWithTypeHint(val, typeHint)
	}

	// Auto-detect from Lua type.
	switch {
	case val.IsBool():
		return val.AsBool()
	case val.IsInt():
		return int32(val.AsInt()) // Default integer → DINT
	case val.IsFloat():
		return float32(val.AsFloat()) // Default float → REAL
	case val.IsString():
		return val.AsString()
	default:
		luaErrorf("write_tag: unsupported Lua type %s", val.Type())
		return nil
	}
}

// luaValueWithTypeHint converts a Lua value to a specific Go type based on the
// CIP type name hint.
func luaValueWithTypeHint(val vm.Value, hint string) any {
	switch strings.ToUpper(hint) {
	case "BOOL":
		if val.IsBool() {
			return val.AsBool()
		}
		return val.AsInt() != 0
	case "SINT":
		return int8(val.AsInt())
	case "INT":
		return int16(val.AsInt())
	case "DINT":
		return int32(val.AsInt())
	case "LINT":
		return int64(val.AsInt())
	case "USINT":
		return uint8(val.AsInt())
	case "UINT":
		return uint16(val.AsInt())
	case "UDINT":
		return uint32(val.AsInt())
	case "ULINT":
		return uint64(val.AsInt())
	case "REAL":
		return float32(val.AsFloat())
	case "LREAL":
		return float64(val.AsFloat())
	case "STRING":
		return val.AsString()
	default:
		luaErrorf("write_tag: unknown CIP type %q", hint)
		return nil
	}
}
