package runtime

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
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

// Start starts the UDP listener on the given address (e.g. ":2222")
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

// RemoveConnection removes a connection from the runtime and signals its
// StopChan so that any associated goroutine exits.
func (r *Runtime) RemoveConnection(connID uint32) {
	r.mu.Lock()
	conn := r.connections[connID]
	delete(r.connections, connID)
	r.mu.Unlock()

	if conn != nil && conn.StopChan != nil {
		select {
		case <-conn.StopChan:
		default:
			close(conn.StopChan)
		}
	}
}

// Addr returns the local UDP address the runtime is listening on.
// Useful for tests to discover the ephemeral port.
func (r *Runtime) Addr() *net.UDPAddr {
	if r.conn != nil {
		return r.conn.LocalAddr().(*net.UDPAddr)
	}
	return nil
}

// SetProducerAddr sets the remote UDP address for a producer connection.
// This tells the scheduler where to send packets for this connection.
func (r *Runtime) SetProducerAddr(connID uint32, addr *net.UDPAddr) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn, ok := r.connections[connID]; ok {
		conn.RemoteAddr = addr
	}
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

		mult := uint64(4) << conn.TimeoutMult
		timeout := conn.RPI * time.Duration(mult)

		if now.Sub(conn.LastReceive) > timeout {
			if conn.StopChan != nil {
				select {
				case <-conn.StopChan:
				default:
					close(conn.StopChan)
				}
			}
			delete(r.connections, id)
		}
	}
}

// listenLoop handles incoming UDP packets
func (r *Runtime) listenLoop() {
	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-r.done:
				return
			default:
			}
			return
		}

		r.handlePacket(buf[:n], remoteAddr)
	}
}

// Stop stops the runtime, closing the UDP connection and waiting for goroutines to exit
func (r *Runtime) Stop() {
	close(r.done)
	if r.conn != nil {
		r.conn.Close()
	}
	r.wg.Wait()
}

// handlePacket processes a single UDP packet
func (r *Runtime) handlePacket(data []byte, remoteAddr *net.UDPAddr) {
	if len(data) < 14 {
		return
	}

	itemCount := binary.LittleEndian.Uint16(data[0:2])
	if itemCount != 2 {
		return
	}

	offset := 2
	// type1
	offset += 2
	len1 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len1 != 4 {
		return
	}

	connID := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Item 2: Data Item
	offset += 2
	len2 := binary.LittleEndian.Uint16(data[offset : offset+2])
	offset += 2

	if len(data) < offset+int(len2) {
		return
	}

	payload := data[offset : offset+int(len2)]

	r.mu.Lock()
	conn, ok := r.connections[connID]
	if !ok {
		r.mu.Unlock()
		return
	}
	conn.LastReceive = time.Now()

	runIdleHeader := conn.RunIdleHeader
	asm := conn.Assembly
	r.mu.Unlock()

	// Class 1 data always starts with a 2-byte sequence count.
	dataOffset := 2
	if len(payload) < dataOffset {
		return
	}
	if runIdleHeader {
		dataOffset += 4 // skip 4-byte run/idle header
		if len(payload) < dataOffset {
			return
		}
	}

	if asm != nil {
		r.assemblyObj.SetAttributeSingle(asm.ID, 3, payload[dataOffset:])
	}
}
