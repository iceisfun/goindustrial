package server

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/eip"
)

// mockObject implements cip.Object for testing
type mockObject struct {
	handleFunc func(service cip.USINT, path cip.Path, data []byte) ([]byte, error)
}

func (m *mockObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	if m.handleFunc != nil {
		return m.handleFunc(service, path, data)
	}
	return []byte{0x01, 0x02, 0x03, 0x04}, nil
}

func TestNewServer(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.router != router {
		t.Error("Server router not set correctly")
	}
}

func TestServer_Start(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Start on a random available port
	err := server.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Server started successfully
}

func TestServer_RegisterSession(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Create a pipe to simulate connection
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	// Handle connection in goroutine
	go server.handleConnection(serverConn)

	// Send RegisterSession command
	header := &eip.EncapsulationHeader{
		Command:       eip.CommandRegisterSession,
		Length:        4,
		SessionHandle: 0,
		Status:        0,
	}
	headerBytes := header.Bytes()

	// RegisterSession data: Protocol Version (2) + Options (2)
	regData := make([]byte, 4)
	binary.LittleEndian.PutUint16(regData[0:], 1) // Protocol version
	binary.LittleEndian.PutUint16(regData[2:], 0) // Options

	_, err := clientConn.Write(headerBytes)
	if err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	_, err = clientConn.Write(regData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Read response
	respHeader := make([]byte, 24)
	_, err = io.ReadFull(clientConn, respHeader)
	if err != nil {
		t.Fatalf("Failed to read response header: %v", err)
	}

	// Check response command
	respCmd := binary.LittleEndian.Uint16(respHeader[0:2])
	if eip.Command(respCmd) != eip.CommandRegisterSession {
		t.Errorf("Response command = 0x%04X, want 0x%04X", respCmd, eip.CommandRegisterSession)
	}

	// Check status (should be success)
	status := binary.LittleEndian.Uint32(respHeader[8:12])
	if status != 0 {
		t.Errorf("Response status = 0x%08X, want 0", status)
	}

	// Check session handle is set
	sessionHandle := binary.LittleEndian.Uint32(respHeader[4:8])
	if sessionHandle == 0 {
		t.Error("Session handle should not be 0")
	}

	// Read response data
	respLen := binary.LittleEndian.Uint16(respHeader[2:4])
	if respLen > 0 {
		respData := make([]byte, respLen)
		_, err = io.ReadFull(clientConn, respData)
		if err != nil {
			t.Fatalf("Failed to read response data: %v", err)
		}
	}
}

func TestServer_UnregisterSession(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	// Track when connection closes
	done := make(chan struct{})
	go func() {
		server.handleConnection(serverConn)
		close(done)
	}()

	// First register session
	regHeader := &eip.EncapsulationHeader{
		Command:       eip.CommandRegisterSession,
		Length:        4,
		SessionHandle: 0,
	}
	clientConn.Write(regHeader.Bytes())
	regData := make([]byte, 4)
	binary.LittleEndian.PutUint16(regData[0:], 1)
	clientConn.Write(regData)

	// Read response
	respHeader := make([]byte, 24)
	io.ReadFull(clientConn, respHeader)
	respLen := binary.LittleEndian.Uint16(respHeader[2:4])
	if respLen > 0 {
		respData := make([]byte, respLen)
		io.ReadFull(clientConn, respData)
	}

	sessionHandle := binary.LittleEndian.Uint32(respHeader[4:8])

	// Now send UnregisterSession
	unregHeader := &eip.EncapsulationHeader{
		Command:       eip.CommandUnregisterSession,
		Length:        0,
		SessionHandle: eip.SessionHandle(sessionHandle),
	}
	clientConn.Write(unregHeader.Bytes())

	// Connection should close
	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Connection should have closed after UnregisterSession")
	}
}

func TestServer_SendRRData(t *testing.T) {
	router := cip.NewMessageRouter()

	// Register a mock object
	mockObj := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			return []byte{0xDE, 0xAD, 0xBE, 0xEF}, nil
		},
	}
	router.RegisterObject(0x04, mockObj) // Assembly class

	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go server.handleConnection(serverConn)

	// First register session
	regHeader := &eip.EncapsulationHeader{
		Command: eip.CommandRegisterSession,
		Length:  4,
	}
	clientConn.Write(regHeader.Bytes())
	clientConn.Write(make([]byte, 4))

	// Read response
	respBuf := make([]byte, 24)
	io.ReadFull(clientConn, respBuf)
	respLen := binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		io.ReadFull(clientConn, make([]byte, respLen))
	}
	sessionHandle := binary.LittleEndian.Uint32(respBuf[4:8])

	// Build SendRRData request
	// Message Router Request: Service + PathSize + Path + Data
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}), // Class 4, Instance 1, Attr 3
		RequestData: nil,
	}
	mrReqBytes, _ := mrReq.Encode()

	// Build CPF
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
	)
	cpfData, _ := cpf.Encode()

	// Build RRData: Interface Handle (4) + Timeout (2) + CPF
	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	rrHeader := &eip.EncapsulationHeader{
		Command:       eip.CommandSendRRData,
		Length:        uint16(len(rrData)),
		SessionHandle: eip.SessionHandle(sessionHandle),
	}

	clientConn.Write(rrHeader.Bytes())
	clientConn.Write(rrData)

	// Read response
	io.ReadFull(clientConn, respBuf)
	status := binary.LittleEndian.Uint32(respBuf[8:12])
	if status != 0 {
		t.Errorf("RRData response status = 0x%08X, want 0", status)
	}

	respLen = binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		respData := make([]byte, respLen)
		io.ReadFull(clientConn, respData)
		// Response should contain CPF with our mock data
	}
}

func TestServer_SendRRData_Error(t *testing.T) {
	router := cip.NewMessageRouter()

	// Register a mock object that returns an error
	mockObj := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			return nil, cip.Error{Status: cip.StatusObjectDoesNotExist}
		},
	}
	router.RegisterObject(0x04, mockObj)

	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go server.handleConnection(serverConn)

	// Register session
	regHeader := &eip.EncapsulationHeader{
		Command: eip.CommandRegisterSession,
		Length:  4,
	}
	clientConn.Write(regHeader.Bytes())
	clientConn.Write(make([]byte, 4))

	respBuf := make([]byte, 24)
	io.ReadFull(clientConn, respBuf)
	respLen := binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		io.ReadFull(clientConn, make([]byte, respLen))
	}
	sessionHandle := binary.LittleEndian.Uint32(respBuf[4:8])

	// Build request to non-existent class
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0xFF, 0x24, 0x01, 0x30, 0x03}), // Class 0xFF (not registered)
	}
	mrReqBytes, _ := mrReq.Encode()

	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrReqBytes),
	)
	cpfData, _ := cpf.Encode()

	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	rrHeader := &eip.EncapsulationHeader{
		Command:       eip.CommandSendRRData,
		Length:        uint16(len(rrData)),
		SessionHandle: eip.SessionHandle(sessionHandle),
	}

	clientConn.Write(rrHeader.Bytes())
	clientConn.Write(rrData)

	// Read response - should still succeed at encapsulation level
	io.ReadFull(clientConn, respBuf)
	// The CIP error is in the response data, not the encapsulation status
}

func TestServer_MaxPacketSize(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		server.handleConnection(serverConn)
		close(done)
	}()

	// Send header with length > MaxPacketSize (4096)
	header := &eip.EncapsulationHeader{
		Command: eip.CommandSendRRData,
		Length:  5000, // Exceeds max
	}
	clientConn.Write(header.Bytes())

	// Connection should be closed
	select {
	case <-done:
		// Expected - server closed connection due to oversized packet
	case <-time.After(time.Second):
		t.Error("Server should close connection for oversized packet")
	}
}

func TestServer_UnsupportedCommand(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go server.handleConnection(serverConn)

	// Send unsupported command
	header := &eip.EncapsulationHeader{
		Command: eip.Command(0xFFFF), // Unsupported
		Length:  0,
	}
	clientConn.Write(header.Bytes())

	// Read response
	respBuf := make([]byte, 24)
	io.ReadFull(clientConn, respBuf)

	status := binary.LittleEndian.Uint32(respBuf[8:12])
	if status != 0x0001 {
		t.Errorf("Response status = 0x%08X, want 0x0001 (fail)", status)
	}
}

func TestServer_SendUnitData(t *testing.T) {
	router := cip.NewMessageRouter()

	mockObj := &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			return []byte{0x11, 0x22, 0x33, 0x44}, nil
		},
	}
	router.RegisterObject(0x04, mockObj)

	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go server.handleConnection(serverConn)

	// Register session first
	regHeader := &eip.EncapsulationHeader{
		Command: eip.CommandRegisterSession,
		Length:  4,
	}
	clientConn.Write(regHeader.Bytes())
	clientConn.Write(make([]byte, 4))

	respBuf := make([]byte, 24)
	io.ReadFull(clientConn, respBuf)
	respLen := binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		io.ReadFull(clientConn, make([]byte, respLen))
	}
	sessionHandle := binary.LittleEndian.Uint32(respBuf[4:8])

	// Build SendUnitData request with connected addressing
	// Message Router Request
	mrReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}),
	}
	mrReqBytes, _ := mrReq.Encode()

	// Connected Data Item needs Sequence Count + PDU
	connDataBuf := new(bytes.Buffer)
	binary.Write(connDataBuf, binary.LittleEndian, uint16(1)) // Sequence count
	connDataBuf.Write(mrReqBytes)

	// Build CPF with Connected Address Item and Connected Data Item
	connID := uint32(0x12345678)
	connIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(connIDBytes, connID)

	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDConnectedAddress, connIDBytes),      // 0xA1
		eip.NewCPFItem(eip.ItemIDConnectedData, connDataBuf.Bytes()), // 0xB1
	)
	cpfData, _ := cpf.Encode()

	// Build UnitData: Interface Handle (4) + Timeout (2) + CPF
	unitData := make([]byte, 6+len(cpfData))
	copy(unitData[6:], cpfData)

	unitHeader := &eip.EncapsulationHeader{
		Command:       eip.CommandSendUnitData,
		Length:        uint16(len(unitData)),
		SessionHandle: eip.SessionHandle(sessionHandle),
	}

	clientConn.Write(unitHeader.Bytes())
	clientConn.Write(unitData)

	// Read response
	io.ReadFull(clientConn, respBuf)
	status := binary.LittleEndian.Uint32(respBuf[8:12])
	if status != 0 {
		t.Errorf("UnitData response status = 0x%08X, want 0", status)
	}

	respLen = binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		respData := make([]byte, respLen)
		io.ReadFull(clientConn, respData)
		// Should contain connected response with echoed sequence count
	}
}

func TestServer_HandleSendRRData_ShortData(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Test with data too short
	_, err := server.handleSendRRData([]byte{0x00, 0x01, 0x02})
	if err == nil {
		t.Error("Expected error for short data")
	}
}

func TestServer_HandleSendUnitData_ShortData(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Test with data too short
	_, err := server.handleSendUnitData([]byte{0x00, 0x01, 0x02})
	if err == nil {
		t.Error("Expected error for short data")
	}
}

func TestServer_HandleSendRRData_NoCPFItem(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Build CPF with no Unconnected Message item
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		// Missing unconnected message item
	)
	cpfData, _ := cpf.Encode()

	data := make([]byte, 6+len(cpfData))
	copy(data[6:], cpfData)

	_, err := server.handleSendRRData(data)
	if err == nil {
		t.Error("Expected error for missing unconnected message item")
	}
}

func TestServer_HandleSendUnitData_NoConnectedItems(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	// Build CPF with wrong items
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, []byte{0x01}),
	)
	cpfData, _ := cpf.Encode()

	data := make([]byte, 6+len(cpfData))
	copy(data[6:], cpfData)

	_, err := server.handleSendUnitData(data)
	if err == nil {
		t.Error("Expected error for missing connected address item")
	}
}

func TestServer_SenderContextPreserved(t *testing.T) {
	router := cip.NewMessageRouter()
	server := NewServer(router)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	go server.handleConnection(serverConn)

	// Send RegisterSession with specific sender context
	header := make([]byte, 24)
	binary.LittleEndian.PutUint16(header[0:], uint16(eip.CommandRegisterSession))
	binary.LittleEndian.PutUint16(header[2:], 4)
	// Set sender context to specific pattern
	copy(header[12:20], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88})

	clientConn.Write(header)
	clientConn.Write(make([]byte, 4))

	// Read response
	respBuf := make([]byte, 24)
	io.ReadFull(clientConn, respBuf)
	respLen := binary.LittleEndian.Uint16(respBuf[2:4])
	if respLen > 0 {
		io.ReadFull(clientConn, make([]byte, respLen))
	}

	// Check sender context is echoed back
	senderContext := respBuf[12:20]
	expected := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	for i, b := range expected {
		if senderContext[i] != b {
			t.Errorf("SenderContext[%d] = 0x%02X, want 0x%02X", i, senderContext[i], b)
		}
	}
}
