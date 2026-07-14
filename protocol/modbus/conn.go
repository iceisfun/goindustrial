package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/hexdump"
	"github.com/iceisfun/goindustrial/logging"
)

// TCPConn is a Modbus TCP connection that manages a single TCP socket with
// concurrent read/write goroutines and a [TransactionPool] for matching
// requests to responses via MBAP transaction IDs. It is the connection type
// used by transport.Transport[*TCPConn].
type TCPConn struct {
	logger          logging.Logger
	host            string
	port            int
	timeout         time.Duration
	hexDumper       *hexdump.Dumper   // optional: set via WithHexDump
	injectedConn    net.Conn          // optional: injected via WithConn for testing
	conn            net.Conn          // active TCP connection
	reader          io.Reader
	writer          io.Writer
	mutex           sync.Mutex
	connected       bool
	closeOnce       sync.Once
	gen             uint64 // connection generation; guards against stale goroutines
	transactionPool *TransactionPool
	writeChan       chan *Transaction
	done            chan struct{}
}

// NewTCPConn creates a new, unconnected TCPConn for the given host. Call
// [TCPConn.Connect] to establish the TCP connection and start the read/write
// goroutines. Use [TCPConnOption] values to configure port, timeout, and logger.
func NewTCPConn(host string, options ...TCPConnOption) *TCPConn {
	c := &TCPConn{
		logger:          logging.NewNopLogger(),
		host:            host,
		port:            DefaultTCPPort,
		timeout:         30 * time.Second,
		connected:       false,
		transactionPool: NewTransactionPool(),
		done:            make(chan struct{}),
	}

	for _, option := range options {
		option(c)
	}

	return c
}

// Connect establishes the TCP connection and starts read/write goroutines.
// If a net.Conn was injected via WithConn, it is used directly instead of dialing.
func (c *TCPConn) Connect(ctx context.Context) error {
	// All logging happens outside the mutex: a logger callback that touches
	// the conn (e.g. IsConnected) must not deadlock against it.
	c.logger.Info(ctx, "Connecting to Modbus TCP server at %s:%d", c.host, c.port)

	if err := c.connect(ctx); err != nil {
		c.logger.Error(ctx, "Failed to connect to %s:%d: %v", c.host, c.port, err)
		return err
	}

	c.logger.Info(ctx, "Connected to Modbus TCP server at %s:%d", c.host, c.port)
	return nil
}

// connect is Connect without the logging; it holds c.mutex for the duration.
func (c *TCPConn) connect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return ErrAlreadyConnected
	}

	// The previous connection's teardown closed its transaction pool (stopping
	// the timeout monitor), so a reconnect needs a fresh one.
	select {
	case <-c.transactionPool.done:
		c.transactionPool = NewTransactionPool()
	default:
		// Fresh pool from the constructor; reset for a clean state.
		c.transactionPool.transactionsMu.Lock()
		c.transactionPool.unsafeReset()
		c.transactionPool.transactionsMu.Unlock()
	}

	if c.injectedConn != nil {
		// Use the injected connection (e.g. from net.Pipe).
		c.conn = c.injectedConn
	} else {
		// Dial TCP.
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(c.timeout)
		}

		dialer := net.Dialer{
			Timeout: time.Until(deadline),
		}

		addr := fmt.Sprintf("%s:%d", c.host, c.port)
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			// The conn may be discarded after a failed dial (TCPConnector
			// does exactly that on every transport retry), so stop the pool's
			// timeout monitor here or it leaks one goroutine per attempt.
			c.transactionPool.Close()
			return err
		}

		c.conn = conn
	}

	c.reader = io.Reader(c.conn)
	c.writer = io.Writer(c.conn)
	if c.hexDumper != nil {
		c.reader = c.hexDumper.WrapReader(c.reader)
		c.writer = c.hexDumper.WrapWriter(c.writer)
	}

	// Each connection is a new generation with its own done channel, write
	// channel, and re-armed closeOnce. The loops receive their generation's
	// state as parameters so goroutines from a previous connection can never
	// touch — or be confused by — the state of a newer one. The write channel
	// in particular must not be shared: a stale write loop that has not yet
	// exited would race the new loop for queued transactions and complete
	// stolen ones with ErrTransportClosing.
	c.gen++
	c.done = make(chan struct{})
	c.writeChan = make(chan *Transaction, 100)
	c.closeOnce = sync.Once{}
	c.connected = true

	go c.readLoop(c.gen, c.done, c.conn, c.reader, c.transactionPool)
	go c.writeLoop(c.gen, c.done, c.writer, c.writeChan)

	return nil
}

// Disconnect closes the TCP connection. If the connection was already torn
// down (e.g. the peer closed it and the read loop cleaned up), Disconnect is
// a silent no-op.
func (c *TCPConn) Disconnect(ctx context.Context) error {
	c.mutex.Lock()
	c.connected = false
	closedSocket, err := c.unsafeTeardown()
	c.mutex.Unlock()

	// Log outside the mutex so a logger callback into the conn cannot
	// deadlock.
	if closedSocket {
		c.logger.Info(ctx, "Disconnected from Modbus TCP server")
	}
	return err
}

// unsafeTeardown releases everything the current connection generation holds:
// it closes the done channel (stopping the read/write loops), closes the
// transaction pool (cancelling in-flight transactions and stopping the
// timeout monitor goroutine), and closes the socket. It runs at most once per
// generation — Connect re-arms closeOnce along with c.done — so it is safe to
// call from both Disconnect and setDisconnected regardless of who observed
// the drop first; the fd is released exactly once and close(c.done) cannot
// panic on a double close. Returns whether this call closed a live socket.
// Caller must hold c.mutex.
func (c *TCPConn) unsafeTeardown() (closedSocket bool, err error) {
	c.closeOnce.Do(func() {
		close(c.done)

		c.transactionPool.Close()

		if c.conn != nil {
			closedSocket = true
			err = c.conn.Close()
		}
	})

	return closedSocket, err
}

// IsConnected returns true if the connection is active.
func (c *TCPConn) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connected
}

// Send sends a Modbus request and blocks until the matching response arrives,
// the context expires, or the connection is closed. The MBAP transaction ID is
// assigned automatically by the underlying [TransactionPool].
func (c *TCPConn) Send(ctx context.Context, request *Request) (*Response, error) {
	// Snapshot the current generation's state under the mutex: Connect
	// replaces done, writeChan, and transactionPool on reconnect.
	c.mutex.Lock()
	connected := c.connected
	done := c.done
	writeChan := c.writeChan
	pool := c.transactionPool
	c.mutex.Unlock()

	if !connected {
		return nil, ErrNotConnected
	}

	c.logger.Debug(ctx, "Sending request: function=%d", request.GetPDU().FunctionCode)

	tx, err := pool.Place(ctx, request)
	if err != nil {
		c.logger.Error(ctx, "Failed to create transaction: %v", err)
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	c.logger.Debug(ctx, "Created transaction %d", request.GetTransactionID())

	select {
	case writeChan <- tx:
		c.logger.Debug(ctx, "Queued transaction %d for writing", request.GetTransactionID())
	case <-ctx.Done():
		pool.Release(request.GetTransactionID())
		return nil, ctx.Err()
	case <-done:
		pool.Release(request.GetTransactionID())
		return nil, ErrTransportClosing
	}

	select {
	case response := <-tx.ResponseCh:
		c.logger.Debug(ctx, "Received response for transaction %d", request.GetTransactionID())
		return response, nil
	case err := <-tx.ErrCh:
		c.logger.Debug(ctx, "Received error for transaction %d: %v", request.GetTransactionID(), err)
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ResetTransactions cancels all in-flight transactions and resets the
// transaction pool without closing the TCP connection.
func (c *TCPConn) ResetTransactions(ctx context.Context) {
	c.logger.Info(ctx, "Resetting transaction pool")

	c.mutex.Lock()
	pool := c.transactionPool
	c.mutex.Unlock()

	pool.transactionsMu.Lock()
	defer pool.transactionsMu.Unlock()

	pool.unsafeReset()

	c.logger.Info(ctx, "Transaction pool has been reset")
}

// readLoop continuously reads responses from the TCP connection. It receives
// its generation's state as parameters rather than reading TCPConn fields, so
// a loop from a previous connection can never observe (or act on) the state
// of a newer one after a reconnect.
func (c *TCPConn) readLoop(gen uint64, done chan struct{}, conn net.Conn, reader io.Reader, pool *TransactionPool) {
	ctx := context.Background()
	c.logger.Debug(ctx, "Starting read loop")

	defer func() {
		c.logger.Debug(ctx, "Exiting read loop")
		c.setDisconnected(gen, fmt.Errorf("read loop exited"))
	}()

	readTimeout := 100 * time.Millisecond

	for {
		select {
		case <-done:
			return
		default:
			if deadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
				deadline.SetReadDeadline(time.Now().Add(readTimeout))
			}

			header := make([]byte, TCPHeaderLength)
			_, err := io.ReadFull(reader, header)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-done:
						return
					default:
						continue
					}
				}

				select {
				case <-done:
					return
				default:
					c.logger.Error(ctx, "Error reading header: %v", err)
					c.setDisconnected(gen, fmt.Errorf("read error: %w", err))
					return
				}
			}

			if hexLogger, ok := c.logger.(logging.HexdumpLogger); ok {
				hexLogger.Hexdump(ctx, header)
			}

			transactionID := TransactionID(binary.BigEndian.Uint16(header[0:2]))
			protocolID := ProtocolID(binary.BigEndian.Uint16(header[2:4]))
			length := binary.BigEndian.Uint16(header[4:6])
			unitID := UnitID(header[6])

			c.logger.Debug(ctx, "Received response: txID=%d, length=%d", transactionID, length)

			// A violated MBAP framing invariant means the byte stream can no
			// longer be trusted: continuing would parse subsequent responses
			// from an arbitrary offset (the old `continue` here never consumed
			// the invalid frame's body, silently desynchronizing the stream).
			// Fail the transaction and tear the connection down; the
			// reconnecting transport recovers with a clean stream.
			if protocolID != TCPProtocolIdentifier {
				c.logger.Error(ctx, "Invalid protocol ID: %d", protocolID)
				c.processError(pool, transactionID, ErrInvalidProtocolHeader)
				c.setDisconnected(gen, ErrInvalidProtocolHeader)
				return
			}

			bodyLength := int(length) - 1
			if bodyLength <= 0 {
				c.logger.Error(ctx, "Invalid response length: %d", length)
				c.processError(pool, transactionID, ErrInvalidResponseLength)
				c.setDisconnected(gen, ErrInvalidResponseLength)
				return
			}

			body := make([]byte, bodyLength)
			_, err = io.ReadFull(reader, body)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-done:
						return
					default:
						continue
					}
				}

				select {
				case <-done:
					return
				default:
					c.logger.Error(ctx, "Error reading body: %v", err)
					c.processError(pool, transactionID, fmt.Errorf("read body error: %w", err))
					c.setDisconnected(gen, err)
					return
				}
			}

			if hexLogger, ok := c.logger.(logging.HexdumpLogger); ok {
				hexLogger.Hexdump(ctx, body)
			}

			functionCode := FunctionCode(body[0])
			responseData := body[1:]
			response := NewResponse(transactionID, unitID, functionCode, responseData)

			tx, ok := pool.Release(transactionID)
			if !ok {
				c.logger.Warn(ctx, "Received response for unknown transaction ID: %d", transactionID)
				continue
			}

			c.logger.Debug(ctx, "Completing transaction %d", transactionID)
			tx.Complete(response, nil)
		}
	}
}

// writeLoop continuously processes requests from its generation's write
// channel. Like readLoop, it receives its generation's state as parameters;
// the done channel — closed exactly once per generation by unsafeTeardown —
// is the single signal that this loop's connection is gone.
func (c *TCPConn) writeLoop(gen uint64, done chan struct{}, writer io.Writer, writeChan <-chan *Transaction) {
	ctx := context.Background()
	c.logger.Debug(ctx, "Starting write loop")

	defer func() {
		c.logger.Debug(ctx, "Exiting write loop")
		c.setDisconnected(gen, fmt.Errorf("write loop exited"))
	}()

	for {
		select {
		case <-done:
			return
		case tx, ok := <-writeChan:
			if !ok {
				return
			}

			select {
			case <-tx.Context().Done():
				c.logger.Debug(ctx, "Transaction %d was cancelled before writing",
					tx.Request.GetTransactionID())
				continue
			case <-done:
				tx.Complete(nil, ErrTransportClosing)
				return
			default:
			}

			c.logger.Debug(ctx, "Writing request for transaction %d",
				tx.Request.GetTransactionID())

			data, err := tx.Request.Encode()
			if err != nil {
				c.logger.Error(ctx, "Error encoding request: %v", err)
				tx.Complete(nil, err)
				continue
			}

			if hexLogger, ok := c.logger.(logging.HexdumpLogger); ok {
				hexLogger.Hexdump(ctx, data)
			}

			select {
			case <-done:
				tx.Complete(nil, ErrTransportClosing)
				return
			default:
			}

			_, err = writer.Write(data)
			if err != nil {
				select {
				case <-done:
					tx.Complete(nil, ErrTransportClosing)
					return
				default:
					c.logger.Error(ctx, "Error writing request: %v", err)
					tx.Complete(nil, err)
					c.setDisconnected(gen, fmt.Errorf("write error: %w", err))
					return
				}
			}

			c.logger.Debug(ctx, "Wrote request for transaction %d",
				tx.Request.GetTransactionID())
		}
	}
}

// processError handles errors for a specific transaction.
func (c *TCPConn) processError(pool *TransactionPool, txID TransactionID, err error) {
	ctx := context.Background()
	if tx, ok := pool.Release(txID); ok {
		c.logger.Debug(ctx, "Processing error for transaction %d: %v", txID, err)
		tx.Complete(nil, err)
	} else {
		c.logger.Warn(ctx, "Error for unknown transaction %d: %v", txID, err)
	}
}

// setDisconnected marks the connection as disconnected and tears it down,
// releasing the socket and pool even when the drop was peer-initiated and no
// Disconnect call ever arrives. gen is the connection generation the calling
// goroutine belongs to: a call from a loop of a previous connection is
// ignored, so a lingering goroutine can never clobber the state of a newer
// connection established by a subsequent Connect.
func (c *TCPConn) setDisconnected(gen uint64, err error) {
	ctx := context.Background()

	c.mutex.Lock()
	if gen != c.gen {
		c.mutex.Unlock()
		return
	}
	wasConnected := c.connected
	c.connected = false
	c.unsafeTeardown()
	c.mutex.Unlock()

	// Log outside the mutex so a logger callback into the conn cannot
	// deadlock the teardown.
	if wasConnected {
		c.logger.Error(ctx, "Connection disconnected: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TCPConnector implements transport.Connector[*TCPConn]
// ---------------------------------------------------------------------------

// TCPConnector creates and connects [TCPConn] instances. It implements
// transport.Connector[*TCPConn] and is used by the reconnecting transport
// to establish new connections on demand.
type TCPConnector struct {
	host    string
	options []TCPConnOption
}

// NewTCPConnector creates a new TCPConnector for the given host with optional
// connection settings.
func NewTCPConnector(host string, options ...TCPConnOption) *TCPConnector {
	return &TCPConnector{
		host:    host,
		options: options,
	}
}

// Connect creates a new TCPConn, connects it, and returns it.
func (tc *TCPConnector) Connect(ctx context.Context) (*TCPConn, error) {
	conn := NewTCPConn(tc.host, tc.options...)
	if err := conn.Connect(ctx); err != nil {
		return nil, err
	}
	return conn, nil
}

// ---------------------------------------------------------------------------
// TCPCloser implements transport.Closer[*TCPConn]
// ---------------------------------------------------------------------------

// TCPCloser disconnects a [TCPConn]. It implements transport.Closer[*TCPConn]
// and is used by the reconnecting transport to tear down stale connections.
type TCPCloser struct{}

// NewTCPCloser creates a new TCPCloser.
func NewTCPCloser() *TCPCloser {
	return &TCPCloser{}
}

// Close disconnects the given TCPConn.
func (tc *TCPCloser) Close(conn *TCPConn) error {
	if conn == nil {
		return nil
	}
	return conn.Disconnect(context.Background())
}
