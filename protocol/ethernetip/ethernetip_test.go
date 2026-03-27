package ethernetip

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/connmgr"
)

// ---------------------------------------------------------------------------
// pipeListener implements net.Listener by returning a pre-created conn on the
// first Accept() call, then blocking until Close() is called.
// ---------------------------------------------------------------------------

type pipeListener struct {
	connCh chan net.Conn
	done   chan struct{}
	once   sync.Once
}

func newPipeListener(serverConn net.Conn) *pipeListener {
	ch := make(chan net.Conn, 1)
	ch <- serverConn
	return &pipeListener{
		connCh: ch,
		done:   make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connCh:
		return c, nil
	case <-l.done:
		return nil, &net.OpError{Op: "accept", Net: "pipe", Err: net.ErrClosed}
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

// ---------------------------------------------------------------------------
// setupPipePair creates a server handling a single pipe connection and returns
// the client-side conn and the message router (for registering mock objects).
// ---------------------------------------------------------------------------

func setupPipePair(t *testing.T, router *cip.MessageRouter, opts ...ServerOption) (*Server, net.Conn) {
	t.Helper()

	serverConn, clientConn := net.Pipe()

	allOpts := append([]ServerOption{WithServerConn(serverConn)}, opts...)
	srv := NewServer(router, allOpts...)
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("server start: %v", err)
	}

	t.Cleanup(func() {
		clientConn.Close()
		srv.Stop()
	})

	return srv, clientConn
}

// ---------------------------------------------------------------------------
// Helpers for raw EIP communication over a conn
// ---------------------------------------------------------------------------

// sendEIPPacket writes a raw EIP header + data to the conn.
func sendEIPPacket(t *testing.T, conn net.Conn, cmd eip.Command, sessionHandle uint32, senderCtx [8]byte, data []byte) {
	t.Helper()
	header := make([]byte, eip.HeaderSize)
	binary.LittleEndian.PutUint16(header[0:], uint16(cmd))
	binary.LittleEndian.PutUint16(header[2:], uint16(len(data)))
	binary.LittleEndian.PutUint32(header[4:], sessionHandle)
	binary.LittleEndian.PutUint32(header[8:], 0)
	copy(header[12:20], senderCtx[:])
	binary.LittleEndian.PutUint32(header[20:], 0)

	if _, err := conn.Write(header); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if len(data) > 0 {
		if _, err := conn.Write(data); err != nil {
			t.Fatalf("write data: %v", err)
		}
	}
}

// recvEIPPacket reads a raw EIP header + data from the conn.
func recvEIPPacket(t *testing.T, conn net.Conn) (eip.Command, uint32, uint32, [8]byte, []byte) {
	t.Helper()
	headerBuf := make([]byte, eip.HeaderSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		t.Fatalf("read header: %v", err)
	}
	cmd := eip.Command(binary.LittleEndian.Uint16(headerBuf[0:2]))
	dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
	session := binary.LittleEndian.Uint32(headerBuf[4:8])
	status := binary.LittleEndian.Uint32(headerBuf[8:12])
	var senderCtx [8]byte
	copy(senderCtx[:], headerBuf[12:20])

	var data []byte
	if dataLen > 0 {
		data = make([]byte, dataLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			t.Fatalf("read data: %v", err)
		}
	}
	return cmd, session, status, senderCtx, data
}

// registerSession sends RegisterSession and returns the session handle.
func registerSession(t *testing.T, conn net.Conn) uint32 {
	t.Helper()
	regData := make([]byte, 4)
	binary.LittleEndian.PutUint16(regData[0:], 1) // Protocol version
	binary.LittleEndian.PutUint16(regData[2:], 0) // Options

	sendEIPPacket(t, conn, eip.CommandRegisterSession, 0, [8]byte{}, regData)

	cmd, session, status, _, _ := recvEIPPacket(t, conn)
	if cmd != eip.CommandRegisterSession {
		t.Fatalf("expected RegisterSession response, got %s", cmd)
	}
	if status != 0 {
		t.Fatalf("register session failed with status 0x%08X", status)
	}
	if session == 0 {
		t.Fatal("session handle should not be 0")
	}
	return session
}

// sendRRDataRequest sends a CIP request wrapped in SendRRData.
func sendRRDataRequest(t *testing.T, conn net.Conn, sessionHandle uint32, mrReq *cip.MessageRouterRequest) *cip.MessageRouterResponse {
	t.Helper()
	mrReqBytes, err := mrReq.Encode()
	if err != nil {
		t.Fatalf("encode MR request: %v", err)
	}

	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
	)
	cpfData, err := cpf.Encode()
	if err != nil {
		t.Fatalf("encode CPF: %v", err)
	}

	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	sendEIPPacket(t, conn, eip.CommandSendRRData, sessionHandle, [8]byte{}, rrData)

	cmd, _, status, _, respData := recvEIPPacket(t, conn)
	if cmd != eip.CommandSendRRData {
		t.Fatalf("expected SendRRData response, got %s", cmd)
	}
	if status != 0 {
		t.Fatalf("SendRRData failed at encapsulation level: 0x%08X", status)
	}

	// Parse CPF from response
	if len(respData) < 6 {
		t.Fatal("response data too short")
	}
	respCPF, err := eip.DecodeCommonPacketFormat(respData[6:])
	if err != nil {
		t.Fatalf("decode response CPF: %v", err)
	}

	item := respCPF.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		t.Fatal("no unconnected message item in response")
	}

	mrResp, err := cip.DecodeMessageRouterResponse(item.Data)
	if err != nil {
		t.Fatalf("decode MR response: %v", err)
	}
	return mrResp
}

// ---------------------------------------------------------------------------
// mockObject implements cip.Object for testing
// ---------------------------------------------------------------------------

type mockObject struct {
	handleFunc func(service cip.USINT, path cip.Path, data []byte) ([]byte, error)
}

func (m *mockObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	if m.handleFunc != nil {
		return m.handleFunc(service, path, data)
	}
	return []byte{0x01, 0x02, 0x03, 0x04}, nil
}

// ===========================================================================
// Test 1: EIP Session Register/Unregister
// ===========================================================================

func TestEIPSessionRegisterUnregister(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	srv := NewServer(router)
	done := make(chan struct{})
	go func() {
		srv.HandleConn(serverConn)
		close(done)
	}()

	// Register
	session := registerSession(t, clientConn)
	if session == 0 {
		t.Fatal("session handle must not be 0")
	}

	// Unregister
	sendEIPPacket(t, clientConn, eip.CommandUnregisterSession, session, [8]byte{}, nil)

	// Connection should close
	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Error("server should have closed connection after UnregisterSession")
	}
}

// ===========================================================================
// Test 2: CIP ReadTag
// ===========================================================================

func TestCIPReadTag(t *testing.T) {
	router := cip.NewMessageRouter()

	// Register a mock object that handles ReadTag (0x4C)
	expectedData := []byte{0xC4, 0x00, 0x2A, 0x00, 0x00, 0x00} // TypeDINT + value 42
	mock := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			if service != cip.ServiceReadTag {
				return nil, cip.Error{Status: cip.StatusServiceNotSupported}
			}
			return expectedData, nil
		},
	}
	// Register under a custom class ID that the symbolic path routes to.
	// Since the server uses the Message Router which routes by class ID,
	// and ReadTag paths use symbolic segments, we'll use a special mock approach.
	// Instead, register it as the Assembly class for simplicity in this test,
	// but build a class-routed path.
	router.RegisterObject(0x04, mock)

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	// Build a ReadTag request with Class/Instance path (for test simplicity)
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceReadTag,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}), // Class 4, Instance 1
		RequestData: []byte{0x01, 0x00},                        // 1 element
	}

	mrResp := sendRRDataRequest(t, clientConn, session, mrReq)
	if !mrResp.IsSuccess() {
		t.Fatalf("ReadTag failed with status 0x%02X", mrResp.GeneralStatus)
	}
	if !bytes.Equal(mrResp.ResponseData, expectedData) {
		t.Fatalf("ReadTag response data = %X, want %X", mrResp.ResponseData, expectedData)
	}
}

// ===========================================================================
// Test 3: CIP WriteTag
// ===========================================================================

func TestCIPWriteTag(t *testing.T) {
	router := cip.NewMessageRouter()

	var writtenData []byte
	mock := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			if service != cip.ServiceWriteTag {
				return nil, cip.Error{Status: cip.StatusServiceNotSupported}
			}
			writtenData = make([]byte, len(data))
			copy(writtenData, data)
			return nil, nil // WriteTag response has no data on success
		},
	}
	router.RegisterObject(0x04, mock)

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	writePayload := []byte{0xC4, 0x00, 0x01, 0x00, 0x64, 0x00, 0x00, 0x00} // TypeDINT, 1 elem, value 100
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceWriteTag,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
		RequestData: writePayload,
	}

	mrResp := sendRRDataRequest(t, clientConn, session, mrReq)
	if !mrResp.IsSuccess() {
		t.Fatalf("WriteTag failed with status 0x%02X", mrResp.GeneralStatus)
	}
	if !bytes.Equal(writtenData, writePayload) {
		t.Fatalf("written data = %X, want %X", writtenData, writePayload)
	}
}

// ===========================================================================
// Test 4: CIP encoding round-trip
// ===========================================================================

func TestCIPEncodingRoundTrip(t *testing.T) {
	original := &cip.MessageRouterRequest{
		Service:     cip.ServiceReadTag,
		RequestPath: cip.Path([]byte{0x91, 0x05, 0x4D, 0x79, 0x54, 0x61, 0x67, 0x00}), // symbolic "MyTag" + pad
		RequestData: []byte{0x01, 0x00},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode manually
	if len(encoded) < 2 {
		t.Fatal("encoded too short")
	}
	service := cip.USINT(encoded[0])
	pathSizeWords := encoded[1]
	pathBytes := encoded[2 : 2+int(pathSizeWords)*2]
	reqData := encoded[2+int(pathSizeWords)*2:]

	if service != original.Service {
		t.Fatalf("service = 0x%02X, want 0x%02X", service, original.Service)
	}
	if !bytes.Equal(pathBytes, original.RequestPath.Bytes()) {
		t.Fatalf("path = %X, want %X", pathBytes, original.RequestPath.Bytes())
	}
	if !bytes.Equal(reqData, original.RequestData) {
		t.Fatalf("request data = %X, want %X", reqData, original.RequestData)
	}
}

// ===========================================================================
// Test 5: EPATH building
// ===========================================================================

func TestEPATHBuilding(t *testing.T) {
	t.Run("SymbolicSegment", func(t *testing.T) {
		p := cip.NewPath()
		p.AddSymbolicSegment("Test")
		expected := []byte{0x91, 0x04, 'T', 'e', 's', 't'} // len 4, no pad needed
		if !bytes.Equal(p.Bytes(), expected) {
			t.Fatalf("symbolic path = %X, want %X", p.Bytes(), expected)
		}
	})

	t.Run("SymbolicSegmentOddPad", func(t *testing.T) {
		p := cip.NewPath()
		p.AddSymbolicSegment("Tag")
		expected := []byte{0x91, 0x03, 'T', 'a', 'g', 0x00} // odd length -> pad
		if !bytes.Equal(p.Bytes(), expected) {
			t.Fatalf("symbolic path = %X, want %X", p.Bytes(), expected)
		}
	})

	t.Run("ClassInstanceAttribute", func(t *testing.T) {
		p := cip.BuildPath(0x04, 0x01, 0x03)
		expected := []byte{
			0x20, 0x04, // Class 4 (8-bit)
			0x24, 0x01, // Instance 1 (8-bit)
			0x30, 0x03, // Attribute 3 (8-bit)
		}
		if !bytes.Equal(p.Bytes(), expected) {
			t.Fatalf("class/instance/attr path = %X, want %X", p.Bytes(), expected)
		}
	})

	t.Run("16BitClass", func(t *testing.T) {
		p := cip.NewPath()
		p.AddClass(0x0100)
		expected := []byte{0x21, 0x00, 0x00, 0x01} // 16-bit class
		if !bytes.Equal(p.Bytes(), expected) {
			t.Fatalf("16-bit class path = %X, want %X", p.Bytes(), expected)
		}
	})

	t.Run("LenWords", func(t *testing.T) {
		p := cip.BuildPath(0x04, 0x01, 0)
		// 4 bytes -> 2 words
		if p.LenWords() != 2 {
			t.Fatalf("LenWords = %d, want 2", p.LenWords())
		}
	})
}

// ===========================================================================
// Test 6: Timer/Counter decode
// ===========================================================================

func TestTimerDecode(t *testing.T) {
	// Build 14-byte timer data:
	// Offset 0-1: Reserved (0x0000)
	// Offset 2-5: Status (EN=1, TT=1, DN=0)
	// Offset 6-9: PRE = 5000
	// Offset 10-13: ACC = 2500
	data := make([]byte, 14)
	binary.LittleEndian.PutUint16(data[0:], 0)
	statusBits := uint32(1<<cip.TimerStatusEN | 1<<cip.TimerStatusTT)
	binary.LittleEndian.PutUint32(data[2:], statusBits)
	binary.LittleEndian.PutUint32(data[6:], 5000)
	binary.LittleEndian.PutUint32(data[10:], 2500)

	timer, err := cip.DecodeTimer(data)
	if err != nil {
		t.Fatalf("DecodeTimer: %v", err)
	}
	if timer.PRE != 5000 {
		t.Errorf("PRE = %d, want 5000", timer.PRE)
	}
	if timer.ACC != 2500 {
		t.Errorf("ACC = %d, want 2500", timer.ACC)
	}
	if !timer.EN {
		t.Error("EN should be true")
	}
	if !timer.TT {
		t.Error("TT should be true")
	}
	if timer.DN {
		t.Error("DN should be false")
	}
}

func TestCounterDecode(t *testing.T) {
	data := make([]byte, 14)
	binary.LittleEndian.PutUint16(data[0:], 0)
	statusBits := uint32(1<<cip.CounterStatusCU | 1<<cip.CounterStatusDN)
	binary.LittleEndian.PutUint32(data[2:], statusBits)
	binary.LittleEndian.PutUint32(data[6:], 100) // PRE
	binary.LittleEndian.PutUint32(data[10:], 100) // ACC

	counter, err := cip.DecodeCounter(data)
	if err != nil {
		t.Fatalf("DecodeCounter: %v", err)
	}
	if counter.PRE != 100 {
		t.Errorf("PRE = %d, want 100", counter.PRE)
	}
	if counter.ACC != 100 {
		t.Errorf("ACC = %d, want 100", counter.ACC)
	}
	if !counter.CU {
		t.Error("CU should be true")
	}
	if !counter.DN {
		t.Error("DN should be true")
	}
	if counter.CD {
		t.Error("CD should be false")
	}
	if counter.OV {
		t.Error("OV should be false")
	}
}

func TestTimerMarshalRoundTrip(t *testing.T) {
	original := &cip.Timer{PRE: 3000, ACC: 1500, EN: true, TT: false, DN: true}
	data, err := original.MarshalCIP()
	if err != nil {
		t.Fatalf("MarshalCIP: %v", err)
	}

	decoded, err := cip.DecodeTimer(data)
	if err != nil {
		t.Fatalf("DecodeTimer: %v", err)
	}
	if decoded.PRE != original.PRE || decoded.ACC != original.ACC {
		t.Fatalf("PRE/ACC mismatch: %v vs %v", decoded, original)
	}
	if decoded.EN != original.EN || decoded.TT != original.TT || decoded.DN != original.DN {
		t.Fatalf("status mismatch: %v vs %v", decoded, original)
	}
}

// ===========================================================================
// Test 7: Assembly Object
// ===========================================================================

func TestAssemblyObject(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(1, make([]byte, 4))

	// Get Attribute Single (Data, attr 3)
	data, err := ao.GetAttributeSingle(1, 3)
	if err != nil {
		t.Fatalf("GetAttributeSingle: %v", err)
	}
	if len(data) != 4 {
		t.Fatalf("data len = %d, want 4", len(data))
	}

	// Set Attribute Single
	err = ao.SetAttributeSingle(1, 3, []byte{0xAA, 0xBB, 0xCC, 0xDD})
	if err != nil {
		t.Fatalf("SetAttributeSingle: %v", err)
	}

	// Verify data
	data, err = ao.GetAttributeSingle(1, 3)
	if err != nil {
		t.Fatalf("GetAttributeSingle after set: %v", err)
	}
	expected := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	if !bytes.Equal(data, expected) {
		t.Fatalf("data = %X, want %X", data, expected)
	}

	// Test non-existent instance
	_, err = ao.GetAttributeSingle(99, 3)
	if err == nil {
		t.Fatal("expected error for non-existent instance")
	}

	// Test unsupported attribute
	_, err = ao.GetAttributeSingle(1, 99)
	if err == nil {
		t.Fatal("expected error for unsupported attribute")
	}

	// Test wrong size write
	err = ao.SetAttributeSingle(1, 3, []byte{0x01, 0x02}) // too short
	if err == nil {
		t.Fatal("expected error for wrong size write")
	}
}

func TestAssemblyHandleRequest(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(1, []byte{0x11, 0x22, 0x33, 0x44})

	// Get via HandleRequest
	path := cip.Path([]byte{0x24, 0x01, 0x30, 0x03}) // Instance 1, Attribute 3
	data, err := ao.HandleRequest(cip.ServiceGetAttributeSingle, path, nil)
	if err != nil {
		t.Fatalf("HandleRequest get: %v", err)
	}
	expected := []byte{0x11, 0x22, 0x33, 0x44}
	if !bytes.Equal(data, expected) {
		t.Fatalf("data = %X, want %X", data, expected)
	}
}

// ===========================================================================
// Test 8: Connection Manager Forward_Open/Forward_Close
// ===========================================================================

func TestConnectionManager(t *testing.T) {
	router := cip.NewMessageRouter()
	cm := connmgr.NewConnectionManager()
	router.RegisterObject(cip.ClassConnectionMgr, cm)

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	// Build Forward_Open request data
	foData := new(bytes.Buffer)
	binary.Write(foData, binary.LittleEndian, cip.BYTE(0x0A))   // PriorityTimeTick
	binary.Write(foData, binary.LittleEndian, cip.USINT(0x05))  // TimeoutTicks
	binary.Write(foData, binary.LittleEndian, cip.UDINT(0x1234)) // OTConnectionID
	binary.Write(foData, binary.LittleEndian, cip.UDINT(0x0000)) // TOConnectionID (server assigns)
	binary.Write(foData, binary.LittleEndian, cip.UINT(0x0001))  // ConnectionSerialNumber
	binary.Write(foData, binary.LittleEndian, cip.UINT(0x1234))  // VendorID
	binary.Write(foData, binary.LittleEndian, cip.UDINT(0xABCD)) // OriginatorSerialNumber
	binary.Write(foData, binary.LittleEndian, cip.USINT(0x00))  // ConnectionTimeoutMultiplier
	binary.Write(foData, binary.LittleEndian, [3]cip.BYTE{})    // Reserved
	binary.Write(foData, binary.LittleEndian, cip.UDINT(10000)) // OTRPI (10ms in us)
	binary.Write(foData, binary.LittleEndian, cip.WORD(0x43F4)) // OTNetworkConnectionParams
	binary.Write(foData, binary.LittleEndian, cip.UDINT(10000)) // TORPI
	binary.Write(foData, binary.LittleEndian, cip.WORD(0x43F4)) // TONetworkConnectionParams
	binary.Write(foData, binary.LittleEndian, cip.BYTE(0x01))   // TransportTypeTrigger
	binary.Write(foData, binary.LittleEndian, cip.USINT(0x02))  // ConnectionPathSize (2 words)
	foData.Write([]byte{0x20, 0x04, 0x24, 0x01})                // Connection path

	// Build MR request targeting Connection Manager (class 0x06)
	foPath := cip.NewPath()
	foPath.AddClass(cip.ClassConnectionMgr)
	foPath.AddInstance(1)

	mrReq := &cip.MessageRouterRequest{
		Service:     cip.USINT(connmgr.ServiceForwardOpen),
		RequestPath: foPath,
		RequestData: foData.Bytes(),
	}

	mrResp := sendRRDataRequest(t, clientConn, session, mrReq)
	if !mrResp.IsSuccess() {
		t.Fatalf("ForwardOpen failed with status 0x%02X", mrResp.GeneralStatus)
	}

	// Parse ForwardOpen response
	if len(mrResp.ResponseData) < 26 {
		t.Fatalf("ForwardOpen response too short: %d bytes", len(mrResp.ResponseData))
	}
	otConnID := binary.LittleEndian.Uint32(mrResp.ResponseData[0:4])
	toConnID := binary.LittleEndian.Uint32(mrResp.ResponseData[4:8])
	if otConnID != 0x1234 {
		t.Errorf("OT Connection ID = 0x%08X, want 0x00001234", otConnID)
	}
	if toConnID == 0 {
		t.Error("TO Connection ID should not be 0")
	}

	// Now send Forward_Close
	fcData := new(bytes.Buffer)
	binary.Write(fcData, binary.LittleEndian, cip.BYTE(0x0A))   // PriorityTimeTick
	binary.Write(fcData, binary.LittleEndian, cip.USINT(0x05))  // TimeoutTicks
	binary.Write(fcData, binary.LittleEndian, cip.UINT(0x0001))  // ConnectionSerialNumber
	binary.Write(fcData, binary.LittleEndian, cip.UINT(0x1234))  // VendorID
	binary.Write(fcData, binary.LittleEndian, cip.UDINT(0xABCD)) // OriginatorSerialNumber
	binary.Write(fcData, binary.LittleEndian, cip.USINT(0x02))  // ConnectionPathSize (2 words)
	binary.Write(fcData, binary.LittleEndian, cip.USINT(0x00))  // Reserved
	fcData.Write([]byte{0x20, 0x04, 0x24, 0x01})                // Connection path

	fcReq := &cip.MessageRouterRequest{
		Service:     cip.USINT(connmgr.ServiceForwardClose),
		RequestPath: foPath,
		RequestData: fcData.Bytes(),
	}

	fcResp := sendRRDataRequest(t, clientConn, session, fcReq)
	if !fcResp.IsSuccess() {
		t.Fatalf("ForwardClose failed with status 0x%02X", fcResp.GeneralStatus)
	}
}

// ===========================================================================
// Test 9: CPF encode/decode round-trip
// ===========================================================================

func TestCPFRoundTrip(t *testing.T) {
	original := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, []byte{0xDE, 0xAD, 0xBE, 0xEF}),
	)

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := eip.DecodeCommonPacketFormat(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", decoded.ItemCount)
	}

	// Check Null Address
	if decoded.Items[0].TypeID != eip.ItemIDNullAddress {
		t.Errorf("item 0 type = 0x%04X, want 0x%04X", decoded.Items[0].TypeID, eip.ItemIDNullAddress)
	}
	if decoded.Items[0].Length != 0 {
		t.Errorf("item 0 length = %d, want 0", decoded.Items[0].Length)
	}

	// Check Unconnected Message
	if decoded.Items[1].TypeID != eip.ItemIDUnconnectedMessage {
		t.Errorf("item 1 type = 0x%04X, want 0x%04X", decoded.Items[1].TypeID, eip.ItemIDUnconnectedMessage)
	}
	expectedData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(decoded.Items[1].Data, expectedData) {
		t.Errorf("item 1 data = %X, want %X", decoded.Items[1].Data, expectedData)
	}
}

func TestCPFConnectedRoundTrip(t *testing.T) {
	connID := make([]byte, 4)
	binary.LittleEndian.PutUint32(connID, 0xCAFEBABE)

	seqAndData := make([]byte, 6)
	binary.LittleEndian.PutUint16(seqAndData[0:], 42) // seq count
	copy(seqAndData[2:], []byte{0x01, 0x02, 0x03, 0x04})

	original := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDConnectedAddress, connID),
		eip.NewCPFItem(eip.ItemIDConnectedData, seqAndData),
	)

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := eip.DecodeCommonPacketFormat(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.ItemCount != 2 {
		t.Fatalf("item count = %d, want 2", decoded.ItemCount)
	}

	addrItem := decoded.FindItemByType(eip.ItemIDConnectedAddress)
	if addrItem == nil {
		t.Fatal("missing connected address item")
	}
	gotConnID := binary.LittleEndian.Uint32(addrItem.Data)
	if gotConnID != 0xCAFEBABE {
		t.Errorf("conn ID = 0x%08X, want 0xCAFEBABE", gotConnID)
	}

	dataItem := decoded.FindItemByType(eip.ItemIDConnectedData)
	if dataItem == nil {
		t.Fatal("missing connected data item")
	}
	gotSeq := binary.LittleEndian.Uint16(dataItem.Data[0:2])
	if gotSeq != 42 {
		t.Errorf("seq count = %d, want 42", gotSeq)
	}
}

// ===========================================================================
// Test 10: Full client-server integration via net.Pipe
// ===========================================================================

func TestFullIntegration(t *testing.T) {
	// Set up the server side with a mock tag reader object
	router := cip.NewMessageRouter()

	// Mock object that responds to ReadTag with a DINT value = 42
	tagObj := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			switch service {
			case cip.ServiceReadTag:
				// Return TypeDINT (0x00C4) + value 42
				resp := make([]byte, 6)
				binary.LittleEndian.PutUint16(resp[0:], uint16(cip.TypeDINT))
				binary.LittleEndian.PutUint32(resp[2:], 42)
				return resp, nil
			case cip.ServiceWriteTag:
				return nil, nil
			default:
				return nil, cip.Error{Status: cip.StatusServiceNotSupported}
			}
		},
	}
	router.RegisterObject(0x04, tagObj) // Assembly class as stand-in

	// Create pipe pair
	serverConn, clientConn := net.Pipe()

	// Start server handling on one end
	srv := NewServer(router)
	serverDone := make(chan struct{})
	go func() {
		srv.HandleConn(serverConn)
		close(serverDone)
	}()

	// Create client-side TCPConn with injected conn
	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}

	// Create Session and Register
	sess := NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Send ReadTag CIP request
	readPath := cip.NewPath()
	readPath.AddClass(0x04)
	readPath.AddInstance(1)

	readReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceReadTag,
		RequestPath: readPath,
		RequestData: []byte{0x01, 0x00}, // 1 element
	}

	resp, err := sess.SendCIPRequest(ctx, readReq)
	if err != nil {
		t.Fatalf("SendCIPRequest: %v", err)
	}
	if !resp.IsSuccess() {
		t.Fatalf("CIP response error: 0x%02X", resp.GeneralStatus)
	}

	// Verify response data
	if len(resp.ResponseData) < 6 {
		t.Fatalf("response data too short: %d bytes", len(resp.ResponseData))
	}
	typeCode := binary.LittleEndian.Uint16(resp.ResponseData[0:2])
	if cip.DataType(typeCode) != cip.TypeDINT {
		t.Errorf("type code = 0x%04X, want 0x%04X (DINT)", typeCode, cip.TypeDINT)
	}
	value := binary.LittleEndian.Uint32(resp.ResponseData[2:6])
	if value != 42 {
		t.Errorf("value = %d, want 42", value)
	}

	// Clean up
	sess.Unregister(ctx)
	tc.Close()

	select {
	case <-serverDone:
		// expected
	case <-time.After(2 * time.Second):
		t.Error("server should have closed after unregister")
	}
}

// ===========================================================================
// Additional edge-case tests
// ===========================================================================

func TestSenderContextPreserved(t *testing.T) {
	router := cip.NewMessageRouter()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	ctx := [8]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	regData := make([]byte, 4)
	binary.LittleEndian.PutUint16(regData[0:], 1)
	binary.LittleEndian.PutUint16(regData[2:], 0)

	sendEIPPacket(t, clientConn, eip.CommandRegisterSession, 0, ctx, regData)
	_, _, _, gotCtx, _ := recvEIPPacket(t, clientConn)

	if gotCtx != ctx {
		t.Fatalf("sender context = %X, want %X", gotCtx, ctx)
	}
}

func TestUnsupportedCommand(t *testing.T) {
	router := cip.NewMessageRouter()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	sendEIPPacket(t, clientConn, eip.Command(0xFFFF), 0, [8]byte{}, nil)
	_, _, status, _, _ := recvEIPPacket(t, clientConn)

	if status != 0x0001 {
		t.Fatalf("status = 0x%08X, want 0x00000001", status)
	}
}

func TestMaxPacketSizeExceeded(t *testing.T) {
	router := cip.NewMessageRouter()

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		NewServer(router).HandleConn(serverConn)
		close(done)
	}()

	// Send header claiming 5000 bytes of data
	header := make([]byte, eip.HeaderSize)
	binary.LittleEndian.PutUint16(header[0:], uint16(eip.CommandSendRRData))
	binary.LittleEndian.PutUint16(header[2:], 5000)
	clientConn.Write(header)

	select {
	case <-done:
		// expected - server closes due to oversized packet
	case <-time.After(2 * time.Second):
		t.Error("server should close connection for oversized packet")
	}
}

// ===========================================================================
// Session Edge Cases (from OpENer / enip-stack-detector insights)
// ===========================================================================

// TestDoubleRegisterSession verifies behavior when a client sends two
// RegisterSession commands on the same connection.
func TestDoubleRegisterSession(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	srv := NewServer(router)
	go srv.HandleConn(serverConn)

	session1 := registerSession(t, clientConn)
	if session1 == 0 {
		t.Fatal("first session should not be 0")
	}

	// Register again on the same connection
	session2 := registerSession(t, clientConn)
	// Server should still return a valid session (may be same or different)
	if session2 == 0 {
		t.Fatal("second session should not be 0")
	}
}

// TestSendRRDataShortPayload verifies the server handles a SendRRData
// with data_length < 6 (missing interface handle and timeout).
func TestSendRRDataShortPayload(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	session := registerSession(t, clientConn)

	// Send RRData with only 2 bytes of payload (short)
	sendEIPPacket(t, clientConn, eip.CommandSendRRData, session, [8]byte{}, []byte{0x00, 0x00})

	// Server should respond with error status
	cmd, _, status, _, _ := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandSendRRData {
		t.Fatalf("expected SendRRData response, got %s", cmd)
	}
	if status == 0 {
		t.Error("expected non-zero status for short SendRRData payload")
	}
}

// TestNopCommand verifies NOP command handling (should return error from our server).
func TestNopCommand(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	session := registerSession(t, clientConn)

	// Send NOP
	sendEIPPacket(t, clientConn, eip.CommandNop, session, [8]byte{}, nil)

	// Server should respond with error (unsupported command)
	cmd, _, status, _, _ := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandNop {
		t.Fatalf("expected Nop response, got %s", cmd)
	}
	if status == 0 {
		t.Error("expected non-zero status for NOP command")
	}
}

// TestSendRRDataInvalidCPF verifies the server handles malformed CPF data.
func TestSendRRDataInvalidCPF(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	session := registerSession(t, clientConn)

	// 6 bytes of interface handle + timeout, then garbage CPF
	payload := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0xFF}
	sendEIPPacket(t, clientConn, eip.CommandSendRRData, session, [8]byte{}, payload)

	cmd, _, status, _, _ := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandSendRRData {
		t.Fatalf("expected SendRRData response, got %s", cmd)
	}
	if status == 0 {
		t.Error("expected non-zero status for invalid CPF")
	}
}

// TestMultipleSenderContexts verifies that different sender contexts are
// echoed correctly across multiple sequential requests.
func TestMultipleSenderContexts(t *testing.T) {
	router := cip.NewMessageRouter()
	router.RegisterObject(0x04, &mockObject{})

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	contexts := [][8]byte{
		{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	for i, ctx := range contexts {
		mrReq := &cip.MessageRouterRequest{
			Service:     cip.ServiceGetAttributeSingle,
			RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
		}
		mrReqBytes, _ := mrReq.Encode()
		cpf := eip.NewCommonPacketFormat(
			eip.NewCPFItem(eip.ItemIDNullAddress, nil),
			eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
		)
		cpfData, _ := cpf.Encode()
		rrData := make([]byte, 6+len(cpfData))
		copy(rrData[6:], cpfData)

		sendEIPPacket(t, clientConn, eip.CommandSendRRData, session, ctx, rrData)
		_, _, _, respCtx, _ := recvEIPPacket(t, clientConn)
		if respCtx != ctx {
			t.Errorf("request %d: sender context = %X, want %X", i, respCtx, ctx)
		}
	}
}

// TestListServicesCommand verifies the server returns a valid ListServices response.
func TestListServicesCommand(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router).HandleConn(serverConn)

	_ = registerSession(t, clientConn)

	sendEIPPacket(t, clientConn, eip.CommandListServices, 0, [8]byte{}, nil)
	cmd, _, status, _, data := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandListServices {
		t.Fatalf("expected ListServices response, got %s", cmd)
	}
	if status != 0 {
		t.Fatalf("ListServices failed: status 0x%08X", status)
	}

	items, err := eip.DecodeListServicesResponse(data)
	if err != nil {
		t.Fatalf("decode ListServices: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 service item, got %d", len(items))
	}
	if items[0].Name != "Communications" {
		t.Errorf("service name = %q, want %q", items[0].Name, "Communications")
	}
	if items[0].CapabilityFlags&0x0020 == 0 {
		t.Error("expected CIP encapsulation capability flag (0x0020)")
	}
}

// TestListIdentityCommand verifies the server returns a valid ListIdentity response.
func TestListIdentityCommand(t *testing.T) {
	router := cip.NewMessageRouter()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	go NewServer(router, WithIdentity(eip.ListIdentityItem{
		TypeID:      eip.ItemIDListIdentity,
		VendorID:    42,
		ProductName: "TestDevice",
	})).HandleConn(serverConn)

	_ = registerSession(t, clientConn)

	sendEIPPacket(t, clientConn, eip.CommandListIdentity, 0, [8]byte{}, nil)
	cmd, _, status, _, data := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandListIdentity {
		t.Fatalf("expected ListIdentity response, got %s", cmd)
	}
	if status != 0 {
		t.Fatalf("ListIdentity failed: status 0x%08X", status)
	}

	items, err := eip.DecodeListIdentityResponse(data)
	if err != nil {
		t.Fatalf("decode ListIdentity: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 identity item, got %d", len(items))
	}
	if items[0].VendorID != 42 {
		t.Errorf("VendorID = %d, want 42", items[0].VendorID)
	}
	if items[0].ProductName != "TestDevice" {
		t.Errorf("ProductName = %q, want %q", items[0].ProductName, "TestDevice")
	}
}

// ===========================================================================
// Connected Messaging (SendUnitData)
// ===========================================================================

func TestSendUnitDataRoundTrip(t *testing.T) {
	router := cip.NewMessageRouter()
	expectedResp := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	router.RegisterObject(0x04, &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			return expectedResp, nil
		},
	})

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	// Build SendUnitData request
	connID := make([]byte, 4)
	binary.LittleEndian.PutUint32(connID, 0x12345678)

	// CIP request preceded by 2-byte sequence number
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}
	mrReqBytes, _ := mrReq.Encode()
	seqData := make([]byte, 2)
	binary.LittleEndian.PutUint16(seqData, 1)
	connData := append(seqData, mrReqBytes...)

	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDConnectionBased, connID),
		eip.NewCPFItem(eip.ItemIDConnectedTransport, connData),
	)
	cpfData, _ := cpf.Encode()

	// Prepend interface handle (4 bytes) + timeout (2 bytes)
	payload := make([]byte, 6+len(cpfData))
	copy(payload[6:], cpfData)

	sendEIPPacket(t, clientConn, eip.CommandSendUnitData, session, [8]byte{}, payload)

	cmd, _, status, _, respData := recvEIPPacket(t, clientConn)
	if cmd != eip.CommandSendUnitData {
		t.Fatalf("expected SendUnitData response, got %s", cmd)
	}
	if status != 0 {
		t.Fatalf("SendUnitData failed: status 0x%08X", status)
	}

	// Parse response CPF
	if len(respData) < 6 {
		t.Fatal("response too short")
	}
	respCPF, err := eip.DecodeCommonPacketFormat(respData[6:])
	if err != nil {
		t.Fatalf("decode CPF: %v", err)
	}

	dataItem := respCPF.FindItemByType(eip.ItemIDConnectedData)
	if dataItem == nil {
		t.Fatal("missing connected data item in response")
	}
	// Skip sequence number (2 bytes), then MR response header (4 bytes), then data
	if len(dataItem.Data) < 6 {
		t.Fatalf("connected data too short: %d bytes", len(dataItem.Data))
	}
	respSeq := binary.LittleEndian.Uint16(dataItem.Data[0:2])
	if respSeq != 1 {
		t.Errorf("response sequence = %d, want 1", respSeq)
	}
}

// ===========================================================================
// Session Validation
// ===========================================================================

func TestSessionHandleUnique(t *testing.T) {
	router := cip.NewMessageRouter()
	srv := NewServer(router)

	// First connection
	s1, c1 := net.Pipe()
	go srv.HandleConn(s1)
	defer c1.Close()

	// Second connection
	s2, c2 := net.Pipe()
	go srv.HandleConn(s2)
	defer c2.Close()

	h1 := registerSession(t, c1)
	h2 := registerSession(t, c2)

	if h1 == h2 {
		t.Errorf("session handles should be unique: both got 0x%08X", h1)
	}
	if h1 == 0 || h2 == 0 {
		t.Error("session handles must not be 0")
	}
}

func TestSessionHandleValidation(t *testing.T) {
	router := cip.NewMessageRouter()
	router.RegisterObject(0x04, &mockObject{})

	_, clientConn := setupPipePair(t, router)
	session := registerSession(t, clientConn)

	// Send SendRRData with wrong session handle
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}
	mrReqBytes, _ := mrReq.Encode()
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
	)
	cpfData, _ := cpf.Encode()
	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	// Use wrong session handle
	wrongSession := session + 999
	sendEIPPacket(t, clientConn, eip.CommandSendRRData, wrongSession, [8]byte{}, rrData)
	_, _, status, _, _ := recvEIPPacket(t, clientConn)

	if status != eip.StatusInvalidSessionHandle {
		t.Fatalf("expected StatusInvalidSessionHandle (0x%08X), got 0x%08X",
			eip.StatusInvalidSessionHandle, status)
	}
}

func TestSessionHandleZeroRejected(t *testing.T) {
	router := cip.NewMessageRouter()
	router.RegisterObject(0x04, &mockObject{})

	_, clientConn := setupPipePair(t, router)
	_ = registerSession(t, clientConn)

	// Send SendRRData with session handle 0 (not registered)
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}
	mrReqBytes, _ := mrReq.Encode()
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
	)
	cpfData, _ := cpf.Encode()
	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	sendEIPPacket(t, clientConn, eip.CommandSendRRData, 0, [8]byte{}, rrData)
	_, _, status, _, _ := recvEIPPacket(t, clientConn)

	if status != eip.StatusInvalidSessionHandle {
		t.Fatalf("expected StatusInvalidSessionHandle, got 0x%08X", status)
	}
}

// ===========================================================================
// Client Tracking
// ===========================================================================

func TestConnectedClients(t *testing.T) {
	router := cip.NewMessageRouter()

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router, WithServerConn(serverConn))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Give HandleConn a moment to register the client
	time.Sleep(50 * time.Millisecond)

	clients := srv.ConnectedClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}

	// Register session and verify it's reflected
	session := registerSession(t, clientConn)
	time.Sleep(20 * time.Millisecond)

	clients = srv.ConnectedClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].SessionHandle != session {
		t.Errorf("session handle = 0x%08X, want 0x%08X", clients[0].SessionHandle, session)
	}

	// Disconnect
	clientConn.Close()
	time.Sleep(50 * time.Millisecond)

	clients = srv.ConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected 0 clients after disconnect, got %d", len(clients))
	}
}

func TestClientCallbacks(t *testing.T) {
	router := cip.NewMessageRouter()

	connectCh := make(chan ConnectedClient, 1)
	disconnectCh := make(chan ConnectedClient, 1)

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router,
		WithServerConn(serverConn),
		WithOnClientConnect(func(c ConnectedClient) {
			connectCh <- c
		}),
		WithOnClientDisconnect(func(c ConnectedClient) {
			disconnectCh <- c
		}),
	)
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case <-connectCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connect callback")
	}

	clientConn.Close()

	select {
	case <-disconnectCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect callback")
	}
}

// ===========================================================================
// MR Decode/Encode Round-Trip
// ===========================================================================

func TestDecodeMessageRouterRequest(t *testing.T) {
	original := &cip.MessageRouterRequest{
		Service:     cip.ServiceReadTag,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
		RequestData: []byte{0x01, 0x00},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := cip.DecodeMessageRouterRequest(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Service != original.Service {
		t.Errorf("service = 0x%02X, want 0x%02X", decoded.Service, original.Service)
	}
	if !bytes.Equal(decoded.RequestPath.Bytes(), original.RequestPath.Bytes()) {
		t.Errorf("path = %X, want %X", decoded.RequestPath.Bytes(), original.RequestPath.Bytes())
	}
	if !bytes.Equal(decoded.RequestData, original.RequestData) {
		t.Errorf("data = %X, want %X", decoded.RequestData, original.RequestData)
	}
}

func TestMessageRouterResponseEncode(t *testing.T) {
	original := &cip.MessageRouterResponse{
		Service:       cip.ServiceReadTag | 0x80,
		GeneralStatus: cip.StatusSuccess,
		ResponseData:  []byte{0xC4, 0x00, 0x2A, 0x00, 0x00, 0x00},
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := cip.DecodeMessageRouterResponse(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Service != original.Service {
		t.Errorf("service = 0x%02X, want 0x%02X", decoded.Service, original.Service)
	}
	if decoded.GeneralStatus != original.GeneralStatus {
		t.Errorf("status = 0x%02X, want 0x%02X", decoded.GeneralStatus, original.GeneralStatus)
	}
	if !bytes.Equal(decoded.ResponseData, original.ResponseData) {
		t.Errorf("data = %X, want %X", decoded.ResponseData, original.ResponseData)
	}
}

// ===========================================================================
// Full Client Loopback (real Client through net.Pipe to Server)
// ===========================================================================

func TestClientServerLoopback(t *testing.T) {
	router := cip.NewMessageRouter()

	// Register a tag object that handles ReadTag and WriteTag
	var storedValue int32 = 100
	tagObj := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			switch service {
			case cip.ServiceReadTag:
				resp := make([]byte, 6)
				binary.LittleEndian.PutUint16(resp[0:], uint16(cip.TypeDINT))
				binary.LittleEndian.PutUint32(resp[2:], uint32(storedValue))
				return resp, nil
			case cip.ServiceWriteTag:
				if len(data) < 8 {
					return nil, cip.Error{Status: cip.StatusNotEnoughData}
				}
				storedValue = int32(binary.LittleEndian.Uint32(data[4:8]))
				return nil, nil
			default:
				return nil, cip.Error{Status: cip.StatusServiceNotSupported}
			}
		},
	}
	router.RegisterObject(0x04, tagObj)

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router, WithServerConn(serverConn))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	// Build a real Client using the pipe connection
	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}
	t.Cleanup(func() { tc.Close() })

	sess := NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Read initial value (should be 100)
	readReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceReadTag,
		RequestPath: cip.BuildPath(0x04, 1, 0),
		RequestData: []byte{0x01, 0x00},
	}
	resp, err := sess.SendCIPRequest(ctx, readReq)
	if err != nil {
		t.Fatalf("ReadTag: %v", err)
	}
	if !resp.IsSuccess() {
		t.Fatalf("ReadTag failed: status 0x%02X", resp.GeneralStatus)
	}
	if len(resp.ResponseData) < 6 {
		t.Fatalf("ReadTag response too short: %d bytes", len(resp.ResponseData))
	}
	val := int32(binary.LittleEndian.Uint32(resp.ResponseData[2:6]))
	if val != 100 {
		t.Errorf("initial value = %d, want 100", val)
	}

	// Write a new value (42)
	writeData := make([]byte, 8)
	binary.LittleEndian.PutUint16(writeData[0:], uint16(cip.TypeDINT))
	binary.LittleEndian.PutUint16(writeData[2:], 1)
	binary.LittleEndian.PutUint32(writeData[4:], 42)

	writeReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceWriteTag,
		RequestPath: cip.BuildPath(0x04, 1, 0),
		RequestData: writeData,
	}
	resp, err = sess.SendCIPRequest(ctx, writeReq)
	if err != nil {
		t.Fatalf("WriteTag: %v", err)
	}
	if !resp.IsSuccess() {
		t.Fatalf("WriteTag failed: status 0x%02X", resp.GeneralStatus)
	}

	// Read back to verify
	resp, err = sess.SendCIPRequest(ctx, readReq)
	if err != nil {
		t.Fatalf("ReadTag after write: %v", err)
	}
	if !resp.IsSuccess() {
		t.Fatalf("ReadTag after write failed: status 0x%02X", resp.GeneralStatus)
	}
	val = int32(binary.LittleEndian.Uint32(resp.ResponseData[2:6]))
	if val != 42 {
		t.Errorf("value after write = %d, want 42", val)
	}

	// Unregister
	sess.Unregister(ctx)
}

func TestServerWithPipeListener(t *testing.T) {
	router := cip.NewMessageRouter()
	router.RegisterObject(0x04, &mockObject{})

	serverConn, clientConn := net.Pipe()
	pl := newPipeListener(serverConn)

	srv := NewServer(router, WithServerListener(pl))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		clientConn.Close()
		srv.Stop()
	})

	// Give the accept loop a moment to hand off the conn
	time.Sleep(50 * time.Millisecond)

	session := registerSession(t, clientConn)
	if session == 0 {
		t.Fatal("session handle should not be 0")
	}
}

// ---------------------------------------------------------------------------
// chanListener accepts many pipe connections via a buffered channel.
// ---------------------------------------------------------------------------

type chanListener struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
}

func newChanListener(buffer int) *chanListener {
	return &chanListener{
		ch:   make(chan net.Conn, buffer),
		done: make(chan struct{}),
	}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, &net.OpError{Op: "accept", Net: "pipe", Err: net.ErrClosed}
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return pipeAddr{} }

// ===========================================================================
// Stress Test: 1 writer + 1000 readers through loopback server
// ===========================================================================

func TestStressConcurrentClients(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numReaders = 1000
		numWrites  = 20
		writeDelay = 2 * time.Millisecond
	)

	// Shared tag value — the writer increments it, readers poll it.
	var currentValue atomic.Int32

	router := cip.NewMessageRouter()
	router.RegisterObject(0x04, &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			switch service {
			case cip.ServiceReadTag:
				v := currentValue.Load()
				resp := make([]byte, 6)
				binary.LittleEndian.PutUint16(resp[0:], uint16(cip.TypeDINT))
				binary.LittleEndian.PutUint32(resp[2:], uint32(v))
				return resp, nil
			case cip.ServiceWriteTag:
				if len(data) < 8 {
					return nil, cip.Error{Status: cip.StatusNotEnoughData}
				}
				currentValue.Store(int32(binary.LittleEndian.Uint32(data[4:8])))
				return nil, nil
			default:
				return nil, cip.Error{Status: cip.StatusServiceNotSupported}
			}
		},
	})

	// Listener that accepts many pipe connections.
	ln := newChanListener(numReaders + 1)
	srv := NewServer(router, WithServerListener(ln))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}

	tagPath := cip.BuildPath(0x04, 1, 0)
	ctx := context.Background()

	// dial creates a pipe pair, hands the server side to the listener,
	// and returns a registered Session + TCPConn.
	dial := func() (*Session, *TCPConn) {
		t.Helper()
		serverConn, clientConn := net.Pipe()
		ln.ch <- serverConn

		tc, err := NewTCPConn("", WithConn(clientConn))
		if err != nil {
			t.Fatalf("NewTCPConn: %v", err)
		}
		sess := NewSession(tc, nil)
		if err := sess.Register(ctx); err != nil {
			tc.Close()
			t.Fatalf("Register: %v", err)
		}
		return sess, tc
	}

	// Connect all clients up front (writer + readers).
	writerSess, writerTC := dial()

	type readerState struct {
		sess     *Session
		tc       *TCPConn
		lastSeen atomic.Int32
		reads    atomic.Int32
	}
	readers := make([]*readerState, numReaders)
	for i := range readers {
		sess, tc := dial()
		readers[i] = &readerState{sess: sess, tc: tc}
	}

	// Confirm all clients are connected.
	clients := srv.ConnectedClients()
	if len(clients) != numReaders+1 {
		t.Fatalf("expected %d connected clients, got %d", numReaders+1, len(clients))
	}

	var wg sync.WaitGroup

	// --- Writer goroutine: write 1, 2, 3, ... numWrites with delays ---

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int32(1); i <= numWrites; i++ {
			writeData := make([]byte, 8)
			binary.LittleEndian.PutUint16(writeData[0:], uint16(cip.TypeDINT))
			binary.LittleEndian.PutUint16(writeData[2:], 1)
			binary.LittleEndian.PutUint32(writeData[4:], uint32(i))

			resp, err := writerSess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
				Service:     cip.ServiceWriteTag,
				RequestPath: tagPath,
				RequestData: writeData,
			})
			if err != nil {
				t.Errorf("write %d: %v", i, err)
				return
			}
			if !resp.IsSuccess() {
				t.Errorf("write %d: CIP status 0x%02X", i, resp.GeneralStatus)
				return
			}
			time.Sleep(writeDelay)
		}
	}()

	// --- Reader goroutines: poll until they see the final write ---

	for _, r := range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				resp, err := r.sess.SendCIPRequest(ctx, &cip.MessageRouterRequest{
					Service:     cip.ServiceReadTag,
					RequestPath: tagPath,
					RequestData: []byte{0x01, 0x00},
				})
				if err != nil {
					return // connection closed
				}
				if !resp.IsSuccess() || len(resp.ResponseData) < 6 {
					continue
				}

				v := int32(binary.LittleEndian.Uint32(resp.ResponseData[2:6]))
				r.reads.Add(1)
				if v > r.lastSeen.Load() {
					r.lastSeen.Store(v)
				}

				// Stop once we've seen the final written value.
				if v >= numWrites {
					return
				}
			}
		}()
	}

	// --- Wait for all goroutines with timeout ---

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for writer + readers to complete")
	}

	// --- Verify reader results ---

	for i, r := range readers {
		if r.reads.Load() == 0 {
			t.Errorf("reader %d: completed 0 reads", i)
		}
		if r.lastSeen.Load() < numWrites {
			t.Errorf("reader %d: last seen %d, want >= %d", i, r.lastSeen.Load(), numWrites)
		}
	}

	// --- Clean shutdown: writer ---

	writerSess.Unregister(ctx)
	writerTC.Close()

	// --- Clean shutdown: all readers ---

	for _, r := range readers {
		r.sess.Unregister(ctx)
		r.tc.Close()
	}

	// --- Give server time to clean up all HandleConn goroutines ---

	time.Sleep(200 * time.Millisecond)

	// --- Verify server state is clean ---

	clients = srv.ConnectedClients()
	if len(clients) != 0 {
		t.Errorf("expected 0 connected clients after shutdown, got %d", len(clients))
	}

	// --- Server shutdown ---

	srv.Stop()
}

// TestServerShutdownDrain verifies that Stop() actively closes all tracked
// client connections, causing HandleConn goroutines to exit cleanly.
func TestServerShutdownDrain(t *testing.T) {
	router := cip.NewMessageRouter()

	const numClients = 3
	ln := newChanListener(numClients)

	srv := NewServer(router, WithServerListener(ln))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}

	type clientPair struct {
		clientConn net.Conn
	}
	clients := make([]clientPair, numClients)

	for i := 0; i < numClients; i++ {
		serverConn, clientConn := net.Pipe()
		clients[i] = clientPair{clientConn: clientConn}
		ln.ch <- serverConn
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for all clients to be registered.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) == numClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != numClients {
		t.Fatalf("expected %d connected clients, got %d", numClients, got)
	}

	// Register a session on each client to prove they are fully active.
	for i := range clients {
		session := registerSession(t, clients[i].clientConn)
		if session == 0 {
			t.Fatalf("client %d: session handle should not be 0", i)
		}
	}

	// Start goroutines that wait for reads to fail (proving conn was closed).
	var wg sync.WaitGroup
	wg.Add(numClients)
	readErrs := make([]error, numClients)
	for i := range clients {
		i := i
		go func() {
			defer wg.Done()
			buf := make([]byte, 1)
			_, err := clients[i].clientConn.Read(buf)
			readErrs[i] = err
		}()
	}

	// Stop the server — this should close all client connections.
	if err := srv.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	// Wait for all HandleConn goroutines to exit (reads should unblock).
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleConn goroutines did not exit within timeout after Stop()")
	}

	// All reads should have returned errors.
	for i, err := range readErrs {
		if err == nil {
			t.Errorf("client %d: expected read error after Stop(), got nil", i)
		}
	}

	// Server should have no tracked clients.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != 0 {
		t.Errorf("expected 0 connected clients after Stop(), got %d", got)
	}

	// Clean up client conns.
	for _, c := range clients {
		c.clientConn.Close()
	}
}

// TestServerMaxClients verifies the server rejects connections once the
// configured max-client limit is reached, and allows new connections once
// an existing client disconnects.
func TestServerMaxClients(t *testing.T) {
	const maxClients = 2

	router := cip.NewMessageRouter()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := NewServer(router,
		WithServerListener(ln),
		WithMaxClients(maxClients),
	)
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	addr := ln.Addr().String()

	conn1, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 2: %v", err)
	}
	defer conn2.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) >= maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != maxClients {
		t.Fatalf("expected %d connected clients, got %d", maxClients, got)
	}

	// 3rd connection should be rejected.
	conn3, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 3: %v", err)
	}
	defer conn3.Close()

	conn3.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, readErr := conn3.Read(buf)
	if readErr == nil {
		t.Fatal("expected 3rd client to be rejected, but read succeeded")
	}

	// Disconnect one client to free a slot.
	conn1.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) < maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// New connection should now succeed.
	conn4, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 4: %v", err)
	}
	defer conn4.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) >= maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != maxClients {
		t.Fatalf("expected %d connected clients after reconnect, got %d", maxClients, got)
	}
}
