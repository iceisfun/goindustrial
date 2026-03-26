package runtime

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goeip/pkg/objects/assembly"
)

func TestScheduler_NewScheduler(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	if s.runtime != r {
		t.Error("Scheduler runtime not set correctly")
	}
	if s.stop == nil {
		t.Error("Scheduler stop channel not initialized")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	s.Start()

	// Give it time to start
	time.Sleep(10 * time.Millisecond)

	// Stop should not block or panic
	s.Stop()
}

func TestScheduler_ProcessTick(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create a producer connection
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsProducer:   true,
		Assembly:     &assembly.AssemblyInstance{Data: []byte{1, 2, 3}},
	}

	r.AddConnection(conn)

	// Simulate LastSend being old
	r.mu.Lock()
	conn.LastSend = time.Now().Add(-200 * time.Millisecond)
	r.mu.Unlock()

	s.processTick()

	// Verify LastSend updated
	r.mu.RLock()
	if time.Since(conn.LastSend) > 10*time.Millisecond {
		t.Errorf("LastSend should have been updated (was %v)", conn.LastSend)
	}
	r.mu.RUnlock()
}

func TestScheduler_ProcessTick_NotReady(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create a producer connection
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsProducer:   true,
		Assembly:     &assembly.AssemblyInstance{Data: []byte{1, 2, 3}},
	}

	r.AddConnection(conn)

	// Simulate LastSend being recent
	now := time.Now()
	r.mu.Lock()
	conn.LastSend = now
	r.mu.Unlock()

	s.processTick()

	// Verify LastSend NOT updated
	r.mu.RLock()
	if !conn.LastSend.Equal(now) {
		t.Errorf("LastSend should NOT have been updated")
	}
	r.mu.RUnlock()
}

func TestScheduler_ProcessTick_ConsumerNotProduced(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create a consumer-only connection (not producer)
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsProducer:   false,
		IsConsumer:   true,
		Assembly:     &assembly.AssemblyInstance{Data: []byte{1, 2, 3}},
	}

	r.AddConnection(conn)

	// Set LastSend to old time
	oldTime := time.Now().Add(-1 * time.Second)
	r.mu.Lock()
	conn.LastSend = oldTime
	r.mu.Unlock()

	s.processTick()

	// Consumer connection should NOT be processed by scheduler
	r.mu.RLock()
	if !conn.LastSend.Equal(oldTime) {
		t.Errorf("Consumer connection's LastSend should NOT be updated by scheduler")
	}
	r.mu.RUnlock()
}

func TestScheduler_ProcessTick_NilAssembly(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create a producer connection without assembly
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsProducer:   true,
		Assembly:     nil, // No assembly
	}

	r.AddConnection(conn)

	// Set LastSend to old time
	oldTime := time.Now().Add(-1 * time.Second)
	r.mu.Lock()
	conn.LastSend = oldTime
	r.mu.Unlock()

	// Should not panic
	s.processTick()

	// LastSend should NOT be updated because Assembly is nil
	r.mu.RLock()
	if !conn.LastSend.Equal(oldTime) {
		t.Errorf("Connection with nil Assembly should be skipped")
	}
	r.mu.RUnlock()
}

func TestScheduler_ProcessTick_RPIRespected(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create two producer connections with different RPIs
	conn1 := &IOConnection{
		ConnectionID: 1,
		RPI:          50 * time.Millisecond,
		IsProducer:   true,
		Assembly:     &assembly.AssemblyInstance{Data: []byte{1}},
	}
	conn2 := &IOConnection{
		ConnectionID: 2,
		RPI:          200 * time.Millisecond,
		IsProducer:   true,
		Assembly:     &assembly.AssemblyInstance{Data: []byte{2}},
	}

	r.AddConnection(conn1)
	r.AddConnection(conn2)

	// Set both LastSend to 100ms ago
	r.mu.Lock()
	conn1.LastSend = time.Now().Add(-100 * time.Millisecond)
	conn2.LastSend = time.Now().Add(-100 * time.Millisecond)
	r.mu.Unlock()

	s.processTick()

	r.mu.RLock()
	// conn1 should have been sent (100ms > 50ms RPI)
	if time.Since(conn1.LastSend) > 10*time.Millisecond {
		t.Error("conn1 should have been sent (RPI elapsed)")
	}
	// conn2 should NOT have been sent (100ms < 200ms RPI)
	if time.Since(conn2.LastSend) < 90*time.Millisecond {
		t.Error("conn2 should NOT have been sent (RPI not elapsed)")
	}
	r.mu.RUnlock()
}

func TestScheduler_ProcessTick_MultipleConnections(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create multiple producer connections
	const numConns = 10
	for i := uint32(1); i <= numConns; i++ {
		conn := &IOConnection{
			ConnectionID: i,
			RPI:          50 * time.Millisecond,
			IsProducer:   true,
			Assembly:     &assembly.AssemblyInstance{Data: []byte{byte(i)}},
		}
		r.AddConnection(conn)

		// Set LastSend to old time
		r.mu.Lock()
		conn.LastSend = time.Now().Add(-100 * time.Millisecond)
		r.mu.Unlock()
	}

	s.processTick()

	// All connections should have been sent
	r.mu.RLock()
	for i := uint32(1); i <= numConns; i++ {
		conn := r.connections[i]
		if time.Since(conn.LastSend) > 10*time.Millisecond {
			t.Errorf("Connection %d should have been sent", i)
		}
	}
	r.mu.RUnlock()
}

func TestScheduler_SendPacket_SequenceIncrement(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	conn := &IOConnection{
		ConnectionID:  1,
		RPI:           50 * time.Millisecond,
		IsProducer:    true,
		SequenceCount: 0,
		Assembly:      &assembly.AssemblyInstance{Data: []byte{1, 2, 3, 4}},
		RemoteAddr:    nil, // No actual sending
	}

	r.AddConnection(conn)

	// Call processTick multiple times with old LastSend to trigger sends
	for i := 0; i < 5; i++ {
		r.mu.Lock()
		conn.LastSend = time.Time{}
		r.mu.Unlock()
		s.processTick()
	}

	r.mu.Lock()
	seq := conn.SequenceCount
	r.mu.Unlock()
	if seq != 5 {
		t.Errorf("SequenceCount = %d, want 5", seq)
	}
}

func TestScheduler_SendPacket_SequenceWrap(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	conn := &IOConnection{
		ConnectionID:  1,
		RPI:           50 * time.Millisecond,
		IsProducer:    true,
		SequenceCount: 0xFFFE, // Near max
		Assembly:      &assembly.AssemblyInstance{Data: []byte{1}},
		RemoteAddr:    nil,
	}

	r.AddConnection(conn)

	r.mu.Lock()
	conn.LastSend = time.Time{}
	r.mu.Unlock()
	s.processTick() // 0xFFFF
	r.mu.Lock()
	seq := conn.SequenceCount
	r.mu.Unlock()
	if seq != 0xFFFF {
		t.Errorf("SequenceCount = %d, want 0xFFFF", seq)
	}

	r.mu.Lock()
	conn.LastSend = time.Time{}
	r.mu.Unlock()
	s.processTick() // 0x0000 (wrap)
	r.mu.Lock()
	seq = conn.SequenceCount
	r.mu.Unlock()
	if seq != 0x0000 {
		t.Errorf("SequenceCount = %d, want 0x0000 (wrapped)", seq)
	}
}

func TestScheduler_SendPacket_WithRunIdleHeader(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	// Set up a UDP listener to capture the packet
	serverAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve address: %v", err)
	}
	serverConn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer serverConn.Close()

	// Get the actual port
	actualAddr := serverConn.LocalAddr().(*net.UDPAddr)

	// Set the runtime's UDP connection
	clientAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	clientConn, _ := net.ListenUDP("udp", clientAddr)
	r.conn = clientConn
	defer clientConn.Close()

	s := NewScheduler(r)

	conn := &IOConnection{
		ConnectionID:  0x12345678,
		RPI:           50 * time.Millisecond,
		IsProducer:    true,
		SequenceCount: 100,
		RunIdleHeader: true,
		Assembly:      &assembly.AssemblyInstance{Data: []byte{0xAA, 0xBB, 0xCC, 0xDD}},
		RemoteAddr:    actualAddr,
	}

	r.AddConnection(conn)

	// Channel to receive packet
	packetChan := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, _ := serverConn.ReadFromUDP(buf)
		packetChan <- buf[:n]
	}()

	r.mu.Lock()
	conn.LastSend = time.Time{}
	r.mu.Unlock()
	s.processTick()

	select {
	case packet := <-packetChan:
		// Packet structure:
		// Offset 0-1: Item Count (2)
		// Offset 2-3: Address Type (0x00A1)
		// Offset 4-5: Address Length (4)
		// Offset 6-9: Connection ID
		// Offset 10-11: Data Type (0x00B1)
		// Offset 12-13: Data Length
		// Offset 14-15: Sequence Count
		// Offset 16-19: Run/Idle Header
		// Offset 20+: Assembly Data

		expectedLen := 2 + 2 + 2 + 4 + 2 + 2 + 2 + 4 + 4 // 24 bytes
		if len(packet) < expectedLen {
			t.Fatalf("Packet too short: %d bytes, want at least %d", len(packet), expectedLen)
		}

		// Check item count
		itemCount := binary.LittleEndian.Uint16(packet[0:2])
		if itemCount != 2 {
			t.Errorf("ItemCount = %d, want 2", itemCount)
		}

		// Check address item type (0x00A1)
		addrType := binary.LittleEndian.Uint16(packet[2:4])
		if addrType != 0x00A1 {
			t.Errorf("Address item type = 0x%04X, want 0x00A1", addrType)
		}

		// Check address length
		addrLen := binary.LittleEndian.Uint16(packet[4:6])
		if addrLen != 4 {
			t.Errorf("Address length = %d, want 4", addrLen)
		}

		// Check connection ID (offset 6-9)
		connID := binary.LittleEndian.Uint32(packet[6:10])
		if connID != 0x12345678 {
			t.Errorf("ConnectionID = 0x%08X, want 0x12345678", connID)
		}

		// Check data item type (0x00B1) at offset 10-11
		dataType := binary.LittleEndian.Uint16(packet[10:12])
		if dataType != 0x00B1 {
			t.Errorf("Data item type = 0x%04X, want 0x00B1", dataType)
		}

		// Check data length at offset 12-13 (should be 2 + 4 + 4 = 10)
		dataLen := binary.LittleEndian.Uint16(packet[12:14])
		if dataLen != 10 {
			t.Errorf("Data length = %d, want 10 (seq + runIdle + data)", dataLen)
		}

		// Check sequence count at offset 14-15 (should be 101 after increment)
		seqCount := binary.LittleEndian.Uint16(packet[14:16])
		if seqCount != 101 {
			t.Errorf("SequenceCount = %d, want 101", seqCount)
		}

		// Check Run/Idle header at offset 16-19 (should be 1 for Run)
		runIdle := binary.LittleEndian.Uint32(packet[16:20])
		if runIdle != 1 {
			t.Errorf("RunIdleHeader = %d, want 1", runIdle)
		}

		// Check assembly data at offset 20+
		if packet[20] != 0xAA || packet[21] != 0xBB || packet[22] != 0xCC || packet[23] != 0xDD {
			t.Errorf("Assembly data mismatch: got %v", packet[20:24])
		}

	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for packet")
	}
}

func TestScheduler_SendPacket_NoRemoteAddr(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	conn := &IOConnection{
		ConnectionID:  1,
		RPI:           50 * time.Millisecond,
		IsProducer:    true,
		SequenceCount: 0,
		Assembly:      &assembly.AssemblyInstance{Data: []byte{1}},
		RemoteAddr:    nil, // No remote address
	}

	r.AddConnection(conn)

	// Should not panic even with nil RemoteAddr
	r.mu.Lock()
	conn.LastSend = time.Time{}
	r.mu.Unlock()
	s.processTick()

	// Sequence should still increment
	r.mu.Lock()
	seq := conn.SequenceCount
	r.mu.Unlock()
	if seq != 1 {
		t.Errorf("SequenceCount = %d, want 1", seq)
	}
}

func TestScheduler_ConcurrentProcessTick(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)
	s := NewScheduler(r)

	// Create connections
	for i := uint32(1); i <= 5; i++ {
		conn := &IOConnection{
			ConnectionID: i,
			RPI:          10 * time.Millisecond,
			IsProducer:   true,
			Assembly:     &assembly.AssemblyInstance{Data: []byte{byte(i)}},
		}
		r.AddConnection(conn)
	}

	// Run multiple processTick concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				s.processTick()
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Should complete without deadlock or panic
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Concurrent processTick deadlocked")
	}
}
