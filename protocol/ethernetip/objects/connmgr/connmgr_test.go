package connmgr

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// buildOpenRequest creates a ForwardOpenRequest with the given triad and
// connection path, encodes it, and returns the encoded bytes.
func buildOpenRequest(t *testing.T, serial cip.UINT, vendor cip.UINT, origSN cip.UDINT, connPath []byte) []byte {
	t.Helper()
	req := &ForwardOpenRequest{
		PriorityTimeTick:            0x0A,
		TimeoutTicks:                0xF0,
		OTConnectionID:              0x1234,
		TOConnectionID:              0,
		ConnectionSerialNumber:      serial,
		VendorID:                    vendor,
		OriginatorSerialNumber:      origSN,
		ConnectionTimeoutMultiplier: 3,
		OTRPI:                       10000,
		OTNetworkConnectionParams:   0x43F4,
		TORPI:                       10000,
		TONetworkConnectionParams:   0x43F4,
		TransportTypeTrigger:        0x01,
		ConnectionPath:              connPath,
	}
	data, err := req.Encode()
	if err != nil {
		t.Fatalf("Encode ForwardOpenRequest: %v", err)
	}
	return data
}

// buildCloseRequest creates a ForwardCloseRequest with the given triad,
// encodes it, and returns the encoded bytes.
func buildCloseRequest(t *testing.T, serial cip.UINT, vendor cip.UINT, origSN cip.UDINT, connPath []byte) []byte {
	t.Helper()
	req := &ForwardCloseRequest{
		PriorityTimeTick:       0x0A,
		TimeoutTicks:           0xF0,
		ConnectionSerialNumber: serial,
		VendorID:               vendor,
		OriginatorSerialNumber: origSN,
		ConnectionPath:         connPath,
	}
	data, err := req.Encode()
	if err != nil {
		t.Fatalf("Encode ForwardCloseRequest: %v", err)
	}
	return data
}

func connPath() []byte {
	p := cip.NewPath()
	p.AddClass(cip.UINT(0x04))
	p.AddInstance(100)
	p.AddClass(cip.UINT(0x04))
	p.AddInstance(101)
	return p.Bytes()
}

func TestForwardOpenRequestEncodeRoundTrip(t *testing.T) {
	cm := NewConnectionManager()
	data := buildOpenRequest(t, 0x0001, 0x0042, 0xABCD, connPath())

	resp, err := cm.HandleForwardOpen(data)
	if err != nil {
		t.Fatalf("HandleForwardOpen: %v", err)
	}

	// Parse the response to verify the triad is echoed back.
	r := bytes.NewReader(resp)
	var otID, toID cip.UDINT
	var serial, vendor cip.UINT
	var origSN cip.UDINT
	binary.Read(r, binary.LittleEndian, &otID)
	binary.Read(r, binary.LittleEndian, &toID)
	binary.Read(r, binary.LittleEndian, &serial)
	binary.Read(r, binary.LittleEndian, &vendor)
	binary.Read(r, binary.LittleEndian, &origSN)

	if otID != 0x1234 {
		t.Errorf("OTConnectionID = 0x%08X, want 0x1234", otID)
	}
	if toID == 0 {
		t.Error("TOConnectionID should not be 0")
	}
	if serial != 0x0001 {
		t.Errorf("ConnectionSerialNumber = 0x%04X, want 0x0001", serial)
	}
	if vendor != 0x0042 {
		t.Errorf("VendorID = 0x%04X, want 0x0042", vendor)
	}
	if origSN != 0xABCD {
		t.Errorf("OriginatorSerialNumber = 0x%08X, want 0xABCD", origSN)
	}
}

func TestForwardCloseRequestEncodeRoundTrip(t *testing.T) {
	cm := NewConnectionManager()

	// Open a connection first so there is something to close.
	openData := buildOpenRequest(t, 0x0007, 0x0099, 0xDEAD, connPath())
	_, err := cm.HandleForwardOpen(openData)
	if err != nil {
		t.Fatalf("HandleForwardOpen: %v", err)
	}

	closeData := buildCloseRequest(t, 0x0007, 0x0099, 0xDEAD, connPath())
	resp, err := cm.HandleForwardClose(closeData)
	if err != nil {
		t.Fatalf("HandleForwardClose: %v", err)
	}

	r := bytes.NewReader(resp)
	var serial, vendor cip.UINT
	var origSN cip.UDINT
	binary.Read(r, binary.LittleEndian, &serial)
	binary.Read(r, binary.LittleEndian, &vendor)
	binary.Read(r, binary.LittleEndian, &origSN)

	if serial != 0x0007 {
		t.Errorf("ConnectionSerialNumber = 0x%04X, want 0x0007", serial)
	}
	if vendor != 0x0099 {
		t.Errorf("VendorID = 0x%04X, want 0x0099", vendor)
	}
	if origSN != 0xDEAD {
		t.Errorf("OriginatorSerialNumber = 0x%08X, want 0xDEAD", origSN)
	}
}

func TestConnectionSizeFromParams(t *testing.T) {
	tests := []struct {
		name   string
		params cip.WORD
		want   uint16
	}{
		{"zero", 0x0000, 0},
		{"small fixed", ConnParamFixedSize | 64, 64},
		{"max 9-bit", 0x01FF, 511},
		{"variable with flags", ConnParamVariableSize | ConnParamPointToPoint | ConnParamPriorityHigh | 200, 200},
		{"high bits ignored", 0xFE00 | 100, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConnectionSizeFromParams(tt.params)
			if got != tt.want {
				t.Errorf("ConnectionSizeFromParams(0x%04X) = %d, want %d", tt.params, got, tt.want)
			}
		})
	}
}

func TestOnOpenOnCloseCallbacks(t *testing.T) {
	var mu sync.Mutex
	var openConn *Connection
	var openReq *ForwardOpenRequest
	var closeConn *Connection

	cm := NewConnectionManager(
		WithOnOpen(func(c *Connection, req *ForwardOpenRequest) {
			mu.Lock()
			openConn = c
			openReq = req
			mu.Unlock()
		}),
		WithOnClose(func(c *Connection) {
			mu.Lock()
			closeConn = c
			mu.Unlock()
		}),
	)

	// Forward Open
	openData := buildOpenRequest(t, 0x0010, 0x0042, 0xBEEF, connPath())
	_, err := cm.HandleForwardOpen(openData)
	if err != nil {
		t.Fatalf("HandleForwardOpen: %v", err)
	}

	mu.Lock()
	if openConn == nil {
		t.Fatal("OnOpen callback not fired")
	}
	if openConn.ConnectionSerialNumber != 0x0010 {
		t.Errorf("OnOpen conn serial = 0x%04X, want 0x0010", openConn.ConnectionSerialNumber)
	}
	if openConn.VendorID != 0x0042 {
		t.Errorf("OnOpen conn vendor = 0x%04X, want 0x0042", openConn.VendorID)
	}
	if openReq == nil {
		t.Fatal("OnOpen request is nil")
	}
	if openReq.OTRPI != 10000 {
		t.Errorf("OnOpen req OTRPI = %d, want 10000", openReq.OTRPI)
	}
	mu.Unlock()

	// Forward Close
	closeData := buildCloseRequest(t, 0x0010, 0x0042, 0xBEEF, connPath())
	_, err = cm.HandleForwardClose(closeData)
	if err != nil {
		t.Fatalf("HandleForwardClose: %v", err)
	}

	mu.Lock()
	if closeConn == nil {
		t.Fatal("OnClose callback not fired")
	}
	if closeConn.ConnectionSerialNumber != 0x0010 {
		t.Errorf("OnClose conn serial = 0x%04X, want 0x0010", closeConn.ConnectionSerialNumber)
	}
	mu.Unlock()
}

func TestConnectionTracking(t *testing.T) {
	cm := NewConnectionManager()

	// Open 3 connections with different triads.
	for i := 0; i < 3; i++ {
		data := buildOpenRequest(t, cip.UINT(i+1), 0x0042, cip.UDINT(i+100), connPath())
		_, err := cm.HandleForwardOpen(data)
		if err != nil {
			t.Fatalf("Open connection %d: %v", i, err)
		}
	}

	cm.mu.RLock()
	if len(cm.connections) != 3 {
		t.Errorf("tracked connections = %d, want 3", len(cm.connections))
	}
	cm.mu.RUnlock()

	// Close them all.
	for i := 0; i < 3; i++ {
		data := buildCloseRequest(t, cip.UINT(i+1), 0x0042, cip.UDINT(i+100), connPath())
		_, err := cm.HandleForwardClose(data)
		if err != nil {
			t.Fatalf("Close connection %d: %v", i, err)
		}
	}

	cm.mu.RLock()
	if len(cm.connections) != 0 {
		t.Errorf("tracked connections after close = %d, want 0", len(cm.connections))
	}
	cm.mu.RUnlock()
}

func TestDuplicateConnectionRejected(t *testing.T) {
	cm := NewConnectionManager()
	data := buildOpenRequest(t, 0x0001, 0x0042, 0xAAAA, connPath())

	// First open succeeds.
	_, err := cm.HandleForwardOpen(data)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	// Second open with the same triad must be rejected.
	_, err = cm.HandleForwardOpen(data)
	if err == nil {
		t.Fatal("expected error for duplicate triad, got nil")
	}

	cipErr, ok := err.(cip.Error)
	if !ok {
		t.Fatalf("expected cip.Error, got %T: %v", err, err)
	}
	if cipErr.Status != cip.StatusConnectionFailure {
		t.Errorf("status = 0x%02X, want 0x%02X", cipErr.Status, cip.StatusConnectionFailure)
	}
	if len(cipErr.ExtStatus) != 1 || cipErr.ExtStatus[0] != ExtStatusConnectionInUse {
		t.Errorf("ExtStatus = %v, want [0x%04X]", cipErr.ExtStatus, ExtStatusConnectionInUse)
	}

	// Only one connection should be tracked.
	cm.mu.RLock()
	count := len(cm.connections)
	cm.mu.RUnlock()
	if count != 1 {
		t.Errorf("tracked connections = %d, want 1", count)
	}

	// Close it, then re-open should succeed.
	closeData := buildCloseRequest(t, 0x0001, 0x0042, 0xAAAA, connPath())
	_, err = cm.HandleForwardClose(closeData)
	if err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = cm.HandleForwardOpen(data)
	if err != nil {
		t.Fatalf("re-open after close: %v", err)
	}
}

func TestHandleRequestDispatch(t *testing.T) {
	cm := NewConnectionManager()
	data := buildOpenRequest(t, 0x0001, 0x0042, 0x1111, connPath())

	// ServiceForwardOpen
	_, err := cm.HandleRequest(ServiceForwardOpen, nil, data)
	if err != nil {
		t.Fatalf("HandleRequest Forward_Open: %v", err)
	}

	// ServiceForwardClose
	closeData := buildCloseRequest(t, 0x0001, 0x0042, 0x1111, connPath())
	_, err = cm.HandleRequest(ServiceForwardClose, nil, closeData)
	if err != nil {
		t.Fatalf("HandleRequest Forward_Close: %v", err)
	}

	// Unsupported service
	_, err = cm.HandleRequest(0xFF, nil, nil)
	if err == nil {
		t.Fatal("expected error for unsupported service")
	}
	cipErr, ok := err.(cip.Error)
	if !ok {
		t.Fatalf("expected cip.Error, got %T", err)
	}
	if cipErr.Status != cip.StatusServiceNotSupported {
		t.Errorf("status = 0x%02X, want 0x%02X", cipErr.Status, cip.StatusServiceNotSupported)
	}
}
