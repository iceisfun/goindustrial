package connmgr

import (
	"github.com/iceisfun/goeip/pkg/cip"
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

// Status Codes
const (
	StatusConnectionFailure cip.USINT = 0x01
)

// Extended Status Codes for Connection Failure
const (
	ExtStatusConnectionInUse     cip.UINT = 0x0100
	ExtStatusTransportNotSupp    cip.UINT = 0x0103
	ExtStatusOwnershipConflict   cip.UINT = 0x0106
	ExtStatusConnectionNotFound  cip.UINT = 0x0109
	ExtStatusInvalidSegmentType  cip.UINT = 0x0315
	ExtStatusInvalidParam        cip.UINT = 0x0311 // Or similar
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
