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

// Scheduler drives the production side of implicit I/O messaging. It polls
// all producer connections registered with a Runtime, and when a connection's
// Requested Packet Interval (RPI) has elapsed it reads the assembly data and
// sends a UDP packet to the remote scanner. The scheduler runs in its own
// goroutine; call Start to launch it and Stop to shut it down.
type Scheduler struct {
	runtime  *Runtime
	stop     chan struct{}
	stopOnce sync.Once
}

// NewScheduler creates a new Scheduler bound to the given Runtime. The
// scheduler is not started until Start is called.
func NewScheduler(r *Runtime) *Scheduler {
	return &Scheduler{
		runtime: r,
		stop:    make(chan struct{}),
	}
}

// Start launches the scheduler goroutine, which checks producer connections
// every 5 ms and sends I/O packets when each connection's RPI has elapsed.
func (s *Scheduler) Start() {
	go s.run()
}

// Stop signals the scheduler goroutine to exit. It is safe to call multiple
// times.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
	})
}

// run is the main loop
func (s *Scheduler) run() {
	ticker := time.NewTicker(5 * time.Millisecond)
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

		dataCopy, err := s.runtime.assemblyObj.GetAttributeSingle(conn.Assembly.ID, 3)
		if err != nil {
			continue
		}

		snapshots = append(snapshots, connSnapshot{
			ConnectionID:  conn.ConnectionID,
			SequenceCount: conn.SequenceCount,
			RunIdleHeader: conn.RunIdleHeader,
			RemoteAddr:    conn.RemoteAddr,
			AssemblyData:  dataCopy,
		})
	}
	s.runtime.mu.Unlock()

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
