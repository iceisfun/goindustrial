package client

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

func TestClient_WriteTag(t *testing.T) {
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

		// Read SendRRData (WriteTag)
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
			// Verify request data?
			// We can inspect dataBuf.
			// Path: 2 bytes count, [0] 0x91, len, 'T', 'e', 's', 't', 'T', 'a', 'g', padding...
			// Service: 0x4D
			// RequestData: Type(0xC4 0x00), Elements(0x01 0x00), Value(...)
		}

		// Send SendRRData Response
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304)

		// CIP Response: Service(0xCD = WriteTag Reply), Status(0)
		// Data: None usually for success, or maybe some status bytes?
		// WriteTag reply is usually just service|0x80 (0xCD) and status 00.
		cipData := []byte{0xCD, 0x00, 0x00, 0x00}

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
		t.Fatalf("NewClient() error = %v", err)
	}
	defer client.Close()

	// Write DINT
	var val int32 = 987654321
	err = client.WriteTag("TestTag", val)
	if err != nil {
		t.Fatalf("WriteTag() error = %v", err)
	}
}
