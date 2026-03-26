package ethernetip

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
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
