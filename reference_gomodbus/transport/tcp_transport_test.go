package transport

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// mockConn implements net.Conn for testing
type mockConn struct {
	readData     []byte
	readIndex    int
	writtenData  []byte
	closed       bool
	readDeadline time.Time
	mutex        sync.Mutex
}

func newMockConn() *mockConn {
	return &mockConn{
		readData:    make([]byte, 0),
		writtenData: make([]byte, 0),
	}
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return 0, net.ErrClosed
	}

	// Check if we've passed the deadline
	if !m.readDeadline.IsZero() && time.Now().After(m.readDeadline) {
		return 0, &timeoutError{}
	}

	if m.readIndex >= len(m.readData) {
		// Block until we have data or are closed
		time.Sleep(10 * time.Millisecond)
		return 0, &timeoutError{}
	}

	n = copy(b, m.readData[m.readIndex:])
	m.readIndex += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return 0, net.ErrClosed
	}

	m.writtenData = append(m.writtenData, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readDeadline = t
	return nil
}

// timeoutError implements net.Error for testing
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// createTestRequest creates a test request for tests
// We're using this helper to avoid confusion with the actual NewRequest function
func createTestRequest(unitID common.UnitID, functionCode common.FunctionCode, data []byte) common.Request {
	return NewRequest(unitID, functionCode, data)
}

// TestDisconnectClosedConnection tests that the Disconnect method handles
// closed connections gracefully.
func TestDisconnectClosedConnection(t *testing.T) {
	// Create a mock connection
	conn := newMockConn()

	// Create a TCPTransport with the mock connection
	transport := NewTCPTransport("localhost")
	transport.conn = conn
	transport.reader = conn
	transport.writer = conn

	// Mark as connected
	transport.connected = true

	// Start the read and write loops
	go transport.readLoop()
	go transport.writeLoop()

	// Wait a moment for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Disconnect should close the goroutines cleanly
	ctx := context.Background()
	err := transport.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Disconnect returned an error: %v", err)
	}

	// Wait a moment for goroutines to exit
	time.Sleep(100 * time.Millisecond)

	// Check that the connection was closed
	if !conn.closed {
		t.Errorf("Connection was not closed")
	}

	// Make sure we can reconnect after disconnect
	// Create a brand new transport to avoid reusing potentially closed channels
	transport = NewTCPTransport("localhost")

	// We'll skip the actual Connect call since it tries to dial a real connection
	// Instead, we'll manually set up the transport as if Connect succeeded
	conn = newMockConn()
	transport.conn = conn
	transport.reader = conn
	transport.writer = conn
	transport.connected = true

	// Start the read and write loops manually
	go transport.readLoop()
	go transport.writeLoop()

	// Wait a moment for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Disconnect again
	err = transport.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Second disconnect returned an error: %v", err)
	}

	// Wait for goroutines to exit
	time.Sleep(100 * time.Millisecond)

	// The connection should be closed
	if !conn.closed {
		t.Errorf("Connection was not closed on second disconnect")
	}
}

// TestMultipleDisconnects tests that calling Disconnect multiple times is safe.
func TestMultipleDisconnects(t *testing.T) {
	// Create a mock connection
	conn := newMockConn()

	// Create a TCPTransport with the mock connection
	transport := NewTCPTransport("localhost")
	transport.conn = conn
	transport.reader = conn
	transport.writer = conn

	// Mark as connected
	transport.connected = true

	// Start the read and write loops
	go transport.readLoop()
	go transport.writeLoop()

	// Wait a moment for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Disconnect
	ctx := context.Background()
	err := transport.Disconnect(ctx)
	if err != nil {
		t.Fatalf("First disconnect returned an error: %v", err)
	}

	// Disconnect again, should be a no-op
	err = transport.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Second disconnect returned an error: %v", err)
	}

	// Disconnect a third time, should still be a no-op
	err = transport.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Third disconnect returned an error: %v", err)
	}
}

// TestRaceConditionDisconnect tests that there's no race condition when
// disconnecting while read/write operations are in progress.
func TestRaceConditionDisconnect(t *testing.T) {
	// Create a mock connection
	conn := newMockConn()

	// Create a TCPTransport with the mock connection
	transport := NewTCPTransport("localhost")
	transport.conn = conn
	transport.reader = conn
	transport.writer = conn

	// Mark as connected
	transport.connected = true

	// Start the read and write loops
	go transport.readLoop()
	go transport.writeLoop()

	// Wait a moment for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Create a context for the test
	ctx := context.Background()

	// Start a goroutine that will disconnect after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		transport.Disconnect(ctx)
	}()

	// Wait for disconnection to complete
	time.Sleep(200 * time.Millisecond)

	// Check that we're disconnected
	if transport.IsConnected() {
		t.Errorf("Transport should be disconnected")
	}

	// The connection should be closed
	if !conn.closed {
		t.Errorf("Connection was not closed")
	}
}

// TestResetTransactions tests the ResetTransactions method
func TestResetTransactions(t *testing.T) {
	// Create a TCPTransport
	transport := NewTCPTransport("localhost")

	// Create a context for the test
	ctx := context.Background()

	// Get initial pool size
	initialCount := transport.transactionPool.GetCount()
	if initialCount != 0 {
		t.Errorf("Expected initial transaction count to be 0, got %d", initialCount)
	}

	// Create a dummy request to add to the pool
	request := createTestRequest(1, 0x03, []byte{0x00, 0x01, 0x00, 0x02})

	// Add it to the pool
	tx, err := transport.transactionPool.Place(ctx, request)
	if err != nil {
		t.Fatalf("Failed to place transaction: %v", err)
	}

	// Verify pool now has one transaction
	if count := transport.transactionPool.GetCount(); count != 1 {
		t.Errorf("Expected transaction count to be 1, got %d", count)
	}

	// Create a channel to detect when transaction is cancelled
	cancelled := make(chan struct{})
	go func() {
		select {
		case <-tx.ErrCh:
			close(cancelled)
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for transaction to be cancelled")
		}
	}()

	// Reset transactions
	transport.ResetTransactions(ctx)

	// Wait for transaction to be cancelled
	<-cancelled

	// Verify pool is empty
	if count := transport.transactionPool.GetCount(); count != 0 {
		t.Errorf("Expected transaction count to be 0 after reset, got %d", count)
	}

	// Verify we can add transactions after reset
	_, err = transport.transactionPool.Place(ctx, request)
	if err != nil {
		t.Fatalf("Failed to place transaction after reset: %v", err)
	}

	// Verify pool now has one transaction
	if count := transport.transactionPool.GetCount(); count != 1 {
		t.Errorf("Expected transaction count to be 1 after adding a new transaction, got %d", count)
	}
}