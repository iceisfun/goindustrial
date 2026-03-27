// Package eip implements the EtherNet/IP encapsulation layer defined in the
// EtherNet/IP specification (Volume 2, Chapter 2).
//
// Every EtherNet/IP message begins with a 24-byte [EncapsulationHeader]
// followed by command-specific data. This package provides encoding and
// decoding for the header, the encapsulation commands (RegisterSession,
// SendRRData, ListIdentity, etc.), and the Common Packet Format (CPF)
// framing that carries CIP payloads.
//
// Key types:
//
//   - [EncapsulationHeader]: the 24-byte header present on every EIP message.
//   - [Command]: the set of encapsulation command codes.
//   - [CommonPacketFormat] and [CPFItem]: the item-based framing used inside
//     SendRRData and SendUnitData to carry CIP request/response data.
//   - [ListIdentityItem] and [ListServicesItem]: device discovery response
//     structures returned by the ListIdentity and ListServices commands.
//   - [RegisterSessionData]: the payload for the RegisterSession command that
//     establishes a session handle for subsequent communication.
//
// All multi-byte fields use little-endian byte order per the EtherNet/IP
// specification.
package eip
