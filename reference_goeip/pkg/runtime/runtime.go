package runtime

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/iceisfun/goeip/pkg/objects/assembly"
)

// IOConnection represents a cyclic I/O connection
type IOConnection struct {
	ConnectionID  uint32
	RPI           time.Duration
	SequenceCount uint16 // 16-bit sequence count for Class 1
	RunIdleHeader bool   // True if 32-bit Run/Idle header is used
	RemoteAddr    *net.UDPAddr
	Assembly      *assembly.AssemblyInstance // The assembly to consume/produce
	LastReceive   time.Time
	LastSend      time.Time
	TimeoutMult   uint8
	IsProducer    bool
	IsConsumer    bool
	StopChan      chan struct{}
}

// Runtime manages the UDP server and I/O connections
type Runtime struct {
	mu          sync.RWMutex
	conn        *net.UDPConn
	connections map[uint32]*IOConnection // Map by ConnectionID (Consuming ID)
	assemblyObj *assembly.AssemblyObject
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewRuntime creates a new Runtime
func NewRuntime(ao *assembly.AssemblyObject) *Runtime {
	return &Runtime{
		connections: make(map[uint32]*IOConnection),
		assemblyObj: ao,
	}
}

// Start starts the UDP listener on port 2222
func (r *Runtime) Start(address string) error {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	r.conn = conn
	r.done = make(chan struct{})

	r.wg.Add(2)
	go func() {
		defer r.wg.Done()
		r.listenLoop()
	}()
	go func() {
		defer r.wg.Done()
		r.watchdogLoop()
	}()

	return nil
}

// AddConnection adds a connection to the runtime
func (r *Runtime) AddConnection(conn *IOConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()
	conn.LastReceive = time.Now()
	r.connections[conn.ConnectionID] = conn
}

// RemoveConnection removes a connection from the runtime
func (r *Runtime) RemoveConnection(connID uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connections, connID)
}

// watchdogLoop checks for connection timeouts
func (r *Runtime) watchdogLoop() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.checkTimeouts()
		}
	}
}

func (r *Runtime) checkTimeouts() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for id, conn := range r.connections {
		if !conn.IsConsumer {
			continue
		}

		// Calculate timeout duration
		// RPI (Duration) * Multiplier (4 bits usually, but we stored as uint8)
		// 0 = x4, 1 = x8, 2 = x16, 3 = x32 ... wait, spec says:
		// 0 = x4, 1 = x8, 2 = x16, 3 = x32, 4 = x64, 5 = x128, 6 = x256, 7 = x512
		// Actually, Connection Timeout Multiplier is:
		// 0: x4
		// 1: x8
		// 2: x16
		// 3: x32
		// ...
		// But usually it's just 4 * RPI.
		// Let's assume Multiplier is the raw value from Forward_Open (0-7).

		mult := uint64(4) << conn.TimeoutMult
		timeout := conn.RPI * time.Duration(mult)

		if now.Sub(conn.LastReceive) > timeout {
			// Timeout!
			// Log it?
			// Remove connection?
			delete(r.connections, id)
			// Also notify Connection Manager?
			// For now just remove from runtime.
		}
	}
}

// listenLoop handles incoming UDP packets
func (r *Runtime) listenLoop() {
	buf := make([]byte, 2048) // Max CIP packet size is usually small
	for {
		n, remoteAddr, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-r.done:
				return
			default:
			}
			// Log error or exit
			return
		}

		r.handlePacket(buf[:n], remoteAddr)
	}
}

// Stop stops the runtime, closing the UDP connection and waiting for goroutines to exit
func (r *Runtime) Stop() {
	close(r.done)
	r.conn.Close()
	r.wg.Wait()
}

// handlePacket processes a single UDP packet
func (r *Runtime) handlePacket(data []byte, remoteAddr *net.UDPAddr) {
	// Packet format:
	// Item Count (UINT) [2]
	// Item 1: Type (UINT) [2] + Length (UINT) [2] + Connection ID (UDINT) [4]
	// Item 2: Type (UINT) [2] + Length (UINT) [2] + Data...
	// Minimum size: 2 + 2 + 2 + 4 + 2 + 2 = 14 bytes (with len1 == 4)

	if len(data) < 14 {
		return
	}

	itemCount := binary.LittleEndian.Uint16(data[0:2])
	if itemCount != 2 {
		return
	}

	// Item 1: Address Item
	// Type (UINT)
	// Length (UINT)
	// Connection ID (UDINT) - if Length == 4

	offset := 2
	// type1 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	len1 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len1 != 4 {
		// Only support Connected Address Item for now
		return
	}

	connID := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Item 2: Data Item
	// Type (UINT)
	// Length (UINT)
	// Data...

	// type2 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2
	len2 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(len2) {
		return
	}

	payload := data[offset : offset+int(len2)]

	// Use a single write lock for lookup, LastReceive update, and reading conn fields.
	r.mu.Lock()
	conn, ok := r.connections[connID]
	if !ok {
		r.mu.Unlock()
		return
	}
	conn.LastReceive = time.Now()

	// Copy fields needed after unlock
	runIdleHeader := conn.RunIdleHeader
	asm := conn.Assembly
	r.mu.Unlock()

	// Handle Run/Idle Header if present
	dataOffset := 0
	if runIdleHeader {
		if len(payload) < 4 {
			return
		}
		// header := binary.LittleEndian.Uint32(payload[0:4])
		// Check Run bit (Bit 0)
		dataOffset = 4
	}

	// Update Assembly Data
	if asm != nil {
		r.assemblyObj.SetAttributeSingle(asm.ID, 3, payload[dataOffset:])
	}
}
