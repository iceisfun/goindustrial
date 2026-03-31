package transport

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockConn struct {
	id int
}

func newMockConnector(counter *atomic.Int32, failAfter int) Connector[*mockConn] {
	return ConnectorFunc[*mockConn](func(ctx context.Context) (*mockConn, error) {
		n := int(counter.Add(1))
		if failAfter > 0 && n > failAfter {
			return nil, errors.New("connect failed")
		}
		return &mockConn{id: n}, nil
	})
}

func newMockCloser() Closer[*mockConn] {
	return CloserFunc[*mockConn](func(conn *mockConn) error {
		return nil
	})
}

func TestDirectTransportConn(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	dt, err := NewDirectTransport(ctx, newMockConnector(&counter, 0), newMockCloser())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer dt.Close()

	conn, err := dt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.id != 1 {
		t.Errorf("expected conn id 1, got %d", conn.id)
	}

	// Same connection on subsequent calls.
	conn2, err := dt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != conn2 {
		t.Error("expected same connection instance")
	}
}

func TestDirectTransportResetThenFail(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	dt, err := NewDirectTransport(ctx, newMockConnector(&counter, 0), newMockCloser())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	conn, _ := dt.Conn(ctx)
	dt.Reset(conn)

	_, err = dt.Conn(ctx)
	if err == nil {
		t.Error("expected error after reset on direct transport")
	}
}

func TestDirectTransportClose(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	dt, err := NewDirectTransport(ctx, newMockConnector(&counter, 0), newMockCloser())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dt.Close()

	_, err = dt.Conn(ctx)
	if err == nil {
		t.Error("expected error after close")
	}

	// Double close should not panic.
	dt.Close()
}

func TestDirectTransportOnConnectCallback(t *testing.T) {
	var counter atomic.Int32
	var connected bool
	ctx := context.Background()

	_, err := NewDirectTransport(ctx, newMockConnector(&counter, 0), newMockCloser(),
		WithOnConnect(func() { connected = true }),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !connected {
		t.Error("expected OnConnect callback to fire")
	}
}

func TestDialReconnectingTransport(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt, err := DialReconnectingTransport(ctx, newMockConnector(&counter, 0), newMockCloser())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rt.Close()

	// Connection should already exist.
	if n := counter.Load(); n != 1 {
		t.Errorf("expected 1 connection at construction, got %d", n)
	}

	// Conn returns the already-established connection.
	conn, err := rt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.id != 1 {
		t.Errorf("expected conn id 1, got %d", conn.id)
	}

	// Still reconnects after Reset.
	rt.Reset(conn)
	conn2, err := rt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn2.id != 2 {
		t.Errorf("expected conn id 2 after reconnect, got %d", conn2.id)
	}
}

func TestDialReconnectingTransportConnectError(t *testing.T) {
	failing := ConnectorFunc[*mockConn](func(ctx context.Context) (*mockConn, error) {
		return nil, errors.New("connection refused")
	})

	rt, err := DialReconnectingTransport(context.Background(), failing, newMockCloser())
	if err == nil {
		t.Fatal("expected error from DialReconnectingTransport")
	}
	if rt != nil {
		t.Error("expected nil transport on error")
	}
}

func TestDialReconnectingTransportCallbacks(t *testing.T) {
	var counter atomic.Int32
	var connected bool
	ctx := context.Background()

	rt, err := DialReconnectingTransport(ctx, newMockConnector(&counter, 0), newMockCloser(),
		WithOnConnect(func() { connected = true }),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rt.Close()

	if !connected {
		t.Error("expected OnConnect callback to fire during Dial")
	}
}

func TestReconnectingTransportLazyConnect(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser())
	defer rt.Close()

	if counter.Load() != 0 {
		t.Error("expected no connection at construction time")
	}

	conn, err := rt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn.id != 1 {
		t.Errorf("expected conn id 1, got %d", conn.id)
	}

	// Subsequent calls return same connection.
	conn2, err := rt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn != conn2 {
		t.Error("expected same connection")
	}
}

func TestReconnectingTransportResetAndReconnect(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser())
	defer rt.Close()

	conn1, _ := rt.Conn(ctx)
	rt.Reset(conn1)

	conn2, err := rt.Conn(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn2.id != 2 {
		t.Errorf("expected conn id 2 after reconnect, got %d", conn2.id)
	}
}

func TestReconnectingTransportStaleResetIgnored(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser())
	defer rt.Close()

	conn1, _ := rt.Conn(ctx)
	rt.Reset(conn1)

	conn2, _ := rt.Conn(ctx)

	// Resetting stale conn1 should be a no-op.
	rt.Reset(conn1)

	conn3, _ := rt.Conn(ctx)
	if conn3 != conn2 {
		t.Error("stale reset should not invalidate current connection")
	}
}

func TestReconnectingTransportClose(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser())
	rt.Conn(ctx)
	rt.Close()

	_, err := rt.Conn(ctx)
	if err == nil {
		t.Error("expected error after close")
	}

	// Double close should not panic.
	rt.Close()
}

func TestReconnectingTransportConcurrency(t *testing.T) {
	var counter atomic.Int32
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser())
	defer rt.Close()

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			conn, err := rt.Conn(ctx)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if conn == nil {
				t.Error("expected non-nil connection")
			}
		})
	}
	wg.Wait()

	// Only one connection should have been created.
	if n := counter.Load(); n != 1 {
		t.Errorf("expected 1 connection, got %d", n)
	}
}

// TestReconnectingTransportCloseDuringConnect tests that Close() completes
// promptly even when a Connect call is in progress. The slow connector
// blocks until its channel is closed, simulating a long connection attempt.
func TestReconnectingTransportCloseDuringConnect(t *testing.T) {
	blockCh := make(chan struct{})
	slowConnector := ConnectorFunc[*mockConn](func(ctx context.Context) (*mockConn, error) {
		select {
		case <-blockCh:
			return nil, errors.New("connect aborted")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	rt := NewReconnectingTransport(slowConnector, newMockCloser())

	// Start a Conn() call that will block in Connect.
	connDone := make(chan error, 1)
	go func() {
		_, err := rt.Conn(context.Background())
		connDone <- err
	}()

	// Give the goroutine time to enter Connect.
	time.Sleep(20 * time.Millisecond)

	// Close the transport. This acquires the write lock; the Conn() goroutine
	// holds the write lock during Connect, so Close will block until Connect
	// returns. Unblock the connector to let everything proceed.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- rt.Close()
	}()

	// Unblock the connector so both Conn() and Close() can proceed.
	close(blockCh)

	select {
	case err := <-connDone:
		// Conn should fail (connect aborted or transport closed).
		if err == nil {
			t.Error("expected error from Conn() during close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Conn() did not return within timeout")
	}

	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return within timeout")
	}

	// After close, Conn should return an error.
	_, err := rt.Conn(context.Background())
	if err == nil {
		t.Error("expected error from Conn() after Close()")
	}
}

func TestReconnectingTransportCallbacks(t *testing.T) {
	var counter atomic.Int32
	var connectCount, disconnectCount int
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser(),
		WithOnConnect(func() { connectCount++ }),
		WithOnDisconnect(func(err error) { disconnectCount++ }),
	)

	conn, _ := rt.Conn(ctx)
	if connectCount != 1 {
		t.Errorf("expected 1 connect callback, got %d", connectCount)
	}

	rt.Reset(conn)
	if disconnectCount != 1 {
		t.Errorf("expected 1 disconnect callback, got %d", disconnectCount)
	}

	rt.Conn(ctx)
	if connectCount != 2 {
		t.Errorf("expected 2 connect callbacks, got %d", connectCount)
	}

	rt.Close()
	if disconnectCount != 2 {
		t.Errorf("expected 2 disconnect callbacks, got %d", disconnectCount)
	}
}

func TestMultipleCallbacks(t *testing.T) {
	var counter atomic.Int32
	var a, b int
	ctx := context.Background()

	rt := NewReconnectingTransport(newMockConnector(&counter, 0), newMockCloser(),
		WithOnConnect(func() { a++ }),
		WithOnConnect(func() { b += 10 }),
		WithOnDisconnect(func(error) { a += 100 }),
		WithOnDisconnect(func(error) { b += 1000 }),
	)

	conn, _ := rt.Conn(ctx)
	if a != 1 || b != 10 {
		t.Errorf("after connect: a=%d b=%d, want a=1 b=10", a, b)
	}

	rt.Reset(conn)
	if a != 101 || b != 1010 {
		t.Errorf("after reset: a=%d b=%d, want a=101 b=1010", a, b)
	}

	rt.Conn(ctx)
	if a != 102 || b != 1020 {
		t.Errorf("after reconnect: a=%d b=%d, want a=102 b=1020", a, b)
	}

	rt.Close()
	if a != 202 || b != 2020 {
		t.Errorf("after close: a=%d b=%d, want a=202 b=2020", a, b)
	}
}
