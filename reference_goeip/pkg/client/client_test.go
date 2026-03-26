package client

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

// MockLogger implements internal.Logger for testing
type MockLogger struct{}

func (l *MockLogger) Debugf(format string, args ...interface{}) {}
func (l *MockLogger) Infof(format string, args ...interface{})  {}
func (l *MockLogger) Warnf(format string, args ...interface{})  {}
func (l *MockLogger) Errorf(format string, args ...interface{}) {}

func TestNewClient(t *testing.T) {
	// Start a dummy TCP server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	// Handle connection in a goroutine
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read Register Session Request
		buf := make([]byte, 1024)
		_, err = conn.Read(buf)
		if err != nil {
			return
		}

		// Send Register Session Response (Success)
		// Header: Command(0x0065), Length(4), SessionHandle(0x01020304), Status(0), Context(0), Options(0)
		// Data: ProtocolVersion(1), OptionFlags(0)
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)     // Command
		binary.LittleEndian.PutUint16(resp[2:4], 4)          // Length
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304) // Session Handle
		binary.LittleEndian.PutUint16(resp[24:26], 1)        // Protocol Version
		conn.Write(resp)
	}()

	client, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()
}

func TestClient_ReadTag(t *testing.T) {
	// Start a dummy TCP server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read Register Session
		buf := make([]byte, 1024)
		conn.Read(buf)

		// Send Register Session Response
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)
		binary.LittleEndian.PutUint16(resp[2:4], 4)
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304)
		binary.LittleEndian.PutUint16(resp[24:26], 1)
		conn.Write(resp)

		// Read SendRRData (ReadTag)
		// Read Header first
		headerBuf := make([]byte, 24)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		if dataLen > 0 {
			dataBuf := make([]byte, dataLen)
			if _, err := io.ReadFull(conn, dataBuf); err != nil {
				return
			}
		}

		// Send SendRRData Response with ReadTag Success
		// We need to construct a valid CIP response
		// Encapsulation Header (24 bytes)
		// Interface Handle (4 bytes)
		// Timeout (2 bytes)
		// Item Count (2 bytes)
		// Address Item (Type + Length + Data)
		// Data Item (Type + Length + Data)

		// Encapsulation Header
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)     // SendRRData
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304) // Session Handle

		// CPF Items
		// Item 1: Null Address (0x0000), Len 0
		// Item 2: Unconnected Data (0x00B2), Len X
		// CIP Response: Service(0xCC = ReadTag Reply), Reserved(0), Status(0), ExtStatusSize(0)
		// Data: Type(0xC4 = DINT), Value(0xDEADBEEF)
		cipData := []byte{0xCC, 0x00, 0x00, 0x00, 0xC4, 0x00, 0xEF, 0xBE, 0xAD, 0xDE}

		cpf := make([]byte, 2+4+4+len(cipData))
		binary.LittleEndian.PutUint16(cpf[0:2], 2) // Item Count
		// Item 1
		binary.LittleEndian.PutUint16(cpf[2:4], 0x0000)
		binary.LittleEndian.PutUint16(cpf[4:6], 0)
		// Item 2
		binary.LittleEndian.PutUint16(cpf[6:8], 0x00B2)
		binary.LittleEndian.PutUint16(cpf[8:10], uint16(len(cipData)))
		copy(cpf[10:], cipData)

		// Update Encapsulation Length
		binary.LittleEndian.PutUint16(encap[2:4], uint16(6+len(cpf))) // Interface Handle(4) + Timeout(2) + CPF

		// Write Response
		conn.Write(encap)
		// Interface Handle + Timeout
		conn.Write([]byte{0, 0, 0, 0, 0, 0})
		// CPF
		conn.Write(cpf)
	}()

	client, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	data, err := client.ReadTag("TestTag")
	if err != nil {
		t.Fatalf("ReadTag() error = %v", err)
	}

	// Expect Type(0xC4, 0x00) + Value(0xDEADBEEF)
	if len(data) != 6 {
		t.Errorf("ReadTag() length = %d, want 6", len(data))
	}
	if data[0] != 0xC4 || data[1] != 0x00 {
		t.Errorf("ReadTag() type = %X, want C4 00", data[0:2])
	}
	val := binary.LittleEndian.Uint32(data[2:])
	if val != 0xDEADBEEF {
		t.Errorf("ReadTag() value = %X, want DEADBEEF", val)
	}
}

func TestClient_ReadTagInto(t *testing.T) {
	// Start a dummy TCP server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read Register Session
		buf := make([]byte, 1024)
		_, err = conn.Read(buf)
		if err != nil {
			t.Logf("MockServer: Read Register Session failed: %v", err)
			return
		}
		// Send Register Session Response
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)
		binary.LittleEndian.PutUint16(resp[2:4], 4)
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304)
		binary.LittleEndian.PutUint16(resp[24:26], 1)
		if _, err := conn.Write(resp); err != nil {
			t.Logf("MockServer: Write Register Session Response failed: %v", err)
			return
		}

		// Read SendRRData (ReadTagInto)
		headerBuf := make([]byte, 24)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			t.Logf("MockServer: Read SendRRData Header failed: %v", err)
			return
		}
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		if dataLen > 0 {
			dataBuf := make([]byte, dataLen)
			if _, err := io.ReadFull(conn, dataBuf); err != nil {
				t.Logf("MockServer: Read SendRRData Data failed: %v", err)
				return
			}
		}

		// Send SendRRData Response
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304)

		// CIP Response: Service(0xCC), Status(0)
		// Data: Type(0xC4 = DINT), Value(123456789)
		cipData := []byte{0xCC, 0x00, 0x00, 0x00, 0xC4, 0x00}
		valBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(valBytes, 123456789)
		cipData = append(cipData, valBytes...)

		cpf := make([]byte, 2+4+4+len(cipData))
		binary.LittleEndian.PutUint16(cpf[0:2], 2)
		binary.LittleEndian.PutUint16(cpf[2:4], 0x0000)
		binary.LittleEndian.PutUint16(cpf[4:6], 0)
		binary.LittleEndian.PutUint16(cpf[6:8], 0x00B2)
		binary.LittleEndian.PutUint16(cpf[8:10], uint16(len(cipData)))
		copy(cpf[10:], cipData)

		binary.LittleEndian.PutUint16(encap[2:4], uint16(6+len(cpf)))
		conn.Write(encap)
		conn.Write([]byte{0, 0, 0, 0, 0, 0})
		conn.Write(cpf)
	}()

	client, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	var val int32
	err = client.ReadTagInto("TestTag", &val)
	if err != nil {
		t.Fatalf("ReadTagInto() error = %v", err)
	}

	if val != 123456789 {
		t.Errorf("ReadTagInto() value = %d, want 123456789", val)
	}
}

func TestClient_ReadTimer(t *testing.T) {
	// Start a dummy TCP server
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read Register Session
		buf := make([]byte, 1024)
		conn.Read(buf)
		// Send Register Session Response
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)
		binary.LittleEndian.PutUint16(resp[2:4], 4)
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304)
		binary.LittleEndian.PutUint16(resp[24:26], 1)
		conn.Write(resp)

		// Read SendRRData (ReadTimer)
		headerBuf := make([]byte, 24)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		if dataLen > 0 {
			dataBuf := make([]byte, dataLen)
			if _, err := io.ReadFull(conn, dataBuf); err != nil {
				return
			}
		}

		// Send SendRRData Response
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304)

		// CIP Response: Service(0xCC), Status(0)
		// Data: Type(0xA002 = Struct?), Value(Timer Data)
		// Timer Data: 14 bytes
		timerData := make([]byte, 14)
		// Status: EN(bit 31)
		binary.LittleEndian.PutUint32(timerData[2:6], 1<<31)
		// PRE: 5000
		binary.LittleEndian.PutUint32(timerData[6:10], 5000)
		// ACC: 2500
		binary.LittleEndian.PutUint32(timerData[10:14], 2500)

		cipData := []byte{0xCC, 0x00, 0x00, 0x00, 0x02, 0xA0, 0x00, 0x00} // Type 0xA002 (struct), struct handle 0x0000
		cipData = append(cipData, timerData...)

		cpf := make([]byte, 2+4+4+len(cipData))
		binary.LittleEndian.PutUint16(cpf[0:2], 2)
		binary.LittleEndian.PutUint16(cpf[2:4], 0x0000)
		binary.LittleEndian.PutUint16(cpf[4:6], 0)
		binary.LittleEndian.PutUint16(cpf[6:8], 0x00B2)
		binary.LittleEndian.PutUint16(cpf[8:10], uint16(len(cipData)))
		copy(cpf[10:], cipData)

		binary.LittleEndian.PutUint16(encap[2:4], uint16(6+len(cpf)))
		conn.Write(encap)
		conn.Write([]byte{0, 0, 0, 0, 0, 0})
		conn.Write(cpf)
	}()

	client, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer client.Close()

	timer, err := client.ReadTimer("TestTimer")
	if err != nil {
		t.Fatalf("ReadTimer() error = %v", err)
	}

	if !timer.EN {
		t.Errorf("Timer EN = false, want true")
	}
	if timer.PRE != 5000 {
		t.Errorf("Timer PRE = %d, want 5000", timer.PRE)
	}
	if timer.ACC != 2500 {
		t.Errorf("Timer ACC = %d, want 2500", timer.ACC)
	}
}
