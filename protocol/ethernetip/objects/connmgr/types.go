// Package connmgr implements the CIP Connection Manager Object (Class 0x06)
// for EtherNet/IP.
//
// The Connection Manager is the CIP object responsible for establishing,
// maintaining, and tearing down logical connections between devices. It
// handles Forward Open and Forward Close requests, which are the mechanism
// EtherNet/IP scanners use to set up implicit (cyclic UDP) I/O connections
// with adapters.
//
// This package provides request/response types for Forward Open, Large
// Forward Open, and Forward Close services, along with a ConnectionManager
// that tracks active connections and dispatches incoming CIP requests.
package connmgr

import (
	"bytes"
	"encoding/binary"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// CIP service codes defined by the Connection Manager Object (Class 0x06).
const (
	// ServiceForwardClose is the Forward_Close service (0x4E) used to tear down a CIP connection.
	ServiceForwardClose cip.USINT = 0x4E
	// ServiceUnconnectedSend is the Unconnected_Send service (0x52) used to route unconnected messages.
	ServiceUnconnectedSend cip.USINT = 0x52
	// ServiceForwardOpen is the Forward_Open service (0x54) used to establish a CIP connection.
	ServiceForwardOpen cip.USINT = 0x54
	// ServiceLargeForwardOpen is the Large_Forward_Open service (0x5B) for connections needing 32-bit parameters.
	ServiceLargeForwardOpen cip.USINT = 0x5B
	// ServiceGetConnectionData is the Get_Connection_Data service (0x56).
	ServiceGetConnectionData cip.USINT = 0x56
	// ServiceSearchConnection is the Search_Connection service (0x57).
	ServiceSearchConnection cip.USINT = 0x57
	// ServiceCloseConnection is the Close_Connection service (0x58).
	ServiceCloseConnection cip.USINT = 0x58
)

// Extended status codes returned by the Connection Manager when a Forward Open
// or Forward Close fails. These appear in the CIP error response as additional
// status words.
const (
	// ExtStatusConnectionInUse indicates the requested connection resources are already in use.
	ExtStatusConnectionInUse cip.UINT = 0x0100
	// ExtStatusTransportNotSupp indicates the requested transport class/trigger is not supported.
	ExtStatusTransportNotSupp cip.UINT = 0x0103
	// ExtStatusOwnershipConflict indicates another originator already owns the connection point.
	ExtStatusOwnershipConflict cip.UINT = 0x0106
	// ExtStatusConnectionNotFound indicates no matching connection was found for the close request.
	ExtStatusConnectionNotFound cip.UINT = 0x0109
	// ExtStatusInvalidSegmentType indicates a path segment type is not recognized.
	ExtStatusInvalidSegmentType cip.UINT = 0x0315
	// ExtStatusInvalidParam indicates a parameter value is out of range or inconsistent.
	ExtStatusInvalidParam cip.UINT = 0x0311
	// ExtStatusVendorSpecificError is a vendor-defined error code.
	ExtStatusVendorSpecificError cip.UINT = 0x031C
)

// ForwardOpenRequest represents the service data for a CIP Forward_Open
// (0x54) request. It carries all the parameters the originator proposes for
// the new connection, including requested packet intervals (RPI), connection
// sizes, and the connection path that identifies the target assembly
// instances.
type ForwardOpenRequest struct {
	PriorityTimeTick            cip.BYTE
	TimeoutTicks                cip.USINT
	OTConnectionID              cip.UDINT
	TOConnectionID              cip.UDINT
	ConnectionSerialNumber      cip.UINT
	VendorID                    cip.UINT
	OriginatorSerialNumber      cip.UDINT
	ConnectionTimeoutMultiplier cip.USINT
	Reserved                    [3]cip.BYTE
	OTRPI                       cip.UDINT
	OTNetworkConnectionParams   cip.WORD
	TORPI                       cip.UDINT
	TONetworkConnectionParams   cip.WORD
	TransportTypeTrigger        cip.BYTE
	ConnectionPathSize          cip.USINT
	ConnectionPath              []byte // Padded to even number of bytes
}

// ForwardOpenResponse represents the success response for a CIP Forward_Open
// request. It echoes back the connection identifiers and provides the actual
// packet intervals (API) the target will use, which may differ from the
// requested intervals.
type ForwardOpenResponse struct {
	OTConnectionID         cip.UDINT
	TOConnectionID         cip.UDINT
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
	OTAPI                  cip.UDINT // Actual Packet Interval
	TOAPI                  cip.UDINT
	ApplicationReplySize   cip.USINT
	Reserved               cip.USINT
	ApplicationReply       []byte
}

// ForwardCloseRequest represents the service data for a CIP Forward_Close
// (0x4E) request. The connection is identified by the connection triad:
// ConnectionSerialNumber, VendorID, and OriginatorSerialNumber.
type ForwardCloseRequest struct {
	PriorityTimeTick       cip.BYTE
	TimeoutTicks           cip.USINT
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
	ConnectionPathSize     cip.USINT
	Reserved               cip.USINT
	ConnectionPath         []byte // Padded
}

// ForwardCloseResponse represents the success response for a CIP
// Forward_Close request. It echoes the connection triad so the originator
// can match it to the closed connection.
type ForwardCloseResponse struct {
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
	ApplicationReplySize   cip.USINT
	Reserved               cip.USINT
	ApplicationReply       []byte
}

// ConnectionSizeFromParams extracts the connection size (bytes) from a 16-bit
// network connection parameter word. The size is encoded in bits 0-8.
func ConnectionSizeFromParams(params cip.WORD) uint16 {
	return uint16(params & 0x01FF)
}

// Network connection parameter bit flags for building the 16-bit
// OTNetworkConnectionParams and TONetworkConnectionParams fields in a
// Forward_Open request. Combine these with a connection size (bits 0-8)
// using bitwise OR.
const (
	// ConnParamFixedSize selects a fixed-size connection (bit 9 = 0).
	ConnParamFixedSize cip.WORD = 0x0000
	// ConnParamVariableSize selects a variable-size connection (bit 9 = 1).
	ConnParamVariableSize cip.WORD = 0x0200
	// ConnParamPointToPoint selects point-to-point connection type (bits 13-14).
	ConnParamPointToPoint cip.WORD = 0x4000
	// ConnParamMulticast selects multicast connection type (bits 13-14).
	ConnParamMulticast cip.WORD = 0x2000
	// ConnParamPriorityLow selects low priority (bits 10-11 = 00).
	ConnParamPriorityLow cip.WORD = 0x0000
	// ConnParamPriorityHigh selects high priority (bits 10-11 = 01).
	ConnParamPriorityHigh cip.WORD = 0x0400
	// ConnParamPriorityScheduled selects scheduled priority (bits 10-11 = 10).
	ConnParamPriorityScheduled cip.WORD = 0x0800
)

// Encode serializes the ForwardOpenRequest to bytes.
func (r *ForwardOpenRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, r.PriorityTimeTick)
	binary.Write(buf, binary.LittleEndian, r.TimeoutTicks)
	binary.Write(buf, binary.LittleEndian, r.OTConnectionID)
	binary.Write(buf, binary.LittleEndian, r.TOConnectionID)
	binary.Write(buf, binary.LittleEndian, r.ConnectionSerialNumber)
	binary.Write(buf, binary.LittleEndian, r.VendorID)
	binary.Write(buf, binary.LittleEndian, r.OriginatorSerialNumber)
	binary.Write(buf, binary.LittleEndian, r.ConnectionTimeoutMultiplier)
	binary.Write(buf, binary.LittleEndian, r.Reserved)
	binary.Write(buf, binary.LittleEndian, r.OTRPI)
	binary.Write(buf, binary.LittleEndian, r.OTNetworkConnectionParams)
	binary.Write(buf, binary.LittleEndian, r.TORPI)
	binary.Write(buf, binary.LittleEndian, r.TONetworkConnectionParams)
	binary.Write(buf, binary.LittleEndian, r.TransportTypeTrigger)
	binary.Write(buf, binary.LittleEndian, cip.USINT(len(r.ConnectionPath)/2))
	buf.Write(r.ConnectionPath)
	return buf.Bytes(), nil
}

// Encode serializes the ForwardCloseRequest to bytes.
func (r *ForwardCloseRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, r.PriorityTimeTick)
	binary.Write(buf, binary.LittleEndian, r.TimeoutTicks)
	binary.Write(buf, binary.LittleEndian, r.ConnectionSerialNumber)
	binary.Write(buf, binary.LittleEndian, r.VendorID)
	binary.Write(buf, binary.LittleEndian, r.OriginatorSerialNumber)
	binary.Write(buf, binary.LittleEndian, cip.USINT(len(r.ConnectionPath)/2))
	binary.Write(buf, binary.LittleEndian, r.Reserved)
	buf.Write(r.ConnectionPath)
	return buf.Bytes(), nil
}

// LargeForwardOpenRequest represents the service data for a CIP
// Large_Forward_Open (0x5B) request. It is identical to ForwardOpenRequest
// except the network connection parameter fields are 32 bits wide (DWORD),
// allowing connection sizes larger than 511 bytes.
type LargeForwardOpenRequest struct {
	PriorityTimeTick            cip.BYTE
	TimeoutTicks                cip.USINT
	OTConnectionID              cip.UDINT
	TOConnectionID              cip.UDINT
	ConnectionSerialNumber      cip.UINT
	VendorID                    cip.UINT
	OriginatorSerialNumber      cip.UDINT
	ConnectionTimeoutMultiplier cip.USINT
	Reserved                    [3]cip.BYTE
	OTRPI                       cip.UDINT
	OTNetworkConnectionParams   cip.DWORD // 32-bit for Large
	TORPI                       cip.UDINT
	TONetworkConnectionParams   cip.DWORD // 32-bit for Large
	TransportTypeTrigger        cip.BYTE
	ConnectionPathSize          cip.USINT
	ConnectionPath              []byte
}

// LargeForwardOpenResponse represents the success response for a CIP
// Large_Forward_Open request.
type LargeForwardOpenResponse struct {
	OTConnectionID         cip.UDINT
	TOConnectionID         cip.UDINT
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
	OTAPI                  cip.UDINT
	TOAPI                  cip.UDINT
	ApplicationReplySize   cip.USINT
	Reserved               cip.USINT
	ApplicationReply       []byte
}
