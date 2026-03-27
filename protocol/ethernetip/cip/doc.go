// Package cip implements the Common Industrial Protocol (CIP) application
// layer used by EtherNet/IP for device communication.
//
// CIP defines a uniform set of services, objects, and encoding rules shared
// across EtherNet/IP, DeviceNet, and ControlNet. This package provides:
//
//   - Message encoding and decoding: [MessageRouterRequest] and
//     [MessageRouterResponse] represent the CIP message router PDU. Use
//     [Marshal] / [Unmarshal] or the [Marshaler] / [Unmarshaler] interfaces
//     to convert between Go values and CIP wire format.
//
//   - EPATH construction: [Path] builds encoded paths that address CIP objects
//     by class, instance, attribute, or symbolic tag name (ANSI Extended Symbol
//     segment). Paths are the primary addressing mechanism in CIP.
//
//   - Service helpers: [NewReadTagRequest] and [NewWriteTagRequest] construct
//     Rockwell Logix Read Tag (0x4C) and Write Tag (0x4D) service requests.
//     [NewGetAttributeSingleRequest] and [NewSetAttributeSingleRequest] cover
//     the generic CIP attribute services.
//
//   - Message routing: [MessageRouter] dispatches incoming requests to
//     registered [Object] implementations by class ID.
//
//   - Vendor-specific structures: [Timer] and [Counter] decode the 14-byte
//     Rockwell Logix timer (TON/TOF/RTO) and counter (CTU/CTD) structures,
//     including status bits, preset, and accumulated values.
//
//   - Tag enumeration: [SymbolInstance] and the associated request/response
//     helpers query the Symbol Object (class 0x6B) to list all tags on a
//     Logix controller.
//
// All multi-byte values use little-endian byte order, matching the CIP
// specification.
package cip
