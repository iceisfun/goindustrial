package session

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"testing"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/eip"
	"github.com/iceisfun/goeip/pkg/transport"
)

// mockTransport implements transport.Transport for testing
type mockTransport struct {
	mu           sync.Mutex
	sendFunc     func(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error
	receiveFunc  func() (*eip.EncapsulationHeader, []byte, error)
	closeFunc    func() error
	closed       bool
	sentCommands []eip.Command
	sentData     [][]byte
	receiveQueue []receiveResult
	receiveIndex int
}

type receiveResult struct {
	header *eip.EncapsulationHeader
	data   []byte
	err    error
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		sentCommands: make([]eip.Command, 0),
		sentData:     make([][]byte, 0),
		receiveQueue: make([]receiveResult, 0),
	}
}

func (m *mockTransport) Send(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendFunc != nil {
		return m.sendFunc(cmd, data, sessionHandle)
	}
	m.sentCommands = append(m.sentCommands, cmd)
	m.sentData = append(m.sentData, data)
	return nil
}

func (m *mockTransport) Receive() (*eip.EncapsulationHeader, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.receiveFunc != nil {
		return m.receiveFunc()
	}
	if m.receiveIndex < len(m.receiveQueue) {
		result := m.receiveQueue[m.receiveIndex]
		m.receiveIndex++
		return result.header, result.data, result.err
	}
	return nil, nil, errors.New("no more receive results")
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockTransport) queueReceive(header *eip.EncapsulationHeader, data []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receiveQueue = append(m.receiveQueue, receiveResult{header, data, err})
}

var _ transport.Transport = (*mockTransport)(nil)

func TestNewSession(t *testing.T) {
	mt := newMockTransport()

	s := NewSession(mt, nil)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
	if s.transport != mt {
		t.Error("Transport not set correctly")
	}
}

func TestNewSession_NilLogger(t *testing.T) {
	mt := newMockTransport()

	// Should not panic with nil logger
	s := NewSession(mt, nil)
	if s == nil {
		t.Fatal("NewSession returned nil")
	}
}

func TestSession_Register(t *testing.T) {
	mt := newMockTransport()

	// Queue successful register response
	mt.queueReceive(&eip.EncapsulationHeader{
		Command:       eip.CommandRegisterSession,
		Length:        4,
		SessionHandle: 0x12345678,
		Status:        eip.StatusSuccess,
	}, []byte{0x01, 0x00, 0x00, 0x00}, nil) // Protocol version + options

	s := NewSession(mt, internal.NopLogger())

	err := s.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if s.sessionHandle != 0x12345678 {
		t.Errorf("sessionHandle = 0x%08X, want 0x12345678", s.sessionHandle)
	}

	// Check that RegisterSession command was sent
	if len(mt.sentCommands) != 1 {
		t.Fatalf("Expected 1 sent command, got %d", len(mt.sentCommands))
	}
	if mt.sentCommands[0] != eip.CommandRegisterSession {
		t.Errorf("Sent command = 0x%04X, want 0x%04X", mt.sentCommands[0], eip.CommandRegisterSession)
	}
}

func TestSession_Register_Failure(t *testing.T) {
	mt := newMockTransport()

	// Queue failed register response
	mt.queueReceive(&eip.EncapsulationHeader{
		Command:       eip.CommandRegisterSession,
		Length:        0,
		SessionHandle: 0,
		Status:        eip.StatusInvalidCommand,
	}, nil, nil)

	s := NewSession(mt, internal.NopLogger())

	err := s.Register()
	if err == nil {
		t.Error("Expected Register to fail")
	}
}

func TestSession_Register_SendError(t *testing.T) {
	mt := newMockTransport()
	mt.sendFunc = func(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error {
		return errors.New("send failed")
	}

	s := NewSession(mt, internal.NopLogger())

	err := s.Register()
	if err == nil {
		t.Error("Expected Register to fail on send error")
	}
}

func TestSession_Register_ReceiveError(t *testing.T) {
	mt := newMockTransport()
	mt.queueReceive(nil, nil, errors.New("receive failed"))

	s := NewSession(mt, internal.NopLogger())

	err := s.Register()
	if err == nil {
		t.Error("Expected Register to fail on receive error")
	}
}

func TestSession_Unregister(t *testing.T) {
	mt := newMockTransport()
	mt.queueReceive(&eip.EncapsulationHeader{
		SessionHandle: 0x12345678,
		Status:        eip.StatusSuccess,
	}, nil, nil)

	s := NewSession(mt, internal.NopLogger())
	s.sessionHandle = 0x12345678

	err := s.Unregister()
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Check that UnregisterSession command was sent
	if len(mt.sentCommands) != 1 {
		t.Fatalf("Expected 1 sent command, got %d", len(mt.sentCommands))
	}
	if mt.sentCommands[0] != eip.CommandUnregisterSession {
		t.Errorf("Sent command = 0x%04X, want 0x%04X", mt.sentCommands[0], eip.CommandUnregisterSession)
	}
}

func TestSession_Close(t *testing.T) {
	mt := newMockTransport()

	s := NewSession(mt, internal.NopLogger())

	err := s.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !mt.closed {
		t.Error("Transport was not closed")
	}
}

func TestSession_SendRRData(t *testing.T) {
	mt := newMockTransport()

	// Build a valid response CPF
	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, []byte{0x8E, 0x00, 0x00, 0x00, 0xDE, 0xAD}), // Response
	)
	cpfData, _ := respCPF.Encode()

	// RRData response: Interface Handle (4) + Timeout (2) + CPF
	rrRespData := make([]byte, 6+len(cpfData))
	copy(rrRespData[6:], cpfData)

	mt.queueReceive(&eip.EncapsulationHeader{
		Command:       eip.CommandSendRRData,
		Length:        uint16(len(rrRespData)),
		SessionHandle: 0x12345678,
		Status:        eip.StatusSuccess,
	}, rrRespData, nil)

	s := NewSession(mt, internal.NopLogger())
	s.sessionHandle = 0x12345678

	resp, err := s.SendRRData([]byte{0x0E, 0x03, 0x20, 0x04, 0x24, 0x01, 0x30, 0x03})
	if err != nil {
		t.Fatalf("SendRRData failed: %v", err)
	}

	// Check we got the response data
	if len(resp) == 0 {
		t.Error("Expected response data")
	}
}

func TestSession_SendRRData_Failure(t *testing.T) {
	mt := newMockTransport()

	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandSendRRData,
		Status:  0x00000001, // Fail
	}, nil, nil)

	s := NewSession(mt, internal.NopLogger())
	s.sessionHandle = 0x12345678

	_, err := s.SendRRData([]byte{0x0E, 0x03, 0x20, 0x04})
	if err == nil {
		t.Error("Expected SendRRData to fail")
	}
}

func TestSession_SendRRData_ShortResponse(t *testing.T) {
	mt := newMockTransport()

	// Response data too short (less than 6 bytes)
	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandSendRRData,
		Length:  4,
		Status:  eip.StatusSuccess,
	}, []byte{0x00, 0x00, 0x00, 0x00}, nil)

	s := NewSession(mt, internal.NopLogger())
	s.sessionHandle = 0x12345678

	_, err := s.SendRRData([]byte{0x0E, 0x03, 0x20, 0x04})
	if err == nil {
		t.Error("Expected SendRRData to fail on short response")
	}
}

func TestSession_SendCIPRequest(t *testing.T) {
	mt := newMockTransport()

	// Build CIP response
	cipResp := &cip.MessageRouterResponse{
		Service:       0x8E, // GetAttributeSingle reply
		GeneralStatus: cip.StatusSuccess,
		ResponseData:  []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}

	// Encode response manually
	respBuf := new(bytes.Buffer)
	binary.Write(respBuf, binary.LittleEndian, cipResp.Service)
	binary.Write(respBuf, binary.LittleEndian, cipResp.Reserved)
	binary.Write(respBuf, binary.LittleEndian, cipResp.GeneralStatus)
	binary.Write(respBuf, binary.LittleEndian, cipResp.ExtStatusSize)
	respBuf.Write(cipResp.ResponseData)

	// Build CPF response
	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, respBuf.Bytes()),
	)
	cpfData, _ := respCPF.Encode()

	rrRespData := make([]byte, 6+len(cpfData))
	copy(rrRespData[6:], cpfData)

	mt.queueReceive(&eip.EncapsulationHeader{
		Command:       eip.CommandSendRRData,
		Length:        uint16(len(rrRespData)),
		SessionHandle: 0x12345678,
		Status:        eip.StatusSuccess,
	}, rrRespData, nil)

	s := NewSession(mt, internal.NopLogger())
	s.sessionHandle = 0x12345678

	req := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeSingle,
		RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}),
	}

	resp, err := s.SendCIPRequest(req)
	if err != nil {
		t.Fatalf("SendCIPRequest failed: %v", err)
	}

	if resp.Service != 0x8E {
		t.Errorf("Response Service = 0x%02X, want 0x8E", resp.Service)
	}
	if resp.GeneralStatus != cip.StatusSuccess {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x00", resp.GeneralStatus)
	}
	if len(resp.ResponseData) != 4 {
		t.Errorf("Response data length = %d, want 4", len(resp.ResponseData))
	}
}

func TestSession_ListIdentity(t *testing.T) {
	mt := newMockTransport()

	// Build ListIdentity response
	// Format:
	// - Item Count (2)
	// - Item Type (2) = 0x000C
	// - Item Length (2)
	// - Item Data:
	//   - EncapsVersion (2)
	//   - SocketAddr (16)
	//   - VendorID (2)
	//   - DeviceType (2)
	//   - ProductCode (2)
	//   - Revision (2)
	//   - Status (2)
	//   - SerialNumber (4)
	//   - ProductNameLen (1)
	//   - ProductName (variable)
	//   - State (1)

	identityData := new(bytes.Buffer)

	// Item count = 1
	binary.Write(identityData, binary.LittleEndian, uint16(1))

	// Item Type (0x000C)
	binary.Write(identityData, binary.LittleEndian, uint16(0x000C))

	// Calculate item data length:
	// EncapsVersion(2) + SocketAddr(16) + VendorID(2) + DeviceType(2) + ProductCode(2) +
	// Revision(2) + Status(2) + SerialNumber(4) + ProductNameLen(1) + ProductName(4) + State(1) = 38
	productName := "Test"
	itemDataLen := 2 + 16 + 2 + 2 + 2 + 2 + 2 + 4 + 1 + len(productName) + 1
	binary.Write(identityData, binary.LittleEndian, uint16(itemDataLen))

	// EncapsVersion
	binary.Write(identityData, binary.LittleEndian, uint16(1))

	// SocketAddr (16 bytes)
	socketAddr := [16]byte{}
	binary.LittleEndian.PutUint16(socketAddr[0:], 2)  // sin_family = AF_INET
	binary.BigEndian.PutUint16(socketAddr[2:], 44818) // sin_port (big-endian)
	copy(socketAddr[4:8], []byte{192, 168, 1, 100})   // sin_addr
	identityData.Write(socketAddr[:])

	// VendorID
	binary.Write(identityData, binary.LittleEndian, uint16(1))

	// DeviceType
	binary.Write(identityData, binary.LittleEndian, uint16(14))

	// ProductCode
	binary.Write(identityData, binary.LittleEndian, uint16(1))

	// Revision (2 bytes: major, minor)
	identityData.Write([]byte{1, 1})

	// Status
	binary.Write(identityData, binary.LittleEndian, uint16(0))

	// SerialNumber
	binary.Write(identityData, binary.LittleEndian, uint32(12345))

	// ProductName length-prefixed string
	binary.Write(identityData, binary.LittleEndian, uint8(len(productName)))
	identityData.WriteString(productName)

	// State
	binary.Write(identityData, binary.LittleEndian, uint8(0))

	respData := identityData.Bytes()

	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandListIdentity,
		Length:  uint16(len(respData)),
		Status:  eip.StatusSuccess,
	}, respData, nil)

	s := NewSession(mt, internal.NopLogger())

	items, err := s.ListIdentity()
	if err != nil {
		t.Fatalf("ListIdentity failed: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}

	if len(items) > 0 {
		if items[0].ProductName != productName {
			t.Errorf("ProductName = %q, want %q", items[0].ProductName, productName)
		}
		if items[0].VendorID != 1 {
			t.Errorf("VendorID = %d, want 1", items[0].VendorID)
		}
	}
}

func TestSession_ListIdentity_Failure(t *testing.T) {
	mt := newMockTransport()

	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandListIdentity,
		Status:  0x00000001, // Fail
	}, nil, nil)

	s := NewSession(mt, internal.NopLogger())

	_, err := s.ListIdentity()
	if err == nil {
		t.Error("Expected ListIdentity to fail")
	}
}

func TestSession_ListServices(t *testing.T) {
	mt := newMockTransport()

	// Build ListServices response
	servicesData := make([]byte, 0)

	// Item count = 1
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 1)
	servicesData = append(servicesData, itemCount...)

	// Item Type (0x0100) + Item Length
	itemType := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemType, 0x0100)
	servicesData = append(servicesData, itemType...)

	// Service item data
	// Protocol Version (2) + Capability Flags (2) + Service Name (16)
	itemData := make([]byte, 20)
	binary.LittleEndian.PutUint16(itemData[0:], 1)      // Protocol version
	binary.LittleEndian.PutUint16(itemData[2:], 0x0100) // Capabilities
	copy(itemData[4:], "Communications")                // Service name

	itemLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemLen, uint16(len(itemData)))
	servicesData = append(servicesData, itemLen...)
	servicesData = append(servicesData, itemData...)

	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandListServices,
		Length:  uint16(len(servicesData)),
		Status:  eip.StatusSuccess,
	}, servicesData, nil)

	s := NewSession(mt, internal.NopLogger())

	items, err := s.ListServices()
	if err != nil {
		t.Fatalf("ListServices failed: %v", err)
	}

	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}
}

func TestSession_ListServices_Failure(t *testing.T) {
	mt := newMockTransport()

	mt.queueReceive(&eip.EncapsulationHeader{
		Command: eip.CommandListServices,
		Status:  0x00000001, // Fail
	}, nil, nil)

	s := NewSession(mt, internal.NopLogger())

	_, err := s.ListServices()
	if err == nil {
		t.Error("Expected ListServices to fail")
	}
}
