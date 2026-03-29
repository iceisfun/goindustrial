// Package hexdump provides wire-level hex dump tracing for industrial protocol
// connections. It wraps [io.Reader] and [io.Writer] interfaces to produce
// traditional hex dump output (similar to hexdump -C) for every read and write.
//
// The primary entry point is [NewDumper], which creates a [Dumper] that formats
// all traffic and writes it to a configurable [io.Writer]. Use
// [Dumper.WrapReader] and [Dumper.WrapWriter] to intercept traffic on any
// reader or writer.
//
// Both Modbus TCP and EtherNet/IP connection options accept a [Dumper] via
// their respective WithHexDump functions.
//
// # Output Format
//
// Each dump block begins with a direction header showing the transfer direction
// and byte count, followed by lines of hex and ASCII data:
//
//	>>> WRITE 12 bytes
//	00000000  00 01 00 00 00 06 01 03  00 00 00 0a              |............    |
//	<<< READ 15 bytes
//	00000000  00 01 00 00 00 09 01 03  06 00 01 00 02 00 03     |...............  |
//
// Short final lines are space-padded so the ASCII column always aligns,
// preserving the visual structure that makes hex dumps useful for humans.
package hexdump
