package modbus

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// captureLogger records Info and Error messages so tests can assert on what
// was logged. All other levels fall through to the embedded nop logger.
type captureLogger struct {
	logging.Logger
	mu    sync.Mutex
	infos []string
	errs  []string
}

func newCaptureLogger() *captureLogger {
	return &captureLogger{Logger: logging.NewNopLogger()}
}

func (l *captureLogger) Info(_ context.Context, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, fmt.Sprintf(format, args...))
}

func (l *captureLogger) Error(_ context.Context, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, fmt.Sprintf(format, args...))
}

func (l *captureLogger) WithFields(map[string]any) logging.Logger { return l }

func (l *captureLogger) infosContaining(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, msg := range l.infos {
		if strings.Contains(msg, substr) {
			n++
		}
	}
	return n
}

func (l *captureLogger) errorsContaining(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, msg := range l.errs {
		if strings.Contains(msg, substr) {
			n++
		}
	}
	return n
}

// swapConn is a net.Conn whose underlying connection can be replaced between
// connection cycles, so a single TCPConn (whose injected conn is fixed) can be
// reconnected against a fresh net.Pipe. Close closes the current inner conn
// and counts calls.
type swapConn struct {
	mu     sync.Mutex
	inner  net.Conn
	closes int
}

func (s *swapConn) get() net.Conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner
}

func (s *swapConn) swap(c net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = c
}

func (s *swapConn) closeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closes
}

func (s *swapConn) Read(b []byte) (int, error)  { return s.get().Read(b) }
func (s *swapConn) Write(b []byte) (int, error) { return s.get().Write(b) }

func (s *swapConn) Close() error {
	s.mu.Lock()
	s.closes++
	inner := s.inner
	s.mu.Unlock()
	return inner.Close()
}

func (s *swapConn) LocalAddr() net.Addr                { return s.get().LocalAddr() }
func (s *swapConn) RemoteAddr() net.Addr               { return s.get().RemoteAddr() }
func (s *swapConn) SetDeadline(t time.Time) error      { return s.get().SetDeadline(t) }
func (s *swapConn) SetReadDeadline(t time.Time) error  { return s.get().SetReadDeadline(t) }
func (s *swapConn) SetWriteDeadline(t time.Time) error { return s.get().SetWriteDeadline(t) }

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// TestPeerCloseReleasesSocketAndGoroutines asserts that a peer-initiated close
// alone — with no Disconnect call — releases everything the connection holds:
// the socket fd, the read/write loops, and the transaction pool's timeout
// monitor goroutine. Without this, a TCPConn abandoned after a peer close (or
// replaced by the reconnecting transport before Reset runs) pins a goroutine
// and an fd until Disconnect happens to be called.
func TestPeerCloseReleasesSocketAndGoroutines(t *testing.T) {
	baseline := runtime.NumGoroutine()

	serverConn, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_ = serverConn.Close()

	waitFor(t, 2*time.Second, "connected flag cleared after peer close", func() bool {
		return !c.IsConnected()
	})
	waitFor(t, 2*time.Second, "socket closed after peer close (fd leak)", func() bool {
		return wrapped.closes.Load() == 1
	})
	waitFor(t, 3*time.Second, "all connection goroutines exited (read/write loop + pool monitor)", func() bool {
		return runtime.NumGoroutine() <= baseline
	})
}

// TestDisconnectQuietAfterPeerClose asserts that when the socket was already
// torn down by a peer-initiated close, the transport's subsequent Disconnect
// is silent: no "Disconnected" info log and no second Close of the socket.
// The single Error log from the loop that observed the drop is the one
// record of the event.
func TestDisconnectQuietAfterPeerClose(t *testing.T) {
	logs := newCaptureLogger()
	serverConn, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped), WithConnLogger(logs))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	_ = serverConn.Close()

	waitFor(t, 2*time.Second, "teardown after peer close", func() bool {
		return wrapped.closes.Load() == 1
	})

	if err := (&TCPCloser{}).Close(c); err != nil {
		t.Logf("close returned: %v", err)
	}

	if got := logs.infosContaining("Disconnect"); got != 0 {
		t.Errorf("Disconnect after peer close logged %d disconnect info message(s), want 0 (socket was already dead)", got)
	}
	if got := wrapped.closes.Load(); got != 1 {
		t.Errorf("underlying socket Close called %d time(s), want 1", got)
	}
	if got := logs.errorsContaining("Connection disconnected"); got != 1 {
		t.Errorf("got %d 'Connection disconnected' error log(s), want exactly 1", got)
	}
}

// TestDisconnectLiveSocketLogsExactlyOnce asserts that disconnecting a live
// connection logs the disconnect exactly once, and that repeated Disconnect
// calls add nothing.
func TestDisconnectLiveSocketLogsExactlyOnce(t *testing.T) {
	logs := newCaptureLogger()
	_, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped), WithConnLogger(logs))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	closer := &TCPCloser{}
	_ = closer.Close(c)
	if got := logs.infosContaining("Disconnected from Modbus TCP server"); got != 1 {
		t.Errorf("after first Disconnect: %d 'Disconnected' log(s), want 1", got)
	}

	_ = closer.Close(c)
	if got := logs.infosContaining("Disconnected from Modbus TCP server"); got != 1 {
		t.Errorf("after second Disconnect: %d 'Disconnected' log(s), want 1 (redundant Disconnect must be silent)", got)
	}
}

// TestReconnectAfterPeerClose reuses a single TCPConn across many
// connect/teardown cycles, alternating peer-initiated closes with local
// Disconnects, reconnecting immediately after teardown WITHOUT waiting for
// the previous cycle's goroutines to drain — goroutines left over from an old
// connection must never clobber the state of a newer one (the connection must
// stay connected once established). Run with -race: stale loops touching
// swapped connection state also show up here.
func TestReconnectAfterPeerClose(t *testing.T) {
	baseline := runtime.NumGoroutine()

	serverConn, clientConn := net.Pipe()
	sc := &swapConn{inner: clientConn}

	c := NewTCPConn("test", WithConn(sc))

	const cycles = 12
	for i := range cycles {
		if err := c.Connect(context.Background()); err != nil {
			t.Fatalf("cycle %d: connect: %v", i, err)
		}

		// A stale goroutine from a previous cycle must not mark the new
		// connection disconnected.
		time.Sleep(50 * time.Millisecond)
		if !c.IsConnected() {
			t.Fatalf("cycle %d: connection clobbered by stale goroutine (IsConnected false after fresh Connect)", i)
		}

		wantCloses := i + 1
		if i%2 == 0 {
			_ = serverConn.Close() // peer-initiated
		} else {
			_ = (&TCPCloser{}).Close(c) // local disconnect
		}

		// Wait only for the socket teardown (so the swap below can't race the
		// old generation's Close), NOT for the old goroutines to exit —
		// reconnecting while they still run is exactly the case being tested.
		waitFor(t, 2*time.Second, fmt.Sprintf("cycle %d: teardown (socket closed)", i), func() bool {
			return sc.closeCount() == wantCloses && !c.IsConnected()
		})

		serverConn, clientConn = net.Pipe()
		sc.swap(clientConn)
	}

	_ = (&TCPCloser{}).Close(c)
	waitFor(t, 3*time.Second, "all connection goroutines released after final disconnect", func() bool {
		return runtime.NumGoroutine() <= baseline
	})
}

// TestFailedConnectReleasesPoolMonitor asserts that a failed dial does not
// leak the transaction pool's timeout monitor goroutine. TCPConnector.Connect
// discards the TCPConn when the dial fails, so anything the constructor
// started must be stopped on the failure path — a reconnecting transport
// retrying an unreachable PLC would otherwise leak one goroutine per attempt,
// without bound.
func TestFailedConnectReleasesPoolMonitor(t *testing.T) {
	// Grab a port with nothing listening on it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	baseline := runtime.NumGoroutine()

	c := NewTCPConn("127.0.0.1", WithPort(port), WithTimeout(2*time.Second))
	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("connect unexpectedly succeeded")
	}

	waitFor(t, 3*time.Second, "pool monitor goroutine released after failed dial", func() bool {
		return runtime.NumGoroutine() <= baseline
	})
}

// reentrantLogger calls back into the TCPConn from inside Info/Error, the way
// a real logger might (e.g. attaching connection state to every entry). The
// conn must never invoke the logger while holding the mutex those callbacks
// need, or connect/teardown deadlocks.
type reentrantLogger struct {
	logging.Logger
	mu   sync.Mutex
	conn *TCPConn
}

func newReentrantLogger() *reentrantLogger {
	return &reentrantLogger{Logger: logging.NewNopLogger()}
}

func (l *reentrantLogger) attach(c *TCPConn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.conn = c
}

func (l *reentrantLogger) poke() {
	l.mu.Lock()
	c := l.conn
	l.mu.Unlock()
	if c != nil {
		c.IsConnected()
	}
}

func (l *reentrantLogger) Info(context.Context, string, ...any)  { l.poke() }
func (l *reentrantLogger) Error(context.Context, string, ...any) { l.poke() }

func (l *reentrantLogger) WithFields(map[string]any) logging.Logger { return l }

// runWithTimeout fails the test if fn does not return within the timeout —
// the deadlock detector for the reentrant-logger tests.
func runWithTimeout(t *testing.T, timeout time.Duration, name string, fn func()) {
	t.Helper()
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		fn()
	}()
	select {
	case <-doneCh:
	case <-time.After(timeout):
		t.Fatalf("%s deadlocked (did not return within %v)", name, timeout)
	}
}

// TestNoDeadlockWithReentrantLogger asserts that Connect, peer-close teardown,
// and Disconnect all complete when the logger calls back into the conn.
func TestNoDeadlockWithReentrantLogger(t *testing.T) {
	logs := newReentrantLogger()
	serverConn, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", WithConn(wrapped), WithConnLogger(logs))
	logs.attach(c)

	runWithTimeout(t, 2*time.Second, "Connect with reentrant logger", func() {
		if err := c.Connect(context.Background()); err != nil {
			t.Errorf("connect: %v", err)
		}
	})

	// Peer-initiated close: setDisconnected must log the drop without holding
	// the mutex the logger callback needs, or teardown never finishes.
	_ = serverConn.Close()
	waitFor(t, 2*time.Second, "teardown after peer close with reentrant logger", func() bool {
		return wrapped.closes.Load() == 1
	})

	runWithTimeout(t, 2*time.Second, "Disconnect with reentrant logger", func() {
		_ = (&TCPCloser{}).Close(c)
	})
}

// TestWriteChanPerGeneration asserts that each connection generation gets its
// own write channel. With a shared channel, a write loop from a previous
// generation that has not yet exited can steal a transaction queued by the
// new generation and complete it with ErrTransportClosing, resetting a
// healthy connection.
func TestWriteChanPerGeneration(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	sc := &swapConn{inner: clientConn}

	c := NewTCPConn("test", WithConn(sc))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	c.mutex.Lock()
	ch1 := c.writeChan
	c.mutex.Unlock()

	_ = (&TCPCloser{}).Close(c)
	_ = serverConn.Close()
	serverConn, clientConn = net.Pipe()
	_ = serverConn
	sc.swap(clientConn)

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	c.mutex.Lock()
	ch2 := c.writeChan
	c.mutex.Unlock()

	if ch1 == ch2 {
		t.Fatal("write channel shared across connection generations; stale write loops can steal new transactions")
	}

	_ = (&TCPCloser{}).Close(c)
}
