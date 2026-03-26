package connmgr

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/iceisfun/goeip/pkg/cip"
)

// ConnectionManager implements the CIP Connection Manager Object (Class 0x06)
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[uint32]*Connection // Map of ConnectionID -> Connection
	nextConnID  uint32
}

// Connection represents a logical CIP connection
type Connection struct {
	OTConnectionID         uint32
	TOConnectionID         uint32
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
	// Add more fields as needed for runtime
}

// NewConnectionManager creates a new Connection Manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[uint32]*Connection),
		nextConnID:  0x80000000, // Start high to avoid conflicts with typical PLC IDs? Or just 1.
	}
}

// HandleForwardOpen processes a Forward_Open request
func (cm *ConnectionManager) HandleForwardOpen(reqData []byte) ([]byte, error) {
	req := &ForwardOpenRequest{}
	r := bytes.NewReader(reqData)

	if err := binary.Read(r, binary.LittleEndian, &req.PriorityTimeTick); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TimeoutTicks); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TOConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionTimeoutMultiplier); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.Reserved); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTRPI); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTNetworkConnectionParams); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TORPI); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TONetworkConnectionParams); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TransportTypeTrigger); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionPathSize); err != nil {
		return nil, err
	}

	// Read Connection Path
	pathLen := int(req.ConnectionPathSize) * 2 // Words to Bytes
	req.ConnectionPath = make([]byte, pathLen)
	if _, err := r.Read(req.ConnectionPath); err != nil {
		return nil, err
	}

	// Logic to allocate Connection ID if we are the target (T->O)
	// For now, we assume we are the target and we need to allocate a T->O ID.
	// The O->T ID is provided by the originator.

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Simple ID allocation
	cm.nextConnID++
	myConnID := cm.nextConnID

	// Store connection (simplified)
	conn := &Connection{
		OTConnectionID:         uint32(req.OTConnectionID),
		TOConnectionID:         myConnID,
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
	}
	cm.connections[myConnID] = conn

	// Construct Response
	resp := &ForwardOpenResponse{
		OTConnectionID:         cip.UDINT(req.OTConnectionID),
		TOConnectionID:         cip.UDINT(myConnID),
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
		OTAPI:                  req.OTRPI, // Actual Packet Interval = Requested
		TOAPI:                  req.TORPI,
		ApplicationReplySize:   0,
		Reserved:               0,
		ApplicationReply:       nil,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, resp.OTConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.TOConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.OTAPI); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.TOAPI); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.ApplicationReplySize); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.Reserved); err != nil {
		return nil, err
	}
	// Application Reply is empty

	return buf.Bytes(), nil
}

// HandleForwardClose processes a Forward_Close request
func (cm *ConnectionManager) HandleForwardClose(reqData []byte) ([]byte, error) {
	req := &ForwardCloseRequest{}
	r := bytes.NewReader(reqData)

	if err := binary.Read(r, binary.LittleEndian, &req.PriorityTimeTick); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TimeoutTicks); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionPathSize); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.Reserved); err != nil {
		return nil, err
	}

	pathLen := int(req.ConnectionPathSize) * 2
	req.ConnectionPath = make([]byte, pathLen)
	if _, err := r.Read(req.ConnectionPath); err != nil {
		return nil, err
	}

	// Find and remove connection by triad (Serial, Vendor, Originator)
	cm.mu.Lock()
	for id, conn := range cm.connections {
		if conn.ConnectionSerialNumber == req.ConnectionSerialNumber &&
			conn.VendorID == req.VendorID &&
			conn.OriginatorSerialNumber == req.OriginatorSerialNumber {
			delete(cm.connections, id)
			break
		}
	}
	cm.mu.Unlock()

	resp := &ForwardCloseResponse{
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
		ApplicationReplySize:   0,
		Reserved:               0,
		ApplicationReply:       nil,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, resp.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.ApplicationReplySize); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.Reserved); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// HandleLargeForwardOpen processes a Large_Forward_Open request
func (cm *ConnectionManager) HandleLargeForwardOpen(reqData []byte) ([]byte, error) {
	req := &LargeForwardOpenRequest{}
	r := bytes.NewReader(reqData)

	if err := binary.Read(r, binary.LittleEndian, &req.PriorityTimeTick); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TimeoutTicks); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TOConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionTimeoutMultiplier); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.Reserved); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTRPI); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.OTNetworkConnectionParams); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TORPI); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TONetworkConnectionParams); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.TransportTypeTrigger); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &req.ConnectionPathSize); err != nil {
		return nil, err
	}

	pathLen := int(req.ConnectionPathSize) * 2
	req.ConnectionPath = make([]byte, pathLen)
	if _, err := r.Read(req.ConnectionPath); err != nil {
		return nil, err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.nextConnID++
	myConnID := cm.nextConnID

	conn := &Connection{
		OTConnectionID:         uint32(req.OTConnectionID),
		TOConnectionID:         myConnID,
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
	}
	cm.connections[myConnID] = conn

	resp := &LargeForwardOpenResponse{
		OTConnectionID:         cip.UDINT(req.OTConnectionID),
		TOConnectionID:         cip.UDINT(myConnID),
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
		OTAPI:                  req.OTRPI,
		TOAPI:                  req.TORPI,
		ApplicationReplySize:   0,
		Reserved:               0,
		ApplicationReply:       nil,
	}

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, resp.OTConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.TOConnectionID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.ConnectionSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.VendorID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.OriginatorSerialNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.OTAPI); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.TOAPI); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.ApplicationReplySize); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, resp.Reserved); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// HandleRequest implements the cip.Object interface
func (cm *ConnectionManager) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	switch service {
	case ServiceForwardOpen:
		return cm.HandleForwardOpen(data)
	case ServiceLargeForwardOpen:
		return cm.HandleLargeForwardOpen(data)
	case ServiceForwardClose:
		return cm.HandleForwardClose(data)
	default:
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
}
