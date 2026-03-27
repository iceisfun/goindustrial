package lua

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/iceisfun/golua/vm"
	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

// openModbus registers the "modbus" global module in the VM.
//
// Lua API:
//
//	client = modbus.connect(addr, opts?)   -- connect to a Modbus TCP server
//	client:read_holding_registers(addr, qty) -> table of integers
//	client:read_input_registers(addr, qty) -> table of integers
//	client:read_coils(addr, qty) -> table of booleans
//	client:read_discrete_inputs(addr, qty) -> table of booleans
//	client:write_register(addr, value)
//	client:write_registers(addr, {v1, v2, ...})
//	client:write_coil(addr, bool)
//	client:write_coils(addr, {true, false, ...})
//	client:read_write_registers(read_addr, read_qty, write_addr, {v1, ...}) -> table
//	client:read_device_id() -> table
//	client:to_int32(high, low) -> integer
//	client:to_float32(high, low) -> float
//	client:close()
//
// Note: All client methods are designed to be called with Lua colon syntax
// (client:method(args)), which passes the client table as the implicit first
// argument. The native functions skip this self parameter.
func openModbus(v *vm.VM) {
	mod := vm.NewEmptyTable()
	mod.SetString("connect", vm.NewNativeFunc(modbusConnect))
	v.SetGlobal("modbus", vm.NewTable(mod))
}

func modbusConnect(v *vm.VM) int {
	addr := getString(v, 1, "modbus.connect")

	port := int64(modbus.DefaultTCPPort)
	unit := int64(1)
	retries := int64(0)
	retryDelay := 0.5
	timeout := 10.0

	if v.Get(2).IsTable() {
		opts := v.Get(2).AsTable()
		port = tableGetInt(opts, "port", port)
		unit = tableGetInt(opts, "unit", unit)
		retries = tableGetInt(opts, "retries", retries)
		retryDelay = tableGetFloat(opts, "retry_delay", retryDelay)
		timeout = tableGetFloat(opts, "timeout", timeout)
	}

	ctx := v.Context()
	connCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
	defer cancel()

	client, err := modbus.Connect(connCtx, addr,
		modbus.WithPort(int(port)),
		modbus.WithUnitID(modbus.UnitID(unit)),
		modbus.WithRetries(int(retries)),
		modbus.WithRetryDelay(time.Duration(retryDelay*float64(time.Second))),
		modbus.WithLogger(logging.NewNopLogger()),
	)
	if err != nil {
		panic(fmt.Sprintf("modbus.connect: %s", err.Error()))
	}

	v.Set(0, vm.NewTable(modbusClientToLua(client, ctx)))
	return 1
}

// modbusClientToLua wraps a *modbus.Client in a Lua table with methods.
// All methods expect to be called with colon syntax (client:method(args)),
// so the first stack argument (index 1) is the self table, and real
// arguments start at index 2.
func modbusClientToLua(client *modbus.Client, ctx context.Context) *vm.Table {
	t := vm.NewEmptyTable()

	// read_holding_registers(address, quantity) -> table
	t.SetString("read_holding_registers", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:read_holding_registers"))
		quantity := modbus.Quantity(getInt(v, 3, "client:read_holding_registers"))

		regs, err := client.ReadHoldingRegisters(ctx, address, quantity)
		if err != nil {
			panic(fmt.Sprintf("read_holding_registers: %s", err.Error()))
		}

		v.Set(0, vm.NewTable(registersToLuaTable(regs)))
		return 1
	}))

	// read_input_registers(address, quantity) -> table
	t.SetString("read_input_registers", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:read_input_registers"))
		quantity := modbus.Quantity(getInt(v, 3, "client:read_input_registers"))

		regs, err := client.ReadInputRegisters(ctx, address, quantity)
		if err != nil {
			panic(fmt.Sprintf("read_input_registers: %s", err.Error()))
		}

		v.Set(0, vm.NewTable(registersToLuaTable(regs)))
		return 1
	}))

	// read_coils(address, quantity) -> table of bools
	t.SetString("read_coils", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:read_coils"))
		quantity := modbus.Quantity(getInt(v, 3, "client:read_coils"))

		vals, err := client.ReadCoils(ctx, address, quantity)
		if err != nil {
			panic(fmt.Sprintf("read_coils: %s", err.Error()))
		}

		v.Set(0, vm.NewTable(boolsToLuaTable(vals)))
		return 1
	}))

	// read_discrete_inputs(address, quantity) -> table of bools
	t.SetString("read_discrete_inputs", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:read_discrete_inputs"))
		quantity := modbus.Quantity(getInt(v, 3, "client:read_discrete_inputs"))

		vals, err := client.ReadDiscreteInputs(ctx, address, quantity)
		if err != nil {
			panic(fmt.Sprintf("read_discrete_inputs: %s", err.Error()))
		}

		v.Set(0, vm.NewTable(boolsToLuaTable(vals)))
		return 1
	}))

	// write_register(address, value)
	t.SetString("write_register", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:write_register"))
		value := modbus.RegisterValue(getInt(v, 3, "client:write_register"))

		if err := client.WriteSingleRegister(ctx, address, value); err != nil {
			panic(fmt.Sprintf("write_register: %s", err.Error()))
		}

		return 0
	}))

	// write_registers(address, {v1, v2, ...})
	t.SetString("write_registers", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:write_registers"))
		tbl := getTable(v, 3, "client:write_registers")

		values := luaTableToRegisters(tbl)
		if err := client.WriteMultipleRegisters(ctx, address, values); err != nil {
			panic(fmt.Sprintf("write_registers: %s", err.Error()))
		}

		return 0
	}))

	// write_coil(address, value)
	t.SetString("write_coil", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:write_coil"))
		val := v.Get(3)
		if !val.IsBool() {
			panic("bad argument #2 to 'client:write_coil' (boolean expected)")
		}

		if err := client.WriteSingleCoil(ctx, address, val.AsBool()); err != nil {
			panic(fmt.Sprintf("write_coil: %s", err.Error()))
		}

		return 0
	}))

	// write_coils(address, {true, false, ...})
	t.SetString("write_coils", vm.NewNativeFunc(func(v *vm.VM) int {
		address := modbus.Address(getInt(v, 2, "client:write_coils"))
		tbl := getTable(v, 3, "client:write_coils")

		values := luaTableToCoils(tbl)
		if err := client.WriteMultipleCoils(ctx, address, values); err != nil {
			panic(fmt.Sprintf("write_coils: %s", err.Error()))
		}

		return 0
	}))

	// read_write_registers(read_addr, read_qty, write_addr, {v1, v2, ...}) -> table
	t.SetString("read_write_registers", vm.NewNativeFunc(func(v *vm.VM) int {
		readAddr := modbus.Address(getInt(v, 2, "client:read_write_registers"))
		readQty := modbus.Quantity(getInt(v, 3, "client:read_write_registers"))
		writeAddr := modbus.Address(getInt(v, 4, "client:read_write_registers"))
		tbl := getTable(v, 5, "client:read_write_registers")

		writeValues := luaTableToRegisters(tbl)
		regs, err := client.ReadWriteMultipleRegisters(ctx, readAddr, readQty, writeAddr, writeValues)
		if err != nil {
			panic(fmt.Sprintf("read_write_registers: %s", err.Error()))
		}

		v.Set(0, vm.NewTable(registersToLuaTable(regs)))
		return 1
	}))

	// read_device_id() -> table with vendor_name, product_code, revision
	t.SetString("read_device_id", vm.NewNativeFunc(func(v *vm.VM) int {
		// self at index 1, no other args
		devID, err := client.ReadDeviceIdentification(ctx, modbus.ReadDeviceIDBasicStream, modbus.DeviceIDVendorName)
		if err != nil {
			panic(fmt.Sprintf("read_device_id: %s", err.Error()))
		}

		result := vm.NewEmptyTable()
		result.SetString("vendor_name", vm.NewString(devID.GetVendorName()))
		result.SetString("product_code", vm.NewString(devID.GetProductCode()))
		result.SetString("revision", vm.NewString(devID.GetRevision()))

		v.Set(0, vm.NewTable(result))
		return 1
	}))

	// to_int32(high_reg, low_reg) -> integer
	t.SetString("to_int32", vm.NewNativeFunc(func(v *vm.VM) int {
		high := uint16(getInt(v, 2, "client:to_int32"))
		low := uint16(getInt(v, 3, "client:to_int32"))

		buf := []byte{byte(high >> 8), byte(high), byte(low >> 8), byte(low)}
		val := int32(binary.BigEndian.Uint32(buf))

		v.Set(0, vm.NewInt(int64(val)))
		return 1
	}))

	// to_float32(high_reg, low_reg) -> float
	t.SetString("to_float32", vm.NewNativeFunc(func(v *vm.VM) int {
		high := uint16(getInt(v, 2, "client:to_float32"))
		low := uint16(getInt(v, 3, "client:to_float32"))

		buf := []byte{byte(high >> 8), byte(high), byte(low >> 8), byte(low)}
		bits := binary.BigEndian.Uint32(buf)

		v.Set(0, vm.NewFloat(float64(math.Float32frombits(bits))))
		return 1
	}))

	// close()
	t.SetString("close", vm.NewNativeFunc(func(v *vm.VM) int {
		// self at index 1
		if err := client.Close(); err != nil {
			panic(fmt.Sprintf("close: %s", err.Error()))
		}
		return 0
	}))

	return t
}

// luaTableToRegisters converts a 1-based Lua table of integers to a register slice.
func luaTableToRegisters(tbl vm.LuaTable) []modbus.RegisterValue {
	length := tbl.Len()
	if length <= 0 {
		panic("expected a non-empty table of register values")
	}

	values := make([]modbus.RegisterValue, length)
	for i := 1; i <= length; i++ {
		val := tbl.Get(vm.NewInt(int64(i)))
		if !val.IsNumber() {
			panic(fmt.Sprintf("table element %d: number expected, got %s", i, val.Type()))
		}
		values[i-1] = modbus.RegisterValue(val.AsInt())
	}
	return values
}

// luaTableToCoils converts a 1-based Lua table of booleans to a coil slice.
func luaTableToCoils(tbl vm.LuaTable) []modbus.CoilValue {
	length := tbl.Len()
	if length <= 0 {
		panic("expected a non-empty table of boolean values")
	}

	values := make([]modbus.CoilValue, length)
	for i := 1; i <= length; i++ {
		val := tbl.Get(vm.NewInt(int64(i)))
		if !val.IsBool() {
			panic(fmt.Sprintf("table element %d: boolean expected, got %s", i, val.Type()))
		}
		values[i-1] = val.AsBool()
	}
	return values
}
