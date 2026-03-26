package transport

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
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
