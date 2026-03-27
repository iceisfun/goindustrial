package runtime

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
)

func TestAddRemoveConnection(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	rt := NewRuntime(ao)

	rt.AddConnection(&IOConnection{ConnectionID: 1, StopChan: make(chan struct{})})
	rt.AddConnection(&IOConnection{ConnectionID: 2, StopChan: make(chan struct{})})
	rt.AddConnection(&IOConnection{ConnectionID: 3, StopChan: make(chan struct{})})

	rt.mu.RLock()
	if len(rt.connections) != 3 {
		t.Errorf("connections = %d, want 3", len(rt.connections))
	}
	rt.mu.RUnlock()

	rt.RemoveConnection(2)

	rt.mu.RLock()
	if len(rt.connections) != 2 {
		t.Errorf("connections after remove = %d, want 2", len(rt.connections))
	}
	if _, ok := rt.connections[2]; ok {
		t.Error("connection 2 should have been removed")
	}
	rt.mu.RUnlock()

	rt.RemoveConnection(1)
	rt.RemoveConnection(3)

	rt.mu.RLock()
	if len(rt.connections) != 0 {
		t.Errorf("connections after remove all = %d, want 0", len(rt.connections))
	}
	rt.mu.RUnlock()
}

func TestRemoveConnectionClosesStopChan(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	rt := NewRuntime(ao)

	stop := make(chan struct{})
	rt.AddConnection(&IOConnection{ConnectionID: 42, StopChan: stop})

	rt.RemoveConnection(42)

	select {
	case <-stop:
		// ok, closed
	case <-time.After(time.Second):
		t.Fatal("StopChan was not closed after RemoveConnection")
	}
}

func TestRemoveNonexistentConnection(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	rt := NewRuntime(ao)

	// Should not panic.
	rt.RemoveConnection(999)
}

func TestTimeoutClosesStopChan(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	rt := NewRuntime(ao)
	if err := rt.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { rt.Stop() })

	stop := make(chan struct{})
	rt.AddConnection(&IOConnection{
		ConnectionID: 77,
		RPI:          10 * time.Millisecond,
		IsConsumer:   true,
		TimeoutMult:  0, // timeout = 10ms * (4 << 0) = 40ms
		StopChan:     stop,
	})

	// Backdate LastReceive so the watchdog fires quickly.
	rt.mu.Lock()
	rt.connections[77].LastReceive = time.Now().Add(-200 * time.Millisecond)
	rt.mu.Unlock()

	select {
	case <-stop:
		// ok, timed out and closed
	case <-time.After(2 * time.Second):
		t.Fatal("StopChan was not closed by watchdog timeout")
	}

	rt.mu.RLock()
	_, exists := rt.connections[77]
	rt.mu.RUnlock()
	if exists {
		t.Error("timed-out connection should have been removed from the map")
	}
}

// buildClass1CPFPacket builds a CPF (Common Packet Format) packet suitable for
// the runtime's handlePacket parser: 2 items, address item with connection ID,
// data item with 2-byte sequence count followed by payload.
func buildClass1CPFPacket(connID uint32, seqCount uint16, payload []byte) []byte {
	buf := make([]byte, 2048)
	offset := 0

	// Item Count
	binary.LittleEndian.PutUint16(buf[offset:], 2)
	offset += 2

	// Item 1: Connected Address Item (type 0x00A1)
	binary.LittleEndian.PutUint16(buf[offset:], 0x00A1)
	offset += 2
	binary.LittleEndian.PutUint16(buf[offset:], 4) // length
	offset += 2
	binary.LittleEndian.PutUint32(buf[offset:], connID)
	offset += 4

	// Item 2: Connected Data Item (type 0x00B1)
	binary.LittleEndian.PutUint16(buf[offset:], 0x00B1)
	offset += 2
	dataLen := 2 + len(payload) // 2-byte seq count + payload
	binary.LittleEndian.PutUint16(buf[offset:], uint16(dataLen))
	offset += 2

	// Sequence count
	binary.LittleEndian.PutUint16(buf[offset:], seqCount)
	offset += 2

	// Payload (assembly data)
	copy(buf[offset:], payload)
	offset += len(payload)

	return buf[:offset]
}

func TestHandlePacket(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 4))

	rt := NewRuntime(ao)
	if err := rt.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { rt.Stop() })

	runtimeAddr := rt.Addr()

	// Add a consumer connection.
	rt.AddConnection(&IOConnection{
		ConnectionID: 0xAA,
		RPI:          time.Second,
		IsConsumer:   true,
		TimeoutMult:  3,
		Assembly:     ao.GetInstance(100),
		StopChan:     make(chan struct{}),
	})

	// Build and send a Class 1 packet.
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	pkt := buildClass1CPFPacket(0xAA, 1, payload)

	sender, err := net.DialUDP("udp", nil, runtimeAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer sender.Close()

	if _, err := sender.Write(pkt); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Wait for the runtime to process the packet.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := ao.GetAttributeSingle(100, 3)
		if err == nil && bytes.Equal(data, payload) {
			return // success
		}
		time.Sleep(5 * time.Millisecond)
	}

	data, _ := ao.GetAttributeSingle(100, 3)
	t.Fatalf("assembly data = %X, want %X", data, payload)
}

func TestHandlePacketWithRunIdleHeader(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(200, make([]byte, 4))

	rt := NewRuntime(ao)
	if err := rt.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { rt.Stop() })

	runtimeAddr := rt.Addr()

	rt.AddConnection(&IOConnection{
		ConnectionID:  0xBB,
		RPI:           time.Second,
		IsConsumer:    true,
		TimeoutMult:   3,
		Assembly:      ao.GetInstance(200),
		RunIdleHeader: true,
		StopChan:      make(chan struct{}),
	})

	// Build packet with Run/Idle header (4 extra bytes after sequence count).
	assemblyData := []byte{0xCA, 0xFE, 0xBA, 0xBE}
	runIdlePayload := make([]byte, 4+len(assemblyData))
	binary.LittleEndian.PutUint32(runIdlePayload[0:4], 1) // Run mode
	copy(runIdlePayload[4:], assemblyData)

	pkt := buildClass1CPFPacket(0xBB, 1, runIdlePayload)

	sender, err := net.DialUDP("udp", nil, runtimeAddr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer sender.Close()

	if _, err := sender.Write(pkt); err != nil {
		t.Fatalf("Write: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := ao.GetAttributeSingle(200, 3)
		if err == nil && bytes.Equal(data, assemblyData) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	data, _ := ao.GetAttributeSingle(200, 3)
	t.Fatalf("assembly data = %X, want %X", data, assemblyData)
}

func TestSchedulerProducesTick(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(101, []byte{0x01, 0x02, 0x03, 0x04})

	rt := NewRuntime(ao)
	if err := rt.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { rt.Stop() })

	// Open a UDP listener to receive produced packets.
	recvAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	recvConn, err := net.ListenUDP("udp", recvAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer recvConn.Close()
	recvConn.SetReadDeadline(time.Now().Add(5 * time.Second))

	targetAddr := recvConn.LocalAddr().(*net.UDPAddr)

	// Add a producer connection.
	rt.AddConnection(&IOConnection{
		ConnectionID: 0xCC,
		RPI:          10 * time.Millisecond,
		IsProducer:   true,
		Assembly:     ao.GetInstance(101),
		RemoteAddr:   targetAddr,
		StopChan:     make(chan struct{}),
	})

	sched := NewScheduler(rt)
	sched.Start()
	t.Cleanup(func() { sched.Stop() })

	// Read at least one packet.
	buf := make([]byte, 2048)
	n, _, err := recvConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP: %v", err)
	}

	if n < 14 {
		t.Fatalf("packet too short: %d bytes", n)
	}

	// Parse the CPF packet.
	data := buf[:n]
	itemCount := binary.LittleEndian.Uint16(data[0:2])
	if itemCount != 2 {
		t.Fatalf("item count = %d, want 2", itemCount)
	}

	// Skip address item header (type + len = 4 bytes) and connection ID (4 bytes).
	connID := binary.LittleEndian.Uint32(data[6:10])
	if connID != 0xCC {
		t.Errorf("connection ID = 0x%08X, want 0xCC", connID)
	}

	// Data item starts at offset 10: type(2) + len(2) + seqcount(2) + data.
	dataItemLen := binary.LittleEndian.Uint16(data[12:14])
	if dataItemLen < 6 { // 2 seq + 4 assembly data
		t.Fatalf("data item len = %d, want >= 6", dataItemLen)
	}

	// Sequence count at offset 14.
	seqCount := binary.LittleEndian.Uint16(data[14:16])
	if seqCount == 0 {
		t.Error("sequence count should not be 0")
	}

	// Assembly data starts at offset 16.
	assemblyData := data[16 : 16+4]
	if !bytes.Equal(assemblyData, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("assembly data = %X, want 01020304", assemblyData)
	}
}
