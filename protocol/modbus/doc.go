// Package modbus implements a Modbus TCP client and server for industrial
// communication with PLCs and other automation devices.
//
// Modbus TCP is a widely used industrial protocol that runs over TCP/IP.
// It uses numbered function codes to read and write four types of data in
// a device:
//
//   - Coils: single-bit read/write outputs (function codes 1, 5, 15).
//   - Discrete inputs: single-bit read-only inputs (function code 2).
//   - Holding registers: 16-bit read/write data locations (function codes 3, 6, 16).
//   - Input registers: 16-bit read-only data (function code 4).
//
// Every Modbus TCP message is wrapped in an MBAP (Modbus Application Protocol)
// header that carries a transaction ID for request/response correlation, a
// protocol identifier, a length field, and a unit ID (also called slave
// address) that selects the target device on multi-drop networks.
//
// # Client
//
// Use [Connect] for the common case of dialing a Modbus TCP server:
//
//	client, err := modbus.Connect(ctx, "192.168.1.10",
//	    modbus.WithUnitID(1),
//	    modbus.WithRetries(2),
//	)
//	if err != nil { ... }
//	defer client.Close()
//
//	regs, err := client.ReadHoldingRegisters(ctx, 100, 10)
//
// The [Client] also implements the generic [github.com/iceisfun/goindustrial/plc.PLC]
// interface, so it can be used with the monitor and other protocol-agnostic
// tooling in this module.
//
// # Server
//
// Use [NewServer] to create a Modbus TCP server backed by a [DataStore]:
//
//	store := modbus.NewMemoryStore()
//	srv := modbus.NewServer("0.0.0.0",
//	    modbus.WithServerPort(502),
//	    modbus.WithServerDataStore(store),
//	)
//	if err := srv.Start(ctx); err != nil { ... }
//
// Custom function-code handlers can be registered with [Server.SetHandler].
package modbus
