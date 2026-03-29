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
		writeChan:       make(chan *Transaction, 100),
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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return ErrAlreadyConnected
	}

	c.logger.Info(ctx, "Connecting to Modbus TCP server at %s:%d", c.host, c.port)

	// Reset done channel if reconnecting.
	select {
	case <-c.done:
		c.done = make(chan struct{})
	default:
	}

	// Reset the transaction pool for a clean state.
	c.transactionPool.transactionsMu.Lock()
	c.transactionPool.unsafeReset()
	c.transactionPool.transactionsMu.Unlock()

	if c.writeChan == nil {
		c.writeChan = make(chan *Transaction, 100)
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
			c.logger.Error(ctx, "Failed to connect to %s: %v", addr, err)
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

	c.closeOnce = sync.Once{}
	c.connected = true

	c.logger.Info(ctx, "Connected to Modbus TCP server at %s:%d", c.host, c.port)

	go c.readLoop()
	go c.writeLoop()

	return nil
}

// Disconnect closes the TCP connection.
func (c *TCPConn) Disconnect(ctx context.Context) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return nil
	}

	c.logger.Info(ctx, "Disconnecting from Modbus TCP server")

	c.connected = false

	close(c.done)

	// Give goroutines a moment to notice the done channel.
	time.Sleep(10 * time.Millisecond)

	var err error
	c.closeOnce.Do(func() {
		c.transactionPool.transactionsMu.Lock()
		c.transactionPool.unsafeReset()
		c.transactionPool.transactionsMu.Unlock()

		if c.conn != nil {
			err = c.conn.Close()
		}
	})

	c.logger.Info(ctx, "Disconnected from Modbus TCP server")
	return err
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
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}

	c.logger.Debug(ctx, "Sending request: function=%d", request.GetPDU().FunctionCode)

	tx, err := c.transactionPool.Place(ctx, request)
	if err != nil {
		c.logger.Error(ctx, "Failed to create transaction: %v", err)
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	c.logger.Debug(ctx, "Created transaction %d", request.GetTransactionID())

	select {
	case c.writeChan <- tx:
		c.logger.Debug(ctx, "Queued transaction %d for writing", request.GetTransactionID())
	case <-ctx.Done():
		c.transactionPool.Release(request.GetTransactionID())
		return nil, ctx.Err()
	case <-c.done:
		c.transactionPool.Release(request.GetTransactionID())
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

	c.transactionPool.transactionsMu.Lock()
	defer c.transactionPool.transactionsMu.Unlock()

	c.transactionPool.unsafeReset()

	c.logger.Info(ctx, "Transaction pool has been reset")
}

// readLoop continuously reads responses from the TCP connection.
func (c *TCPConn) readLoop() {
	ctx := context.Background()
	c.logger.Debug(ctx, "Starting read loop")

	defer func() {
		c.logger.Debug(ctx, "Exiting read loop")
		c.setDisconnected(fmt.Errorf("read loop exited"))
	}()

	readTimeout := 100 * time.Millisecond

	for {
		select {
		case <-c.done:
			return
		default:
			if !c.IsConnected() {
				return
			}

			if deadline, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
				deadline.SetReadDeadline(time.Now().Add(readTimeout))
			}

			header := make([]byte, TCPHeaderLength)
			_, err := io.ReadFull(c.reader, header)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-c.done:
						return
					default:
						continue
					}
				}

				select {
				case <-c.done:
					return
				default:
					c.logger.Error(ctx, "Error reading header: %v", err)
					c.setDisconnected(fmt.Errorf("read error: %w", err))
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

			if protocolID != TCPProtocolIdentifier {
				c.logger.Error(ctx, "Invalid protocol ID: %d", protocolID)
				c.processError(transactionID, ErrInvalidProtocolHeader)
				continue
			}

			bodyLength := int(length) - 1
			if bodyLength <= 0 {
				c.logger.Error(ctx, "Invalid response length: %d", length)
				c.processError(transactionID, ErrInvalidResponseLength)
				continue
			}

			body := make([]byte, bodyLength)
			_, err = io.ReadFull(c.reader, body)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-c.done:
						return
					default:
						continue
					}
				}

				select {
				case <-c.done:
					return
				default:
					c.logger.Error(ctx, "Error reading body: %v", err)
					c.processError(transactionID, fmt.Errorf("read body error: %w", err))
					c.setDisconnected(err)
					return
				}
			}

			if hexLogger, ok := c.logger.(logging.HexdumpLogger); ok {
				hexLogger.Hexdump(ctx, body)
			}

			functionCode := FunctionCode(body[0])
			responseData := body[1:]
			response := NewResponse(transactionID, unitID, functionCode, responseData)

			tx, ok := c.transactionPool.Release(transactionID)
			if !ok {
				c.logger.Warn(ctx, "Received response for unknown transaction ID: %d", transactionID)
				continue
			}

			c.logger.Debug(ctx, "Completing transaction %d", transactionID)
			tx.Complete(response, nil)
		}
	}
}

// writeLoop continuously processes requests from the writeChan.
func (c *TCPConn) writeLoop() {
	ctx := context.Background()
	c.logger.Debug(ctx, "Starting write loop")

	defer func() {
		c.logger.Debug(ctx, "Exiting write loop")
		c.setDisconnected(fmt.Errorf("write loop exited"))
	}()

	for {
		if !c.IsConnected() {
			return
		}

		select {
		case <-c.done:
			return
		case tx, ok := <-c.writeChan:
			if !ok {
				return
			}

			if !c.IsConnected() {
				tx.Complete(nil, ErrNotConnected)
				return
			}

			select {
			case <-tx.Context().Done():
				c.logger.Debug(ctx, "Transaction %d was cancelled before writing",
					tx.Request.GetTransactionID())
				continue
			case <-c.done:
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
			case <-c.done:
				tx.Complete(nil, ErrTransportClosing)
				return
			default:
			}

			_, err = c.writer.Write(data)
			if err != nil {
				select {
				case <-c.done:
					tx.Complete(nil, ErrTransportClosing)
					return
				default:
					c.logger.Error(ctx, "Error writing request: %v", err)
					tx.Complete(nil, err)
					c.setDisconnected(fmt.Errorf("write error: %w", err))
					return
				}
			}

			c.logger.Debug(ctx, "Wrote request for transaction %d",
				tx.Request.GetTransactionID())
		}
	}
}

// processError handles errors for a specific transaction.
func (c *TCPConn) processError(txID TransactionID, err error) {
	ctx := context.Background()
	if tx, ok := c.transactionPool.Release(txID); ok {
		c.logger.Debug(ctx, "Processing error for transaction %d: %v", txID, err)
		tx.Complete(nil, err)
	} else {
		c.logger.Warn(ctx, "Error for unknown transaction %d: %v", txID, err)
	}
}

// setDisconnected marks the connection as disconnected.
func (c *TCPConn) setDisconnected(err error) {
	ctx := context.Background()
	c.mutex.Lock()
	wasConnected := c.connected
	c.connected = false
	c.mutex.Unlock()

	if wasConnected {
		c.logger.Error(ctx, "Connection disconnected: %v", err)

		c.transactionPool.transactionsMu.Lock()
		c.transactionPool.unsafeReset()
		c.transactionPool.transactionsMu.Unlock()
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
