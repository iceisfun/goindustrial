// Package runtime manages the UDP-based implicit (cyclic) I/O messaging
// runtime for EtherNet/IP.
//
// In EtherNet/IP, implicit messaging is the mechanism by which a scanner and
// adapter exchange process data at a fixed rate over UDP. After a Forward Open
// establishes a connection, both sides send and receive I/O data packets at
// the Requested Packet Interval (RPI). This package provides the Runtime
// (UDP listener and consumer) and Scheduler (producer) that together handle
// that cyclic data exchange.
//
// Typical usage:
//
//  1. Create an assembly.AssemblyObject and register Input/Output instances.
//  2. Create a Runtime with NewRuntime, then call Start to begin listening.
//  3. When a Forward Open succeeds, call AddConnection to register the I/O
//     connection with the runtime.
//  4. Create a Scheduler with NewScheduler and call Start to begin producing
//     data at each connection's RPI.
//  5. On Forward Close, call RemoveConnection to tear down the connection.
//  6. Call Scheduler.Stop and Runtime.Stop during shutdown.
package runtime

import (
	"encoding/binary"
	"net"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
)

// IOConnection represents a single cyclic I/O connection between a scanner
// and an adapter. It tracks the connection parameters negotiated during
// Forward Open, the associated assembly instance, and timing state for
// production and consumption.
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

// Runtime manages the UDP listener that receives implicit I/O packets and
// dispatches them to the correct assembly instance. It also runs a watchdog
// that removes connections that have not received data within their timeout
// window. All methods are safe for concurrent use.
type Runtime struct {
	mu          sync.RWMutex
	conn        *net.UDPConn
	connections map[uint32]*IOConnection // Map by ConnectionID (Consuming ID)
	assemblyObj *assembly.AssemblyObject
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewRuntime creates a new Runtime backed by the given AssemblyObject. The
// runtime is not started until Start is called.
func NewRuntime(ao *assembly.AssemblyObject) *Runtime {
	return &Runtime{
		connections: make(map[uint32]*IOConnection),
		assemblyObj: ao,
	}
}

// Start begins listening for implicit I/O UDP packets on the given address
// (for example ":2222" or "0.0.0.0:2222"). It spawns a listener goroutine
// and a watchdog goroutine. Use Stop to shut down both.
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

// AddConnection registers an I/O connection with the runtime. The connection's
// LastReceive timestamp is set to the current time, and it becomes eligible
// for the watchdog timeout check.
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

// Addr returns the local UDP address the runtime is listening on, or nil if
// the runtime has not been started. This is useful in tests to discover the
// ephemeral port assigned by the OS.
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

// Stop shuts down the runtime by closing the UDP connection and waiting for
// the listener and watchdog goroutines to exit.
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
