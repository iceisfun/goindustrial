package runtime

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/iceisfun/goeip/pkg/objects/assembly"
)

func TestRuntime_AddConnection(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	connID := uint32(0x12345678)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsConsumer:   true,
	}

	r.AddConnection(conn)

	r.mu.RLock()
	defer r.mu.RUnlock()

	storedConn, ok := r.connections[connID]
	if !ok {
		t.Fatal("Connection was not added to runtime")
	}

	if storedConn.ConnectionID != connID {
		t.Errorf("Expected ConnectionID %d, got %d", connID, storedConn.ConnectionID)
	}

	// Verify LastReceive was initialized
	if storedConn.LastReceive.IsZero() {
		t.Error("LastReceive should be initialized to current time, not zero")
	}

	// LastReceive should be very recent (within 1 second)
	if time.Since(storedConn.LastReceive) > time.Second {
		t.Error("LastReceive should be initialized to approximately now")
	}
}

func TestRuntime_RemoveConnection(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		IsConsumer:   true,
	}

	r.AddConnection(conn)
	r.RemoveConnection(connID)

	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.connections[connID]; ok {
		t.Error("Connection should have been removed")
	}
}

func TestRuntime_CheckTimeouts(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	// Create a consumer connection
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsConsumer:   true,
		TimeoutMult:  0, // x4 = 400ms
	}

	r.AddConnection(conn)

	// Verify connection added
	r.mu.RLock()
	if _, ok := r.connections[connID]; !ok {
		t.Fatalf("Connection not added")
	}
	r.mu.RUnlock()

	// Simulate time passing (LastReceive was set to Now() in AddConnection)
	// We need to manipulate LastReceive to simulate timeout
	r.mu.Lock()
	conn.LastReceive = time.Now().Add(-500 * time.Millisecond)
	r.mu.Unlock()

	// Check timeouts
	r.checkTimeouts()

	// Verify connection removed
	r.mu.RLock()
	if _, ok := r.connections[connID]; ok {
		t.Errorf("Connection should have timed out")
	}
	r.mu.RUnlock()
}

func TestRuntime_CheckTimeouts_NoTimeout(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	// Create a consumer connection
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsConsumer:   true,
		TimeoutMult:  0, // x4 = 400ms
	}

	r.AddConnection(conn)

	// Simulate time passing (less than timeout)
	r.mu.Lock()
	conn.LastReceive = time.Now().Add(-200 * time.Millisecond)
	r.mu.Unlock()

	// Check timeouts
	r.checkTimeouts()

	// Verify connection still exists
	r.mu.RLock()
	if _, ok := r.connections[connID]; !ok {
		t.Errorf("Connection should NOT have timed out")
	}
	r.mu.RUnlock()
}

func TestRuntime_CheckTimeouts_ProducerNotChecked(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	// Create a producer-only connection (not consumer)
	connID := uint32(1)
	conn := &IOConnection{
		ConnectionID: connID,
		RPI:          100 * time.Millisecond,
		IsProducer:   true,
		IsConsumer:   false, // Not a consumer, so timeout shouldn't apply
		TimeoutMult:  0,
	}

	r.AddConnection(conn)

	// Simulate LastReceive being very old
	r.mu.Lock()
	conn.LastReceive = time.Now().Add(-10 * time.Second)
	r.mu.Unlock()

	// Check timeouts
	r.checkTimeouts()

	// Producer connection should NOT be removed (timeout only applies to consumers)
	r.mu.RLock()
	if _, ok := r.connections[connID]; !ok {
		t.Errorf("Producer connection should NOT be timed out")
	}
	r.mu.RUnlock()
}

func TestRuntime_CheckTimeouts_DifferentMultipliers(t *testing.T) {
	tests := []struct {
		name        string
		mult        uint8
		rpi         time.Duration
		elapsed     time.Duration
		shouldExist bool
	}{
		{
			name:        "Mult 0 (x4), within timeout",
			mult:        0,
			rpi:         100 * time.Millisecond,
			elapsed:     350 * time.Millisecond, // < 400ms
			shouldExist: true,
		},
		{
			name:        "Mult 0 (x4), past timeout",
			mult:        0,
			rpi:         100 * time.Millisecond,
			elapsed:     450 * time.Millisecond, // > 400ms
			shouldExist: false,
		},
		{
			name:        "Mult 1 (x8), within timeout",
			mult:        1,
			rpi:         100 * time.Millisecond,
			elapsed:     700 * time.Millisecond, // < 800ms
			shouldExist: true,
		},
		{
			name:        "Mult 1 (x8), past timeout",
			mult:        1,
			rpi:         100 * time.Millisecond,
			elapsed:     900 * time.Millisecond, // > 800ms
			shouldExist: false,
		},
		{
			name:        "Mult 2 (x16), within timeout",
			mult:        2,
			rpi:         50 * time.Millisecond,
			elapsed:     700 * time.Millisecond, // < 800ms (50ms * 16)
			shouldExist: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ao := assembly.NewAssemblyObject()
			r := NewRuntime(ao)

			connID := uint32(1)
			conn := &IOConnection{
				ConnectionID: connID,
				RPI:          tt.rpi,
				IsConsumer:   true,
				TimeoutMult:  tt.mult,
			}

			r.AddConnection(conn)

			r.mu.Lock()
			conn.LastReceive = time.Now().Add(-tt.elapsed)
			r.mu.Unlock()

			r.checkTimeouts()

			r.mu.RLock()
			_, exists := r.connections[connID]
			r.mu.RUnlock()

			if exists != tt.shouldExist {
				t.Errorf("Connection exists=%v, want %v", exists, tt.shouldExist)
			}
		})
	}
}

func TestRuntime_HandlePacket_ValidPacket(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 4)) // 4-byte assembly

	r := NewRuntime(ao)

	// Create and add a consumer connection
	connID := uint32(0x12345678)
	assemblyInst := &assembly.AssemblyInstance{ID: 100, Data: make([]byte, 4)}
	conn := &IOConnection{
		ConnectionID:  connID,
		RPI:           100 * time.Millisecond,
		IsConsumer:    true,
		RunIdleHeader: false,
		Assembly:      assemblyInst,
	}
	r.AddConnection(conn)

	// Set LastReceive to old time to verify it gets updated
	r.mu.Lock()
	oldTime := time.Now().Add(-1 * time.Second)
	conn.LastReceive = oldTime
	r.mu.Unlock()

	// Build a valid I/O packet
	// Format: ItemCount(2) + AddressItem(TypeID:2 + Len:2 + ConnID:4) + DataItem(TypeID:2 + Len:2 + Data)
	packet := make([]byte, 0, 64)

	// Item Count = 2
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 2)
	packet = append(packet, itemCount...)

	// Address Item (0x00A1 Connected Address Item)
	addrType := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrType, 0x00A1)
	packet = append(packet, addrType...)

	addrLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrLen, 4) // Connection ID is 4 bytes
	packet = append(packet, addrLen...)

	connIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(connIDBytes, connID)
	packet = append(packet, connIDBytes...)

	// Data Item (0x00B1 Connected Data Item)
	dataType := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataType, 0x00B1)
	packet = append(packet, dataType...)

	// Data: 4 bytes of assembly data
	assemblyData := []byte{0x01, 0x02, 0x03, 0x04}
	dataLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataLen, uint16(len(assemblyData)))
	packet = append(packet, dataLen...)
	packet = append(packet, assemblyData...)

	// Handle the packet
	remoteAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 2222}
	r.handlePacket(packet, remoteAddr)

	// Verify LastReceive was updated
	r.mu.RLock()
	if !conn.LastReceive.After(oldTime) {
		t.Error("LastReceive should have been updated")
	}
	r.mu.RUnlock()

	// Verify assembly data was updated
	data, err := ao.GetAttributeSingle(100, 3)
	if err != nil {
		t.Fatalf("Failed to get assembly data: %v", err)
	}
	for i, b := range assemblyData {
		if data[i] != b {
			t.Errorf("Assembly data[%d] = %d, want %d", i, data[i], b)
		}
	}
}

func TestRuntime_HandlePacket_UnknownConnectionID(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	// Don't add any connection - packet should be ignored

	// Build a packet with unknown connection ID
	packet := buildIOPacket(0xDEADBEEF, []byte{0x01, 0x02, 0x03, 0x04})

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 2222}

	// Should not panic
	r.handlePacket(packet, remoteAddr)
}

func TestRuntime_HandlePacket_TooShort(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 2222}

	// Test various short packets
	shortPackets := [][]byte{
		{},                             // Empty
		{0x00},                         // 1 byte
		{0x00, 0x00},                   // 2 bytes (just item count)
		{0x02, 0x00, 0x00, 0x00},       // 4 bytes
		{0x02, 0x00, 0x00, 0x00, 0x00}, // 5 bytes
	}

	for i, packet := range shortPackets {
		// Should not panic
		r.handlePacket(packet, remoteAddr)
		t.Logf("Short packet %d handled without panic", i)
	}
}

func TestRuntime_HandlePacket_WrongItemCount(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	r := NewRuntime(ao)

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 2222}

	// Packet with item count != 2
	packet := make([]byte, 20)
	binary.LittleEndian.PutUint16(packet[0:], 1) // Only 1 item (wrong)

	// Should return early without processing
	r.handlePacket(packet, remoteAddr)
}

func TestRuntime_HandlePacket_WithRunIdleHeader(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 4))

	r := NewRuntime(ao)

	connID := uint32(0x12345678)
	assemblyInst := &assembly.AssemblyInstance{ID: 100, Data: make([]byte, 4)}
	conn := &IOConnection{
		ConnectionID:  connID,
		RPI:           100 * time.Millisecond,
		IsConsumer:    true,
		RunIdleHeader: true, // Has 4-byte Run/Idle header
		Assembly:      assemblyInst,
	}
	r.AddConnection(conn)

	// Build packet with Run/Idle header
	packet := make([]byte, 0, 64)

	// Item Count = 2
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 2)
	packet = append(packet, itemCount...)

	// Address Item
	addrType := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrType, 0x00A1)
	packet = append(packet, addrType...)
	addrLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrLen, 4)
	packet = append(packet, addrLen...)
	connIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(connIDBytes, connID)
	packet = append(packet, connIDBytes...)

	// Data Item with Run/Idle header + assembly data
	dataType := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataType, 0x00B1)
	packet = append(packet, dataType...)

	runIdleHeader := make([]byte, 4)
	binary.LittleEndian.PutUint32(runIdleHeader, 0x00000001) // Run mode
	assemblyData := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	dataLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataLen, uint16(len(runIdleHeader)+len(assemblyData)))
	packet = append(packet, dataLen...)
	packet = append(packet, runIdleHeader...)
	packet = append(packet, assemblyData...)

	remoteAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.100"), Port: 2222}
	r.handlePacket(packet, remoteAddr)

	// Verify assembly data was updated (Run/Idle header skipped)
	data, err := ao.GetAttributeSingle(100, 3)
	if err != nil {
		t.Fatalf("Failed to get assembly data: %v", err)
	}
	for i, b := range assemblyData {
		if data[i] != b {
			t.Errorf("Assembly data[%d] = 0x%02X, want 0x%02X", i, data[i], b)
		}
	}
}

// Helper function to build an I/O packet
func buildIOPacket(connID uint32, data []byte) []byte {
	packet := make([]byte, 0, 64)

	// Item Count = 2
	itemCount := make([]byte, 2)
	binary.LittleEndian.PutUint16(itemCount, 2)
	packet = append(packet, itemCount...)

	// Address Item (0x00A1)
	addrType := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrType, 0x00A1)
	packet = append(packet, addrType...)
	addrLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(addrLen, 4)
	packet = append(packet, addrLen...)
	connIDBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(connIDBytes, connID)
	packet = append(packet, connIDBytes...)

	// Data Item (0x00B1)
	dataType := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataType, 0x00B1)
	packet = append(packet, dataType...)
	dataLen := make([]byte, 2)
	binary.LittleEndian.PutUint16(dataLen, uint16(len(data)))
	packet = append(packet, dataLen...)
	packet = append(packet, data...)

	return packet
}
