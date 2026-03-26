package client

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
)

func TestRead_Int32(t *testing.T) {
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

		// Register session
		buf := make([]byte, 1024)
		conn.Read(buf)
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)
		binary.LittleEndian.PutUint16(resp[2:4], 4)
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304)
		binary.LittleEndian.PutUint16(resp[24:26], 1)
		conn.Write(resp)

		// Read SendRRData request
		headerBuf := make([]byte, 24)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		if dataLen > 0 {
			dataBuf := make([]byte, dataLen)
			io.ReadFull(conn, dataBuf)
		}

		// Send response: DINT type (0xC4) + value 42
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304)

		cipData := []byte{0xCC, 0x00, 0x00, 0x00, 0xC4, 0x00}
		valBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(valBytes, 42)
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

	c, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer c.Close()

	val, err := Read[int32](c, "TestTag")
	if err != nil {
		t.Fatalf("Read[int32]() error = %v", err)
	}
	if val != 42 {
		t.Errorf("Read[int32]() = %d, want 42", val)
	}
}

func TestReadSlice_Int32(t *testing.T) {
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

		// Register session
		buf := make([]byte, 1024)
		conn.Read(buf)
		resp := make([]byte, 28)
		binary.LittleEndian.PutUint16(resp[0:2], 0x0065)
		binary.LittleEndian.PutUint16(resp[2:4], 4)
		binary.LittleEndian.PutUint32(resp[4:8], 0x01020304)
		binary.LittleEndian.PutUint16(resp[24:26], 1)
		conn.Write(resp)

		// Read SendRRData request
		headerBuf := make([]byte, 24)
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		if dataLen > 0 {
			dataBuf := make([]byte, dataLen)
			io.ReadFull(conn, dataBuf)
		}

		// Send response: DINT type (0xC4) + 3 values [10, 20, 30]
		encap := make([]byte, 24)
		binary.LittleEndian.PutUint16(encap[0:2], 0x006F)
		binary.LittleEndian.PutUint32(encap[4:8], 0x01020304)

		cipData := []byte{0xCC, 0x00, 0x00, 0x00, 0xC4, 0x00}
		for _, v := range []int32{10, 20, 30} {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(v))
			cipData = append(cipData, b...)
		}

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

	c, err := Connect(l.Addr().String(), &MockLogger{})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer c.Close()

	vals, err := ReadSlice[int32](c, "TestArray", 3)
	if err != nil {
		t.Fatalf("ReadSlice[int32]() error = %v", err)
	}
	if len(vals) != 3 {
		t.Fatalf("ReadSlice[int32]() len = %d, want 3", len(vals))
	}
	expected := []int32{10, 20, 30}
	for i, v := range vals {
		if v != expected[i] {
			t.Errorf("ReadSlice[int32]()[%d] = %d, want %d", i, v, expected[i])
		}
	}
}
