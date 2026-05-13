// Package lua provides GoLua bindings for the goindustrial library, exposing
// Modbus TCP and EtherNet/IP (CIP) protocol operations to Lua scripts.
//
// This package is optional — the core goindustrial library has zero external
// dependencies. Import this package only if you embed Lua scripting via golua.
//
// Usage:
//
//	import (
//	    "github.com/iceisfun/golua/v2/vm"
//	    "github.com/iceisfun/golua/v2/stdlib"
//	    industrialLua "github.com/iceisfun/goindustrial/lua"
//	)
//
//	v := vm.New()
//	stdlib.Open(v)
//	industrialLua.Open(v)
//
// This registers two global modules:
//
//   - modbus — Modbus TCP client operations
//   - eip    — EtherNet/IP (CIP) client operations
//
// Both modules create clients that use the VM context for cancellation and
// timeouts.
package lua

import (
	"github.com/iceisfun/golua/v2/vm"
)

// Open registers the goindustrial modules (modbus, eip) as globals in the VM.
func Open(v *vm.VM) {
	openModbus(v)
	openEIP(v)
}
