// Package ethernetip implements an EtherNet/IP client and server for
// communicating with industrial controllers such as Allen-Bradley/Rockwell
// Logix PLCs.
//
// EtherNet/IP is an industrial Ethernet protocol that uses TCP for explicit
// (request/response) messaging and UDP for implicit (cyclic I/O) messaging.
// The protocol carries CIP (Common Industrial Protocol) messages over standard
// Ethernet hardware.
//
// The package provides three main entry points:
//
//   - [Client] performs explicit messaging over TCP. It reads and writes PLC
//     tags by name (e.g. "Motor_Speed"), enumerates tags, and queries device
//     identity. Use [Connect] for a one-shot connection, or
//     [NewReconnectingClient] for an auto-reconnecting client.
//
//   - [IOScanner] manages implicit I/O connections. It sends a CIP
//     Forward_Open over TCP to establish a cyclic data exchange, then
//     produces and consumes assembly data over UDP at the negotiated
//     Requested Packet Interval (RPI).
//
//   - [Server] implements an EtherNet/IP adapter (target). It accepts TCP
//     connections, registers sessions, and routes incoming CIP requests
//     through a [cip.MessageRouter].
package ethernetip
