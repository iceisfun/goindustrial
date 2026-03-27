package connmgr

import (
	"bytes"
	"encoding/binary"
	"sync"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// ConnMgrOption is a functional option for configuring a ConnectionManager.
type ConnMgrOption func(*ConnectionManager)

// WithOnOpen sets a callback invoked after a successful Forward_Open.
// The callback receives the new Connection and the parsed request.
func WithOnOpen(fn func(*Connection, *ForwardOpenRequest)) ConnMgrOption {
	return func(cm *ConnectionManager) {
		cm.onOpen = fn
	}
}

// WithOnClose sets a callback invoked after a successful Forward_Close.
func WithOnClose(fn func(*Connection)) ConnMgrOption {
	return func(cm *ConnectionManager) {
		cm.onClose = fn
	}
}

// ConnectionManager implements the CIP Connection Manager Object (Class 0x06).
// It processes Forward Open and Forward Close requests, maintains a table of
// active connections, and invokes optional callbacks when connections are
// opened or closed. All methods are safe for concurrent use.
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[uint32]*Connection
	nextConnID  uint32
	onOpen      func(*Connection, *ForwardOpenRequest)
	onClose     func(*Connection)
}

// Connection represents a logical CIP connection established via Forward Open.
// It is identified by the connection triad (ConnectionSerialNumber, VendorID,
// OriginatorSerialNumber) which is unique across all originators.
type Connection struct {
	OTConnectionID         uint32
	TOConnectionID         uint32
	ConnectionSerialNumber cip.UINT
	VendorID               cip.UINT
	OriginatorSerialNumber cip.UDINT
}

// NewConnectionManager creates a new ConnectionManager with the supplied
// options. Connection IDs are allocated starting at 0x80000000 to avoid
// collisions with originator-assigned IDs.
func NewConnectionManager(opts ...ConnMgrOption) *ConnectionManager {
	cm := &ConnectionManager{
		connections: make(map[uint32]*Connection),
		nextConnID:  0x80000000,
	}
	for _, opt := range opts {
		opt(cm)
	}
	return cm
}

// HandleForwardOpen processes a Forward_Open request from its raw CIP service
// data bytes. It deserializes the request, allocates a new target connection
// ID, stores the connection, invokes the OnOpen callback (if set), and returns
// the serialized ForwardOpenResponse.
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

	pathLen := int(req.ConnectionPathSize) * 2
	req.ConnectionPath = make([]byte, pathLen)
	if _, err := r.Read(req.ConnectionPath); err != nil {
		return nil, err
	}

	cm.mu.Lock()

	// Reject duplicate connection triads per CIP spec Vol.1 §3-5.5.2.
	for _, existing := range cm.connections {
		if existing.ConnectionSerialNumber == req.ConnectionSerialNumber &&
			existing.VendorID == req.VendorID &&
			existing.OriginatorSerialNumber == req.OriginatorSerialNumber {
			cm.mu.Unlock()
			return nil, cip.Error{
				Status:    cip.StatusConnectionFailure,
				ExtStatus: []cip.UINT{ExtStatusConnectionInUse},
			}
		}
	}

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
	cm.mu.Unlock()

	if cm.onOpen != nil {
		cm.onOpen(conn, req)
	}

	resp := &ForwardOpenResponse{
		OTConnectionID:         cip.UDINT(req.OTConnectionID),
		TOConnectionID:         cip.UDINT(myConnID),
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
		OTAPI:                  req.OTRPI,
		TOAPI:                  req.TORPI,
		ApplicationReplySize:   0,
		Reserved:               0,
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, resp.OTConnectionID)
	binary.Write(buf, binary.LittleEndian, resp.TOConnectionID)
	binary.Write(buf, binary.LittleEndian, resp.ConnectionSerialNumber)
	binary.Write(buf, binary.LittleEndian, resp.VendorID)
	binary.Write(buf, binary.LittleEndian, resp.OriginatorSerialNumber)
	binary.Write(buf, binary.LittleEndian, resp.OTAPI)
	binary.Write(buf, binary.LittleEndian, resp.TOAPI)
	binary.Write(buf, binary.LittleEndian, resp.ApplicationReplySize)
	binary.Write(buf, binary.LittleEndian, resp.Reserved)

	return buf.Bytes(), nil
}

// HandleForwardClose processes a Forward_Close request from its raw CIP
// service data bytes. It locates the connection by its triad
// (ConnectionSerialNumber, VendorID, OriginatorSerialNumber), removes it from
// the active table, invokes the OnClose callback (if set), and returns the
// serialized ForwardCloseResponse.
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

	// Find and remove connection by triad
	var closed *Connection
	cm.mu.Lock()
	for id, conn := range cm.connections {
		if conn.ConnectionSerialNumber == req.ConnectionSerialNumber &&
			conn.VendorID == req.VendorID &&
			conn.OriginatorSerialNumber == req.OriginatorSerialNumber {
			closed = conn
			delete(cm.connections, id)
			break
		}
	}
	cm.mu.Unlock()

	if closed != nil && cm.onClose != nil {
		cm.onClose(closed)
	}

	resp := &ForwardCloseResponse{
		ConnectionSerialNumber: req.ConnectionSerialNumber,
		VendorID:               req.VendorID,
		OriginatorSerialNumber: req.OriginatorSerialNumber,
		ApplicationReplySize:   0,
		Reserved:               0,
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, resp.ConnectionSerialNumber)
	binary.Write(buf, binary.LittleEndian, resp.VendorID)
	binary.Write(buf, binary.LittleEndian, resp.OriginatorSerialNumber)
	binary.Write(buf, binary.LittleEndian, resp.ApplicationReplySize)
	binary.Write(buf, binary.LittleEndian, resp.Reserved)

	return buf.Bytes(), nil
}

// HandleRequest implements the cip.Object interface. It dispatches to
// HandleForwardOpen or HandleForwardClose based on the service code.
// Unsupported services return a cip.Error with StatusServiceNotSupported.
func (cm *ConnectionManager) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	switch service {
	case ServiceForwardOpen:
		return cm.HandleForwardOpen(data)
	case ServiceForwardClose:
		return cm.HandleForwardClose(data)
	default:
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
}
