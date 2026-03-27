package connmgr

import (
	"bytes"
	"encoding/binary"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// Service Codes for Connection Manager
const (
	ServiceForwardClose      cip.USINT = 0x4E
	ServiceUnconnectedSend   cip.USINT = 0x52
	ServiceForwardOpen       cip.USINT = 0x54
	ServiceLargeForwardOpen  cip.USINT = 0x5B
	ServiceGetConnectionData cip.USINT = 0x56
	ServiceSearchConnection  cip.USINT = 0x57
	ServiceCloseConnection   cip.USINT = 0x58
)

// Extended Status Codes for Connection Failure
const (
	ExtStatusConnectionInUse     cip.UINT = 0x0100
	ExtStatusTransportNotSupp    cip.UINT = 0x0103
	ExtStatusOwnershipConflict   cip.UINT = 0x0106
	ExtStatusConnectionNotFound  cip.UINT = 0x0109
	ExtStatusInvalidSegmentType  cip.UINT = 0x0315
	ExtStatusInvalidParam        cip.UINT = 0x0311
	ExtStatusVendorSpecificError cip.UINT = 0x031C
)

// ForwardOpenRequest represents the data for a Forward_Open service
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

// ForwardOpenResponse represents the success response for Forward_Open
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

// ForwardCloseRequest represents the data for a Forward_Close service
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

// ForwardCloseResponse represents the success response for Forward_Close
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

// Network connection parameter flags for building Forward_Open requests.
const (
	ConnParamFixedSize      cip.WORD = 0x0000
	ConnParamVariableSize   cip.WORD = 0x0200
	ConnParamPointToPoint   cip.WORD = 0x4000
	ConnParamMulticast      cip.WORD = 0x2000
	ConnParamPriorityLow    cip.WORD = 0x0000
	ConnParamPriorityHigh   cip.WORD = 0x0400
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

// LargeForwardOpenRequest represents the data for a Large_Forward_Open service
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

// LargeForwardOpenResponse represents the success response for Large_Forward_Open
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
