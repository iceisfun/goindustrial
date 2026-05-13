// Package goindustrial provides Go libraries for communicating with industrial
// programmable logic controllers (PLCs) over Modbus TCP and EtherNet/IP (CIP).
//
// # Protocols
//
// Two protocol implementations are included:
//
//   - [github.com/iceisfun/goindustrial/protocol/modbus] — Modbus TCP client
//     and server for reading and writing coils, discrete inputs, holding
//     registers, and input registers.
//
//   - [github.com/iceisfun/goindustrial/protocol/ethernetip] — EtherNet/IP
//     client, server, and I/O scanner for tag-based access to Allen-Bradley /
//     Rockwell Logix controllers.
//
// Both protocol clients implement the [github.com/iceisfun/goindustrial/plc.PLC]
// interface, which provides a single Read/Write surface that works across
// protocols.
//
// # Supporting packages
//
//   - [github.com/iceisfun/goindustrial/plc] — protocol-agnostic PLC
//     abstraction and value types.
//   - [github.com/iceisfun/goindustrial/transport] — shared TCP transport with
//     automatic reconnection and configurable backoff.
//   - [github.com/iceisfun/goindustrial/hexdump] — wire-level hex dump tracing
//     for protocol connections.
//   - [github.com/iceisfun/goindustrial/monitor] — periodic polling monitor
//     that watches data points and invokes callbacks on change or error.
//   - [github.com/iceisfun/goindustrial/logging] — structured logging interface
//     used throughout the module.
//   - [github.com/iceisfun/goindustrial/lua] — optional Lua scripting bindings
//     via [github.com/iceisfun/golua/v2].
package goindustrial
