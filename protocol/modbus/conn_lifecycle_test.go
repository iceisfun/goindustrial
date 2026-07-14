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
// Disconnects. Each cycle must fully release the previous connection's
// resources, and goroutines left over from an old connection must never
// clobber the state of a newer one (the connection must stay connected once
// established). Run with -race: stale loops touching swapped connection state
// also show up here.
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

		waitFor(t, 2*time.Second, fmt.Sprintf("cycle %d: teardown (socket closed)", i), func() bool {
			return sc.closeCount() == wantCloses && !c.IsConnected()
		})
		waitFor(t, 3*time.Second, fmt.Sprintf("cycle %d: connection goroutines released", i), func() bool {
			return runtime.NumGoroutine() <= baseline
		})

		serverConn, clientConn = net.Pipe()
		sc.swap(clientConn)
	}
}
