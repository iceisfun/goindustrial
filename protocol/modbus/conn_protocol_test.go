package modbus

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

// connectPipe creates a TCPConn connected over a net.Pipe and returns the
// conn, the peer (server) end, and the close-counting wrapper.
func connectPipe(t *testing.T, opts ...TCPConnOption) (*TCPConn, net.Conn, *countingConn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: clientConn}

	c := NewTCPConn("test", append([]TCPConnOption{WithConn(wrapped)}, opts...)...)
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Disconnect(context.Background())
		_ = serverConn.Close()
	})
	return c, serverConn, wrapped
}

// peerReadRequest reads one MBAP-framed request from the peer side and
// returns its transaction ID and PDU (function code + data).
func peerReadRequest(t *testing.T, conn net.Conn) (TransactionID, []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	header := make([]byte, TCPHeaderLength)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("peer: read request header: %v", err)
	}
	txID := TransactionID(binary.BigEndian.Uint16(header[0:2]))
	length := binary.BigEndian.Uint16(header[4:6])
	pdu := make([]byte, int(length)-1)
	if _, err := io.ReadFull(conn, pdu); err != nil {
		t.Fatalf("peer: read request PDU: %v", err)
	}
	return txID, pdu
}

// peerWriteFrame writes a raw MBAP frame from the peer side. The length
// field is supplied by the caller so tests can produce invalid framing.
func peerWriteFrame(t *testing.T, conn net.Conn, txID TransactionID, protoID uint16, length uint16, unitID byte, body []byte) {
	t.Helper()
	frame := make([]byte, TCPHeaderLength+len(body))
	binary.BigEndian.PutUint16(frame[0:2], uint16(txID))
	binary.BigEndian.PutUint16(frame[2:4], protoID)
	binary.BigEndian.PutUint16(frame[4:6], length)
	frame[6] = unitID
	copy(frame[TCPHeaderLength:], body)
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(frame); err != nil {
		t.Errorf("peer: write frame: %v", err)
	}
}

// peerWriteResponse writes a well-formed response for the given transaction.
func peerWriteResponse(t *testing.T, conn net.Conn, txID TransactionID, fc FunctionCode, data []byte) {
	t.Helper()
	body := append([]byte{byte(fc)}, data...)
	peerWriteFrame(t, conn, txID, uint16(TCPProtocolIdentifier), uint16(1+len(body)), 1, body)
}

func readHoldingRegistersRequest() *Request {
	return NewRequest(1, FuncReadHoldingRegisters, []byte{0x00, 0x00, 0x00, 0x01})
}

// TestSendNotConnected covers Send before Connect and after Disconnect.
func TestSendNotConnected(t *testing.T) {
	_, clientConn := net.Pipe()
	c := NewTCPConn("test", WithConn(&countingConn{Conn: clientConn}))

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Send before Connect: got %v, want ErrNotConnected", err)
	}

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	_ = c.Disconnect(context.Background())

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Send after Disconnect: got %v, want ErrNotConnected", err)
	}
}

// TestConnectAlreadyConnected covers the double-Connect guard.
func TestConnectAlreadyConnected(t *testing.T) {
	c, _, _ := connectPipe(t)
	if err := c.Connect(context.Background()); !errors.Is(err, ErrAlreadyConnected) {
		t.Fatalf("second Connect: got %v, want ErrAlreadyConnected", err)
	}
}

// TestSendRoundTrip is the conn-level happy path: a request goes out, the
// peer responds with matching MBAP framing, and Send returns the response.
func TestSendRoundTrip(t *testing.T) {
	c, peer, _ := connectPipe(t)

	go func() {
		txID, _ := peerReadRequest(t, peer)
		peerWriteResponse(t, peer, txID, FuncReadHoldingRegisters, []byte{0x02, 0x12, 0x34})
	}()

	resp, err := c.Send(context.Background(), readHoldingRegistersRequest())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Errorf("function code = %v, want %v", resp.GetPDU().FunctionCode, FuncReadHoldingRegisters)
	}
	if got := resp.GetPDU().Data; len(got) != 3 || got[1] != 0x12 || got[2] != 0x34 {
		t.Errorf("response data = %x, want 021234", got)
	}
}

// TestSendErrorOnInvalidProtocolID: a response with a bad MBAP protocol ID
// fails the matching transaction (via processError → ErrCh) and tears the
// connection down — a violated framing invariant means the stream can no
// longer be trusted. (The previous behavior, skipping the frame and reading
// on, silently desynchronized the stream because the invalid frame's body
// was never consumed.)
func TestSendErrorOnInvalidProtocolID(t *testing.T) {
	c, peer, wrapped := connectPipe(t)

	go func() {
		txID, _ := peerReadRequest(t, peer)
		// Header only: the client must reject the frame on the header alone
		// (it never consumed the body before this fix, which was the bug).
		peerWriteFrame(t, peer, txID, 0xDEAD, 2, 1, nil)
	}()

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); !errors.Is(err, ErrInvalidProtocolHeader) {
		t.Fatalf("got %v, want ErrInvalidProtocolHeader", err)
	}

	waitFor(t, 2*time.Second, "teardown after corrupt framing (protocol ID)", func() bool {
		return !c.IsConnected() && wrapped.closes.Load() == 1
	})
}

// TestSendErrorOnInvalidResponseLength: an MBAP length that leaves no room
// for a PDU fails the transaction and tears the connection down, like any
// other framing corruption.
func TestSendErrorOnInvalidResponseLength(t *testing.T) {
	c, peer, wrapped := connectPipe(t)

	go func() {
		txID, _ := peerReadRequest(t, peer)
		peerWriteFrame(t, peer, txID, uint16(TCPProtocolIdentifier), 1, 1, nil)
	}()

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); !errors.Is(err, ErrInvalidResponseLength) {
		t.Fatalf("got %v, want ErrInvalidResponseLength", err)
	}
	waitFor(t, 2*time.Second, "teardown after corrupt framing (length)", func() bool {
		return !c.IsConnected() && wrapped.closes.Load() == 1
	})
}

// TestResponseUnknownTransactionID: a response for a transaction the pool
// does not know is logged and dropped; the real response that follows must
// still complete the Send.
func TestResponseUnknownTransactionID(t *testing.T) {
	c, peer, _ := connectPipe(t)

	go func() {
		txID, _ := peerReadRequest(t, peer)
		peerWriteResponse(t, peer, txID+1000, FuncReadHoldingRegisters, []byte{0x02, 0x00, 0x00})
		peerWriteResponse(t, peer, txID, FuncReadHoldingRegisters, []byte{0x02, 0x00, 0x07})
	}()

	resp, err := c.Send(context.Background(), readHoldingRegistersRequest())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := resp.GetPDU().Data; len(got) != 3 || got[2] != 0x07 {
		t.Errorf("response data = %x, want 020007", got)
	}
}

// TestSendErrorOnTruncatedBody: the peer sends a valid header then closes
// mid-body. The transaction fails and the connection tears down (fd
// released) because the stream is no longer trustworthy.
func TestSendErrorOnTruncatedBody(t *testing.T) {
	c, peer, wrapped := connectPipe(t)

	go func() {
		txID, _ := peerReadRequest(t, peer)
		// Header claims 5 bytes follow the unit ID; send only 1 then close.
		peerWriteFrame(t, peer, txID, uint16(TCPProtocolIdentifier), 6, 1, []byte{byte(FuncReadHoldingRegisters)})
		_ = peer.Close()
	}()

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); err == nil {
		t.Fatal("send succeeded on truncated body, want error")
	}

	waitFor(t, 2*time.Second, "teardown after truncated body", func() bool {
		return !c.IsConnected() && wrapped.closes.Load() == 1
	})
}

// failWriteConn blocks reads (honoring deadlines, like a socket with nothing
// to deliver) but fails every write.
type failWriteConn struct {
	net.Conn
}

func (f *failWriteConn) Write([]byte) (int, error) {
	return 0, errors.New("write refused")
}

// TestSendErrorOnWriteFailure: a failed socket write fails the transaction
// and tears the connection down via the write loop's error path.
func TestSendErrorOnWriteFailure(t *testing.T) {
	_, clientConn := net.Pipe()
	wrapped := &countingConn{Conn: &failWriteConn{Conn: clientConn}}

	c := NewTCPConn("test", WithConn(wrapped))
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); err == nil {
		t.Fatal("send succeeded despite write failure, want error")
	}

	waitFor(t, 2*time.Second, "teardown after write failure", func() bool {
		return !c.IsConnected() && wrapped.closes.Load() == 1
	})
}

// TestResetTransactionsCancelsInflight: ResetTransactions fails in-flight
// Sends without disconnecting, and the connection remains usable.
func TestResetTransactionsCancelsInflight(t *testing.T) {
	c, peer, _ := connectPipe(t)

	// Peer consumes the request but never responds.
	go func() {
		_, _ = peerReadRequest(t, peer)
	}()

	errCh := make(chan error, 1)
	go func() {
		_, err := c.Send(context.Background(), readHoldingRegistersRequest())
		errCh <- err
	}()

	waitFor(t, 2*time.Second, "transaction placed", func() bool {
		c.mutex.Lock()
		pool := c.transactionPool
		c.mutex.Unlock()
		return pool.GetCount() == 1
	})

	c.ResetTransactions(context.Background())

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrTransportClosing) {
			t.Fatalf("in-flight Send got %v, want ErrTransportClosing", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight Send did not return after ResetTransactions")
	}

	if !c.IsConnected() {
		t.Fatal("ResetTransactions must not disconnect")
	}

	go func() {
		txID, _ := peerReadRequest(t, peer)
		peerWriteResponse(t, peer, txID, FuncReadHoldingRegisters, []byte{0x02, 0x00, 0x01})
	}()
	if _, err := c.Send(context.Background(), readHoldingRegistersRequest()); err != nil {
		t.Fatalf("send after reset: %v", err)
	}
}

// TestConcurrentSendsAllReturnOnDisconnect: with the peer neither reading nor
// responding (write loop blocked mid-write, more Sends queued behind it), a
// Disconnect must fail every outstanding Send promptly — none may hang.
func TestConcurrentSendsAllReturnOnDisconnect(t *testing.T) {
	c, _, wrapped := connectPipe(t)

	const senders = 8
	var wg sync.WaitGroup
	errs := make([]error, senders)
	for i := range senders {
		wg.Go(func() {
			_, errs[i] = c.Send(context.Background(), readHoldingRegistersRequest())
		})
	}

	// Let the write loop block on the unread pipe and the rest queue up.
	time.Sleep(50 * time.Millisecond)

	if err := c.Disconnect(context.Background()); err != nil {
		t.Logf("disconnect: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Sends still blocked 3s after Disconnect")
	}

	for i, err := range errs {
		if err == nil {
			t.Errorf("sender %d: Send succeeded across Disconnect, want error", i)
		}
	}
	if got := wrapped.closes.Load(); got != 1 {
		t.Errorf("socket closed %d time(s), want 1", got)
	}
}

// TestTCPCloserNilConn covers the nil guard.
func TestTCPCloserNilConn(t *testing.T) {
	if err := NewTCPCloser().Close(nil); err != nil {
		t.Fatalf("Close(nil) = %v, want nil", err)
	}
}

// TestConnectorDialFailure: a connector dial failure returns the error and
// releases everything the attempt allocated.
func TestConnectorDialFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	baseline := runtime.NumGoroutine()

	connector := NewTCPConnector("127.0.0.1", WithPort(port), WithTimeout(2*time.Second))
	conn, err := connector.Connect(context.Background())
	if err == nil {
		t.Fatal("connector dial unexpectedly succeeded")
	}
	if conn != nil {
		t.Fatalf("connector returned non-nil conn %v with error", conn)
	}

	waitFor(t, 3*time.Second, "no goroutines leaked by failed connector dial", func() bool {
		return runtime.NumGoroutine() <= baseline
	})
}

// TestRealTCPConnectorRoundTrip exercises the actual dial path (everything
// else here uses net.Pipe): connector dial, one request/response over a real
// TCP socket, then closer teardown.
func TestRealTCPConnectorRoundTrip(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	go func() {
		peer, err := l.Accept()
		if err != nil {
			return
		}
		defer peer.Close()
		txID, _ := peerReadRequest(t, peer)
		peerWriteResponse(t, peer, txID, FuncReadHoldingRegisters, []byte{0x02, 0xAB, 0xCD})
	}()

	connector := NewTCPConnector("127.0.0.1", WithPort(port), WithTimeout(2*time.Second))
	conn, err := connector.Connect(context.Background())
	if err != nil {
		t.Fatalf("connector dial: %v", err)
	}

	resp, err := conn.Send(context.Background(), readHoldingRegistersRequest())
	if err != nil {
		t.Fatalf("send over real TCP: %v", err)
	}
	if got := resp.GetPDU().Data; len(got) != 3 || got[1] != 0xAB || got[2] != 0xCD {
		t.Errorf("response data = %x, want 02abcd", got)
	}

	if err := NewTCPCloser().Close(conn); err != nil {
		t.Errorf("closer: %v", err)
	}
	if conn.IsConnected() {
		t.Error("still connected after closer")
	}
}

// TestTransactionPoolGetAndDoubleClose covers pool lookup and idempotent
// close.
func TestTransactionPoolGetAndDoubleClose(t *testing.T) {
	pool := NewTransactionPool()

	req := readHoldingRegistersRequest()
	tx, err := pool.Place(context.Background(), req)
	if err != nil {
		t.Fatalf("place: %v", err)
	}

	if got, ok := pool.Get(req.GetTransactionID()); !ok || got != tx {
		t.Fatalf("Get(%d) = %v, %v; want placed transaction", req.GetTransactionID(), got, ok)
	}
	if _, ok := pool.Get(req.GetTransactionID() + 1); ok {
		t.Fatal("Get of unknown ID reported ok")
	}

	pool.Close()
	pool.Close() // must be idempotent

	select {
	case err := <-tx.ErrCh:
		if !errors.Is(err, ErrTransportClosing) {
			t.Fatalf("pending tx got %v, want ErrTransportClosing", err)
		}
	default:
		t.Fatal("pending transaction not cancelled by pool close")
	}

	if _, err := pool.Place(context.Background(), readHoldingRegistersRequest()); err == nil {
		t.Fatal("Place on closed pool succeeded")
	}
}
