// Package pccc implements the PCCC (Programmable Controller Communication
// Commands) application protocol used by Allen-Bradley SLC 500, MicroLogix,
// and PLC-5 controllers.
//
// PCCC is an Allen-Bradley legacy protocol that addresses data-table files
// such as N7:0, B3:0/2, F8:5, or T4:0.ACC. It predates CIP and has no
// concept of named tags. On EtherNet/IP, a PCCC command is tunneled inside
// the CIP Execute_PCCC service (class 0x67, service 0x4B); on legacy media
// it rides over DF1 serial.
//
// This package implements only the framing layer: encoding PCCC command
// packets, decoding reply packets, and translating PCCC status (STS and
// EXT STS) codes into [Error] values. The CIP/EIP wrapper that actually
// sends a frame on the wire is provided by the parent ethernetip package.
//
// The SLC typed read (FNC 0xA2) and typed write (FNC 0xAA) commands cover
// SLC 5/0x and MicroLogix data-table access. PLC-5 word-range commands
// (FNC 0x01 / 0x08) are not in scope for this package.
//
// All PCCC multi-byte values are little-endian, consistent with both DF1
// and the CIP encapsulation.
package pccc
