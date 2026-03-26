package runtime

import (
	"encoding/binary"
	"net"
	"sync"
	"time"
)

// connSnapshot holds a copy of IOConnection fields needed for sending a packet.
type connSnapshot struct {
	ConnectionID  uint32
	SequenceCount uint16
	RunIdleHeader bool
	RemoteAddr    *net.UDPAddr
	AssemblyData  []byte
}

// Scheduler manages the RPI (Requested Packet Interval) for producing connections
type Scheduler struct {
	runtime  *Runtime
	stop     chan struct{}
	stopOnce sync.Once
}

// NewScheduler creates a new Scheduler
func NewScheduler(r *Runtime) *Scheduler {
	return &Scheduler{
		runtime: r,
		stop:    make(chan struct{}),
	}
}

// Start starts the scheduler loop
func (s *Scheduler) Start() {
	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}

// run is the main loop
func (s *Scheduler) run() {
	ticker := time.NewTicker(5 * time.Millisecond) // Base tick, or use dynamic
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.processTick()
		}
	}
}

// processTick checks all connections and sends data if RPI has elapsed
func (s *Scheduler) processTick() {
	now := time.Now()

	s.runtime.mu.Lock()
	// Collect snapshots of connections that need sending, update their state under lock.
	var snapshots []connSnapshot
	for _, conn := range s.runtime.connections {
		if !conn.IsProducer {
			continue
		}
		if now.Sub(conn.LastSend) < conn.RPI {
			continue
		}
		if conn.Assembly == nil {
			continue
		}

		conn.SequenceCount++
		conn.LastSend = now

		// Copy assembly data under lock
		dataCopy := make([]byte, len(conn.Assembly.Data))
		copy(dataCopy, conn.Assembly.Data)

		snapshots = append(snapshots, connSnapshot{
			ConnectionID:  conn.ConnectionID,
			SequenceCount: conn.SequenceCount,
			RunIdleHeader: conn.RunIdleHeader,
			RemoteAddr:    conn.RemoteAddr,
			AssemblyData:  dataCopy,
		})
	}
	s.runtime.mu.Unlock()

	// Send packets outside the lock
	for i := range snapshots {
		s.sendPacketFromSnapshot(&snapshots[i])
	}
}

func (s *Scheduler) sendPacketFromSnapshot(snap *connSnapshot) {
	if snap.RemoteAddr == nil {
		return
	}

	buf := make([]byte, 2048)
	offset := 0

	// Item Count
	binary.LittleEndian.PutUint16(buf[offset:], 2)
	offset += 2

	// Item 1: Address
	binary.LittleEndian.PutUint16(buf[offset:], 0x00A1) // Connected Address Item
	offset += 2
	binary.LittleEndian.PutUint16(buf[offset:], 4) // Length
	offset += 2
	binary.LittleEndian.PutUint32(buf[offset:], snap.ConnectionID)
	offset += 4

	// Item 2: Data Item
	binary.LittleEndian.PutUint16(buf[offset:], 0x00B1) // Connected Data Item
	offset += 2

	// Calculate Data Length
	// Sequence (2) + Header (0 or 4) + Data
	dataLen := 2
	if snap.RunIdleHeader {
		dataLen += 4
	}
	dataLen += len(snap.AssemblyData)

	binary.LittleEndian.PutUint16(buf[offset:], uint16(dataLen))
	offset += 2

	// Sequence Count
	binary.LittleEndian.PutUint16(buf[offset:], snap.SequenceCount)
	offset += 2

	// Run/Idle Header
	if snap.RunIdleHeader {
		binary.LittleEndian.PutUint32(buf[offset:], 1)
		offset += 4
	}

	// Data
	copy(buf[offset:], snap.AssemblyData)
	offset += len(snap.AssemblyData)

	// Send
	s.runtime.conn.WriteToUDP(buf[:offset], snap.RemoteAddr)
}
