package ethernetip

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/connmgr"
)

// IOConnectionConfig describes the parameters for an implicit I/O connection.
type IOConnectionConfig struct {
	// OTConnectionPoint is the target assembly instance for O→T data.
	OTConnectionPoint uint16

	// TOConnectionPoint is the target assembly instance for T→O data.
	TOConnectionPoint uint16

	// ConfigInstance is the target configuration assembly (0 = none).
	ConfigInstance uint16

	// OTSize is the output assembly size in bytes.
	OTSize uint16

	// TOSize is the input assembly size in bytes.
	TOSize uint16

	// RPI is the Requested Packet Interval for cyclic data exchange.
	RPI time.Duration

	// TimeoutMultiplier controls the watchdog: timeout = RPI * (4 << mult).
	TimeoutMultiplier uint8

	// RunIdleHeader adds a 4-byte run/idle header to each packet.
	RunIdleHeader bool

	// TargetAddr is the UDP address to send O→T data to. If nil, the
	// scanner resolves it from the TCP session's remote address + port 2222.
	TargetAddr *net.UDPAddr
}

// IOScannerOption configures an IOScanner.
type IOScannerOption func(*IOScanner)

// WithIOLogger sets the logger for the scanner.
func WithIOLogger(l logging.Logger) IOScannerOption {
	return func(s *IOScanner) {
		s.logger = l
	}
}

// WithVendorID sets the originator vendor ID for Forward_Open requests.
func WithVendorID(id uint16) IOScannerOption {
	return func(s *IOScanner) {
		s.vendorID = id
	}
}

// WithOriginatorSerial sets the originator serial number.
func WithOriginatorSerial(sn uint32) IOScannerOption {
	return func(s *IOScanner) {
		s.originatorSN = sn
	}
}

// IOScanner manages implicit I/O connections over EtherNet/IP. It uses a TCP
// Session for control-plane operations (Forward_Open/Close) and a UDP socket
// for cyclic I/O data exchange.
type IOScanner struct {
	session      *Session
	udpConn      *net.UDPConn
	logger       logging.Logger
	vendorID     uint16
	originatorSN uint32

	mu          sync.RWMutex
	connections map[uint32]*IOConn // keyed by TOConnectionID (what we receive on)
	nextOTID    atomic.Uint32
	nextSerial  atomic.Uint32

	done chan struct{}
	wg   sync.WaitGroup
}

// IOConn represents an active implicit I/O connection. Data is exchanged
// cyclically at the negotiated RPI. Use SetOutput to update the O→T assembly
// and Input to read the latest T→O assembly data.
type IOConn struct {
	// Connection identifiers (read-only after creation).
	OTConnectionID         uint32
	TOConnectionID         uint32
	ConnectionSerialNumber uint16
	VendorID               uint16
	OriginatorSerialNumber uint32

	outputData []byte
	outputMu   sync.RWMutex

	inputData []byte
	inputMu   sync.RWMutex

	// RPI is the actual negotiated packet interval.
	RPI           time.Duration
	sequenceCount uint16
	lastReceive   atomic.Value // time.Time
	timeoutMult   uint8
	runIdleHeader bool
	remoteAddr    *net.UDPAddr

	scanner *IOScanner
	stopCh  chan struct{}
	stopMu  sync.Once
}

// NewIOScanner creates an I/O scanner that uses the given TCP session for
// Forward_Open/Close and binds a UDP socket to localUDPAddr for cyclic data.
// Pass ":0" as localUDPAddr to use an ephemeral port.
func NewIOScanner(session *Session, localUDPAddr string, opts ...IOScannerOption) (*IOScanner, error) {
	addr, err := net.ResolveUDPAddr("udp", localUDPAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve UDP addr: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind UDP: %w", err)
	}

	s := &IOScanner{
		session:      session,
		udpConn:      udpConn,
		logger:       logging.NewNopLogger(),
		vendorID:     1,
		originatorSN: 1,
		connections:  make(map[uint32]*IOConn),
		done:         make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.receiveLoop()
	}()

	return s, nil
}

// Open establishes an implicit I/O connection by sending Forward_Open over
// TCP and starting cyclic UDP data exchange at the negotiated RPI.
func (s *IOScanner) Open(ctx context.Context, cfg IOConnectionConfig) (*IOConn, error) {
	otConnID := s.nextOTID.Add(1)
	serial := cip.UINT(s.nextSerial.Add(1))

	// Build connection path: [Class Assembly, Instance OT] [Class Assembly, Instance TO]
	connPath := cip.NewPath()
	if cfg.ConfigInstance > 0 {
		connPath.AddClass(cip.ClassAssembly)
		connPath.AddInstance(cip.UINT(cfg.ConfigInstance))
	}
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(cip.UINT(cfg.OTConnectionPoint))
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(cip.UINT(cfg.TOConnectionPoint))

	rpiMicros := cip.UDINT(cfg.RPI / time.Microsecond)

	otParams := connmgr.ConnParamPointToPoint | connmgr.ConnParamFixedSize | cip.WORD(cfg.OTSize)
	toParams := connmgr.ConnParamPointToPoint | connmgr.ConnParamFixedSize | cip.WORD(cfg.TOSize)

	foReq := &connmgr.ForwardOpenRequest{
		PriorityTimeTick:            0x0A,
		TimeoutTicks:                0xF0,
		OTConnectionID:              cip.UDINT(otConnID),
		TOConnectionID:              0, // server assigns
		ConnectionSerialNumber:      serial,
		VendorID:                    cip.UINT(s.vendorID),
		OriginatorSerialNumber:      cip.UDINT(s.originatorSN),
		ConnectionTimeoutMultiplier: cip.USINT(cfg.TimeoutMultiplier),
		OTRPI:                       rpiMicros,
		OTNetworkConnectionParams:   otParams,
		TORPI:                       rpiMicros,
		TONetworkConnectionParams:   toParams,
		TransportTypeTrigger:        0x01, // Class 1, cyclic, server trigger
		ConnectionPath:              connPath.Bytes(),
	}

	foData, err := foReq.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode Forward_Open: %w", err)
	}

	mrPath := cip.NewPath()
	mrPath.AddClass(cip.ClassConnectionMgr)
	mrPath.AddInstance(1)

	mrReq := &cip.MessageRouterRequest{
		Service:     cip.USINT(connmgr.ServiceForwardOpen),
		RequestPath: mrPath,
		RequestData: foData,
	}

	resp, err := s.session.SendCIPRequest(ctx, mrReq)
	if err != nil {
		return nil, fmt.Errorf("Forward_Open send: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("Forward_Open failed: status 0x%02X", resp.GeneralStatus)
	}

	// Parse ForwardOpenResponse (26 bytes minimum: OT+TO conn IDs + triad + APIs + reply size + reserved)
	if len(resp.ResponseData) < 26 {
		return nil, fmt.Errorf("Forward_Open response too short: %d bytes", len(resp.ResponseData))
	}
	toConnID := binary.LittleEndian.Uint32(resp.ResponseData[4:8])
	otAPI := time.Duration(binary.LittleEndian.Uint32(resp.ResponseData[16:20])) * time.Microsecond
	if otAPI == 0 {
		otAPI = cfg.RPI
	}

	// Resolve target UDP address.
	targetAddr := cfg.TargetAddr
	if targetAddr == nil {
		if s.session.conn != nil && s.session.conn.conn != nil {
			host, _, _ := net.SplitHostPort(s.session.conn.conn.RemoteAddr().String())
			targetAddr, _ = net.ResolveUDPAddr("udp", net.JoinHostPort(host, "2222"))
		}
		if targetAddr == nil {
			return nil, fmt.Errorf("cannot resolve target UDP address")
		}
	}

	conn := &IOConn{
		OTConnectionID:         otConnID,
		TOConnectionID:         toConnID,
		ConnectionSerialNumber: uint16(serial),
		VendorID:               s.vendorID,
		OriginatorSerialNumber: s.originatorSN,
		outputData:             make([]byte, cfg.OTSize),
		inputData:              make([]byte, cfg.TOSize),
		RPI:                    otAPI,
		timeoutMult:            cfg.TimeoutMultiplier,
		runIdleHeader:          cfg.RunIdleHeader,
		remoteAddr:             targetAddr,
		scanner:                s,
		stopCh:                 make(chan struct{}),
	}
	conn.lastReceive.Store(time.Now())

	s.mu.Lock()
	s.connections[toConnID] = conn
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		conn.produceLoop()
	}()

	return conn, nil
}

// Close sends Forward_Close for the given connection and stops its producer.
func (s *IOScanner) Close(ctx context.Context, conn *IOConn) error {
	conn.stopMu.Do(func() { close(conn.stopCh) })

	s.mu.Lock()
	delete(s.connections, conn.TOConnectionID)
	s.mu.Unlock()

	connPath := cip.NewPath()
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(1)

	fcReq := &connmgr.ForwardCloseRequest{
		PriorityTimeTick:       0x0A,
		TimeoutTicks:           0xF0,
		ConnectionSerialNumber: cip.UINT(conn.ConnectionSerialNumber),
		VendorID:               cip.UINT(conn.VendorID),
		OriginatorSerialNumber: cip.UDINT(conn.OriginatorSerialNumber),
		ConnectionPath:         connPath.Bytes(),
	}

	fcData, err := fcReq.Encode()
	if err != nil {
		return fmt.Errorf("encode Forward_Close: %w", err)
	}

	mrPath := cip.NewPath()
	mrPath.AddClass(cip.ClassConnectionMgr)
	mrPath.AddInstance(1)

	mrReq := &cip.MessageRouterRequest{
		Service:     cip.USINT(connmgr.ServiceForwardClose),
		RequestPath: mrPath,
		RequestData: fcData,
	}

	resp, err := s.session.SendCIPRequest(ctx, mrReq)
	if err != nil {
		return fmt.Errorf("Forward_Close send: %w", err)
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("Forward_Close failed: status 0x%02X", resp.GeneralStatus)
	}

	return nil
}

// Shutdown closes all active connections and stops the scanner.
func (s *IOScanner) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	conns := make([]*IOConn, 0, len(s.connections))
	for _, c := range s.connections {
		conns = append(conns, c)
	}
	s.mu.RUnlock()

	var firstErr error
	for _, c := range conns {
		if err := s.Close(ctx, c); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	close(s.done)
	s.udpConn.Close()
	s.wg.Wait()

	return firstErr
}

// SetOutput updates the O→T assembly data sent to the target each cycle.
// The data must match the configured OTSize.
func (c *IOConn) SetOutput(data []byte) error {
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if len(data) != len(c.outputData) {
		return fmt.Errorf("output size mismatch: got %d, want %d", len(data), len(c.outputData))
	}
	copy(c.outputData, data)
	return nil
}

// Output returns a copy of the current output assembly.
func (c *IOConn) Output() []byte {
	c.outputMu.RLock()
	defer c.outputMu.RUnlock()
	out := make([]byte, len(c.outputData))
	copy(out, c.outputData)
	return out
}

// Input returns a copy of the last received T→O assembly data.
func (c *IOConn) Input() []byte {
	c.inputMu.RLock()
	defer c.inputMu.RUnlock()
	out := make([]byte, len(c.inputData))
	copy(out, c.inputData)
	return out
}

// InputAge returns the time elapsed since the last received packet.
func (c *IOConn) InputAge() time.Duration {
	if t, ok := c.lastReceive.Load().(time.Time); ok {
		return time.Since(t)
	}
	return 0
}

// IsTimedOut returns true if no packet has been received within the
// watchdog timeout (RPI * (4 << TimeoutMultiplier)).
func (c *IOConn) IsTimedOut() bool {
	mult := uint64(4) << c.timeoutMult
	timeout := c.RPI * time.Duration(mult)
	return c.InputAge() > timeout
}

// produceLoop sends the output assembly to the target at the negotiated RPI.
func (c *IOConn) produceLoop() {
	ticker := time.NewTicker(c.RPI)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-c.scanner.done:
			return
		case <-ticker.C:
			c.sendPacket()
		}
	}
}

func (c *IOConn) sendPacket() {
	c.outputMu.RLock()
	snapshot := make([]byte, len(c.outputData))
	copy(snapshot, c.outputData)
	c.outputMu.RUnlock()

	c.sequenceCount++

	// Build CPF packet manually (same format as runtime/scheduler.go)
	dataLen := 2 + len(snapshot) // seq count + assembly
	if c.runIdleHeader {
		dataLen += 4
	}

	pkt := make([]byte, 14+dataLen)
	// Item count = 2
	binary.LittleEndian.PutUint16(pkt[0:2], 2)
	// Item 1: Connected Address (0x00A1), length 4
	binary.LittleEndian.PutUint16(pkt[2:4], eip.ItemIDConnectedAddress)
	binary.LittleEndian.PutUint16(pkt[4:6], 4)
	binary.LittleEndian.PutUint32(pkt[6:10], c.OTConnectionID)
	// Item 2: Connected Data (0x00B1), length = dataLen
	binary.LittleEndian.PutUint16(pkt[10:12], eip.ItemIDConnectedData)
	binary.LittleEndian.PutUint16(pkt[12:14], uint16(dataLen))

	offset := 14
	binary.LittleEndian.PutUint16(pkt[offset:offset+2], c.sequenceCount)
	offset += 2
	if c.runIdleHeader {
		binary.LittleEndian.PutUint32(pkt[offset:offset+4], 1) // Run mode
		offset += 4
	}
	copy(pkt[offset:], snapshot)

	c.scanner.udpConn.WriteToUDP(pkt, c.remoteAddr)
}

// receiveLoop reads UDP packets and dispatches them to the correct IOConn.
func (s *IOScanner) receiveLoop() {
	buf := make([]byte, 2048)
	for {
		n, _, err := s.udpConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			return
		}

		s.handleIncomingPacket(buf[:n])
	}
}

func (s *IOScanner) handleIncomingPacket(data []byte) {
	// Minimum: 2 (count) + 4 (item1 hdr) + 4 (connID) + 4 (item2 hdr) + 2 (seq) = 16
	if len(data) < 16 {
		return
	}

	itemCount := binary.LittleEndian.Uint16(data[0:2])
	if itemCount != 2 {
		return
	}

	// Item 1: Connected Address
	// type at 2:4, length at 4:6, data at 6:10
	connID := binary.LittleEndian.Uint32(data[6:10])

	// Item 2: Connected Data
	// type at 10:12, length at 12:14, data starts at 14
	dataLen := binary.LittleEndian.Uint16(data[12:14])
	if len(data) < 14+int(dataLen) {
		return
	}
	payload := data[14 : 14+int(dataLen)]

	s.mu.RLock()
	conn, ok := s.connections[connID]
	s.mu.RUnlock()
	if !ok {
		return
	}

	// Skip sequence count (2 bytes)
	offset := 2
	if conn.runIdleHeader {
		offset += 4 // skip run/idle header
	}
	if offset > len(payload) {
		return
	}

	assemblyData := payload[offset:]

	conn.inputMu.Lock()
	if len(assemblyData) == len(conn.inputData) {
		copy(conn.inputData, assemblyData)
	}
	conn.inputMu.Unlock()
	conn.lastReceive.Store(time.Now())
}
