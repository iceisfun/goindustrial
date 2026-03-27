package ethernetip

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/connmgr"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/runtime"
)

// ===========================================================================
// Test 1: ForwardOpenRequest encode round-trip
// ===========================================================================

func TestForwardOpenRequestEncode(t *testing.T) {
	connPath := cip.NewPath()
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(100)
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(101)

	original := &connmgr.ForwardOpenRequest{
		PriorityTimeTick:            0x0A,
		TimeoutTicks:                0xF0,
		OTConnectionID:              0x1234,
		TOConnectionID:              0,
		ConnectionSerialNumber:      0x0001,
		VendorID:                    0x0042,
		OriginatorSerialNumber:      0xABCD,
		ConnectionTimeoutMultiplier: 3,
		OTRPI:                       10000,
		OTNetworkConnectionParams:   0x43F4,
		TORPI:                       10000,
		TONetworkConnectionParams:   0x43F4,
		TransportTypeTrigger:        0x01,
		ConnectionPath:              connPath.Bytes(),
	}

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode using the server-side parser (HandleForwardOpen parses the same format).
	// Manually verify key fields from the encoded bytes.
	r := bytes.NewReader(encoded)

	var ptt cip.BYTE
	binary.Read(r, binary.LittleEndian, &ptt)
	if ptt != 0x0A {
		t.Errorf("PriorityTimeTick = 0x%02X, want 0x0A", ptt)
	}

	var tt cip.USINT
	binary.Read(r, binary.LittleEndian, &tt)
	if tt != 0xF0 {
		t.Errorf("TimeoutTicks = 0x%02X, want 0xF0", tt)
	}

	var otID cip.UDINT
	binary.Read(r, binary.LittleEndian, &otID)
	if otID != 0x1234 {
		t.Errorf("OTConnectionID = 0x%08X, want 0x1234", otID)
	}

	var toID cip.UDINT
	binary.Read(r, binary.LittleEndian, &toID)
	if toID != 0 {
		t.Errorf("TOConnectionID = 0x%08X, want 0", toID)
	}

	var serial cip.UINT
	binary.Read(r, binary.LittleEndian, &serial)
	if serial != 0x0001 {
		t.Errorf("ConnectionSerialNumber = 0x%04X, want 0x0001", serial)
	}

	var vid cip.UINT
	binary.Read(r, binary.LittleEndian, &vid)
	if vid != 0x0042 {
		t.Errorf("VendorID = 0x%04X, want 0x0042", vid)
	}

	var osn cip.UDINT
	binary.Read(r, binary.LittleEndian, &osn)
	if osn != 0xABCD {
		t.Errorf("OriginatorSerialNumber = 0x%08X, want 0xABCD", osn)
	}
}

// ===========================================================================
// Test 2: IOScanner Open/Close lifecycle
// ===========================================================================

func TestIOScannerOpenClose(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 8)) // consume
	ao.RegisterAssembly(101, make([]byte, 8)) // produce

	var openCalled, closeCalled bool
	var mu sync.Mutex

	cm := connmgr.NewConnectionManager(
		connmgr.WithOnOpen(func(c *connmgr.Connection, req *connmgr.ForwardOpenRequest) {
			mu.Lock()
			openCalled = true
			mu.Unlock()
		}),
		connmgr.WithOnClose(func(c *connmgr.Connection) {
			mu.Lock()
			closeCalled = true
			mu.Unlock()
		}),
	)

	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassConnectionMgr, cm)

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router, WithServerConn(serverConn))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}
	t.Cleanup(func() { tc.Close() })

	sess := NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	scanner, err := NewIOScanner(sess, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewIOScanner: %v", err)
	}

	cfg := IOConnectionConfig{
		OTConnectionPoint: 100,
		TOConnectionPoint: 101,
		OTSize:            8,
		TOSize:            8,
		RPI:               10 * time.Millisecond,
		TimeoutMultiplier: 3,
		TargetAddr:        &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999}, // dummy
	}

	conn, err := scanner.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if conn.OTConnectionID == 0 {
		t.Error("OTConnectionID should not be 0")
	}
	if conn.TOConnectionID == 0 {
		t.Error("TOConnectionID should not be 0")
	}

	mu.Lock()
	if !openCalled {
		t.Error("OnOpen callback not fired")
	}
	mu.Unlock()

	if err := scanner.Close(ctx, conn); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	if !closeCalled {
		t.Error("OnClose callback not fired")
	}
	mu.Unlock()

	scanner.Shutdown(ctx)
}

// ===========================================================================
// Test 3: Full bidirectional I/O exchange
// ===========================================================================

func TestIOBidirectionalExchange(t *testing.T) {
	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 4)) // consume (scanner sends here)
	ao.RegisterAssembly(101, make([]byte, 4)) // produce (server sends from here)

	rt := runtime.NewRuntime(ao)
	if err := rt.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("runtime start: %v", err)
	}
	t.Cleanup(func() { rt.Stop() })

	sched := runtime.NewScheduler(rt)
	sched.Start()
	t.Cleanup(func() { sched.Stop() })

	// Get the runtime's actual UDP address for the scanner to target.
	runtimeAddr := rt.Addr()

	cm := connmgr.NewConnectionManager(
		connmgr.WithOnOpen(func(c *connmgr.Connection, req *connmgr.ForwardOpenRequest) {
			rpi := time.Duration(req.OTRPI) * time.Microsecond
			// Consumer: receives data from scanner on OTConnectionID
			rt.AddConnection(&runtime.IOConnection{
				ConnectionID:  c.OTConnectionID,
				RPI:           rpi,
				Assembly:      ao.GetInstance(100),
				IsConsumer:    true,
				TimeoutMult:   uint8(req.ConnectionTimeoutMultiplier),
				RunIdleHeader: false,
				StopChan:      make(chan struct{}),
			})
			// Producer: sends data to scanner on TOConnectionID
			rt.AddConnection(&runtime.IOConnection{
				ConnectionID:  c.TOConnectionID,
				RPI:           rpi,
				Assembly:      ao.GetInstance(101),
				IsProducer:    true,
				RunIdleHeader: false,
				StopChan:      make(chan struct{}),
			})
		}),
		connmgr.WithOnClose(func(c *connmgr.Connection) {
			rt.RemoveConnection(c.OTConnectionID)
			rt.RemoveConnection(c.TOConnectionID)
		}),
	)

	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassConnectionMgr, cm)

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router, WithServerConn(serverConn))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}
	t.Cleanup(func() { tc.Close() })

	sess := NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	scanner, err := NewIOScanner(sess, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewIOScanner: %v", err)
	}

	cfg := IOConnectionConfig{
		OTConnectionPoint: 100,
		TOConnectionPoint: 101,
		OTSize:            4,
		TOSize:            4,
		RPI:               10 * time.Millisecond,
		TimeoutMultiplier: 3,
		TargetAddr:        runtimeAddr,
	}

	ioConn, err := scanner.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// --- Scanner → Server (O→T) ---
	// Write test data to the scanner's output assembly.
	outputData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := ioConn.SetOutput(outputData); err != nil {
		t.Fatalf("SetOutput: %v", err)
	}

	// Wait for the server runtime to receive the data.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := ao.GetAttributeSingle(100, 3)
		if err == nil && bytes.Equal(data, outputData) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	consumed, err := ao.GetAttributeSingle(100, 3)
	if err != nil {
		t.Fatalf("read consume assembly: %v", err)
	}
	if !bytes.Equal(consumed, outputData) {
		t.Errorf("O→T: server received %X, want %X", consumed, outputData)
	}

	// --- Server → Scanner (T→O) ---
	// Write test data to the server's produce assembly.
	produceData := []byte{0xCA, 0xFE, 0xBA, 0xBE}
	if err := ao.SetAttributeSingle(101, 3, produceData); err != nil {
		t.Fatalf("write produce assembly: %v", err)
	}

	// The server scheduler needs the producer connection's RemoteAddr to send
	// to the scanner. Set it to the scanner's UDP address.
	scannerAddr := scanner.udpConn.LocalAddr().(*net.UDPAddr)
	rt.SetProducerAddr(ioConn.TOConnectionID, scannerAddr)

	// Wait for the scanner to receive the data.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		input := ioConn.Input()
		if bytes.Equal(input, produceData) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	input := ioConn.Input()
	if !bytes.Equal(input, produceData) {
		t.Errorf("T→O: scanner received %X, want %X", input, produceData)
	}

	// --- Clean shutdown ---
	if err := scanner.Close(ctx, ioConn); err != nil {
		t.Fatalf("Close: %v", err)
	}
	scanner.Shutdown(ctx)
}

// ===========================================================================
// Test 4: Watchdog timeout detection
// ===========================================================================

func TestIOConnTimeout(t *testing.T) {
	conn := &IOConn{
		RPI:         10 * time.Millisecond,
		timeoutMult: 0, // timeout = 10ms * 4 = 40ms
		inputData:   make([]byte, 4),
		stopCh:      make(chan struct{}),
	}
	// Set last receive to the past.
	conn.lastReceive.Store(time.Now().Add(-100 * time.Millisecond))

	if !conn.IsTimedOut() {
		t.Error("expected IsTimedOut() = true")
	}

	// Set last receive to now.
	conn.lastReceive.Store(time.Now())
	if conn.IsTimedOut() {
		t.Error("expected IsTimedOut() = false after fresh receive")
	}
}

// ===========================================================================
// Test 5: SetOutput size validation
// ===========================================================================

func TestIOConnSetOutputSizeValidation(t *testing.T) {
	conn := &IOConn{
		outputData: make([]byte, 4),
		stopCh:     make(chan struct{}),
	}

	if err := conn.SetOutput([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("SetOutput correct size: %v", err)
	}

	if err := conn.SetOutput([]byte{1, 2}); err == nil {
		t.Error("expected error for wrong size")
	}
}

// ===========================================================================
// Test 6: Multiple connections shutdown
// ===========================================================================

func TestIOScannerMultipleConnections(t *testing.T) {
	cm := connmgr.NewConnectionManager()
	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassConnectionMgr, cm)

	serverConn, clientConn := net.Pipe()
	srv := NewServer(router, WithServerConn(serverConn))
	if err := srv.Start(context.Background(), ""); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}
	t.Cleanup(func() { tc.Close() })

	sess := NewSession(tc, nil)
	ctx := context.Background()
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	scanner, err := NewIOScanner(sess, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewIOScanner: %v", err)
	}

	// Open 5 connections.
	conns := make([]*IOConn, 5)
	for i := range conns {
		conns[i], err = scanner.Open(ctx, IOConnectionConfig{
			OTConnectionPoint: uint16(100 + i),
			TOConnectionPoint: uint16(200 + i),
			OTSize:            4,
			TOSize:            4,
			RPI:               50 * time.Millisecond,
			TimeoutMultiplier: 3,
			TargetAddr:        &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9999},
		})
		if err != nil {
			t.Fatalf("Open connection %d: %v", i, err)
		}
	}

	// Verify all have unique IDs.
	seen := make(map[uint32]bool)
	for _, c := range conns {
		if seen[c.TOConnectionID] {
			t.Errorf("duplicate TOConnectionID: 0x%08X", c.TOConnectionID)
		}
		seen[c.TOConnectionID] = true
	}

	// Shutdown should close all and not hang.
	done := make(chan struct{})
	go func() {
		scanner.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out")
	}
}
