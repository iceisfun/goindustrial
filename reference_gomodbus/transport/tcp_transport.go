package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
)

// TCPTransport implements the common.Transport interface for Modbus TCP
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
type TCPTransport struct {
	logger          common.LoggerInterface
	host            string                 // Server hostname/IP
	port            int                    // TCP port (default: 502, per spec Section 4.1)
	timeout         time.Duration          // Connection timeout
	conn            net.Conn               // TCP connection
	reader          io.Reader              // For reading data from the connection
	writer          io.Writer              // For writing data to the connection
	mutex           sync.Mutex             // Protects access to connection state
	connected       bool                   // Indicates if we have an active connection
	closeOnce       sync.Once              // Ensures we only close the connection once
	transactionPool *TransactionPool       // Manages transaction IDs and responses
	writeChan       chan *Transaction      // Channel for queuing write operations
	done            chan struct{}          // Signals shutdown of goroutines
}

// TCPTransportOption is a function that configures a TCPTransport
type TCPTransportOption func(*TCPTransport)

// WithPort sets the TCP port
func WithPort(port int) TCPTransportOption {
	return func(t *TCPTransport) {
		t.port = port
	}
}

// WithTimeout sets the timeout duration
func WithTimeoutOption(timeout time.Duration) TCPTransportOption {
	return func(t *TCPTransport) {
		t.timeout = timeout
	}
}

// WithReader sets the reader
func WithReader(reader io.Reader) TCPTransportOption {
	return func(t *TCPTransport) {
		t.reader = reader
	}
}

// WithWriter sets the writer
func WithWriter(writer io.Writer) TCPTransportOption {
	return func(t *TCPTransport) {
		t.writer = writer
	}
}

// WithTransportLogger sets the logger for the transport
func WithTransportLogger(logger common.LoggerInterface) TCPTransportOption {
	return func(t *TCPTransport) {
		t.logger = logger
	}
}

// NewTCPTransport creates a new TCPTransport
func NewTCPTransport(host string, options ...TCPTransportOption) *TCPTransport {
	t := &TCPTransport{
		logger:          logging.NewLogger(),
		host:            host,
		port:            common.DefaultTCPPort,
		timeout:         30 * time.Second,
		connected:       false,
		transactionPool: NewTransactionPool(),
		writeChan:       make(chan *Transaction, 100),
		done:            make(chan struct{}),
	}

	for _, option := range options {
		option(t)
	}

	return t
}

// WithLogger sets the logger for the transport and returns the modified transport
func (t *TCPTransport) WithLogger(logger common.LoggerInterface) common.Transport {
	t.logger = logger
	return t
}

// Connect establishes a connection to the Modbus TCP server
func (t *TCPTransport) Connect(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.connected {
		return common.ErrAlreadyConnected
	}

	t.logger.Info(ctx, "Connecting to Modbus TCP server at %s:%d", t.host, t.port)

	// Reset channels if we're reconnecting
	select {
	case <-t.done:
		// done channel is closed, we need to recreate it
		t.done = make(chan struct{})
	default:
		// done channel is still open, nothing to do
	}

	// Reset the transaction pool to ensure clean state during reconnection
	t.transactionPool.transactionsMu.Lock()
	t.transactionPool.unsafeReset()
	t.transactionPool.transactionsMu.Unlock()

	// Re-initialize write channel if needed
	if t.writeChan == nil {
		t.writeChan = make(chan *Transaction, 100)
	}

	// Get deadline from context or use default timeout
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(t.timeout)
	}

	// Connect with timeout
	dialer := net.Dialer{
		Timeout: time.Until(deadline),
	}

	addr := fmt.Sprintf("%s:%d", t.host, t.port)
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.logger.Error(ctx, "Failed to connect to %s: %v", addr, err)
		return err
	}

	t.conn = conn

	// If no custom reader/writer was provided, use the connection
	if t.reader == nil {
		t.reader = t.conn
	}
	if t.writer == nil {
		t.writer = t.conn
	}

	// Reset the closeOnce for reconnection
	t.closeOnce = sync.Once{}

	t.connected = true

	t.logger.Info(ctx, "Connected to Modbus TCP server at %s:%d", t.host, t.port)

	// Start the read and write goroutines
	go t.readLoop()
	go t.writeLoop()

	return nil
}

// Disconnect closes the connection to the Modbus TCP server
func (t *TCPTransport) Disconnect(ctx context.Context) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.connected {
		return nil
	}

	t.logger.Info(ctx, "Disconnecting from Modbus TCP server")

	// Mark as disconnected first to prevent new operations
	t.connected = false

	// Signal goroutines to exit
	close(t.done)

	// Give readLoop and writeLoop a moment to notice the done channel has been closed
	// This helps prevent "use of closed network connection" errors
	time.Sleep(10 * time.Millisecond)

	var err error
	t.closeOnce.Do(func() {
		// Reset the transaction pool instead of closing it
		// This will automatically cancel all pending transactions
		t.transactionPool.transactionsMu.Lock()
		t.transactionPool.unsafeReset()
		t.transactionPool.transactionsMu.Unlock()

		// Close the connection
		if t.conn != nil {
			err = t.conn.Close()
		}
	})

	t.logger.Info(ctx, "Disconnected from Modbus TCP server")
	return err
}

// IsConnected returns true if connected to the server
func (t *TCPTransport) IsConnected() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.connected
}

// ResetTransactions resets the transaction pool without disconnecting
// This can be useful to recover from certain error states where the connection
// is still valid but the transaction state may be corrupted
func (t *TCPTransport) ResetTransactions(ctx context.Context) {
	t.logger.Info(ctx, "Resetting transaction pool")

	t.transactionPool.transactionsMu.Lock()
	defer t.transactionPool.transactionsMu.Unlock()

	// Use unsafeReset to completely reinitialize the transaction pool
	// This will cancel all pending transactions, clear the map, and reset the freeIDs
	t.transactionPool.unsafeReset()

	t.logger.Info(ctx, "Transaction pool has been reset")
}

// readLoop continuously reads from the connection and handles responses
// This implements the client side of the Modbus TCP protocol
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
func (t *TCPTransport) readLoop() {
	ctx := context.Background()
	t.logger.Debug(ctx, "Starting read loop")

	defer func() {
		t.logger.Debug(ctx, "Exiting read loop")
		t.setDisconnected(fmt.Errorf("read loop exited"))
	}()

	// Set a read deadline to ensure we don't block too long on read operations
	// This allows us to check the done channel more frequently
	readTimeout := 100 * time.Millisecond

	for {
		select {
		case <-t.done:
			return
		default:
			// Check if we're still connected
			if !t.IsConnected() {
				return
			}

			// Set a deadline for this read operation
			if deadline, ok := t.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
				deadline.SetReadDeadline(time.Now().Add(readTimeout))
			}

			// Read the response header (7 bytes)
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
			// MBAP Header is 7 bytes: Transaction ID (2), Protocol ID (2), Length (2), Unit ID (1)
			header := make([]byte, common.TCPHeaderLength)
			_, err := io.ReadFull(t.reader, header)
			if err != nil {
				// Check if this is a timeout error (which is expected during shutdown)
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// This is a timeout, check if we should exit
					select {
					case <-t.done:
						return
					default:
						// Continue the loop and try again
						continue
					}
				}

				// If we're already disconnected or shutting down, just exit
				select {
				case <-t.done:
					return
				default:
					// Otherwise, log and report the error
					t.logger.Error(ctx, "Error reading header: %v", err)
					t.setDisconnected(fmt.Errorf("read error: %w", err))
					return
				}
			}

			// If logger implements Hexdump and we're at trace level, log the header
			if hexLogger, ok := t.logger.(common.LoggerInterfaceHexdump); ok {
				hexLogger.Hexdump(ctx, header)
			}

			// Parse the MBAP header
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1, Table 3
			// Field 1: Transaction Identifier (2 bytes)
			transactionID := common.TransactionID(binary.BigEndian.Uint16(header[0:2]))
			// Field 2: Protocol Identifier (2 bytes) - Always 0 for Modbus
			protocolID := common.ProtocolID(binary.BigEndian.Uint16(header[2:4]))
			// Field 3: Length (2 bytes) - Number of bytes following
			length := binary.BigEndian.Uint16(header[4:6])
			// Field 4: Unit Identifier (1 byte) - Slave address
			unitID := common.UnitID(header[6])

			t.logger.Debug(ctx, "Received response: txID=%d, length=%d", transactionID, length)

			// Check ProtocolID - should be 0 for Modbus TCP
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1
			if protocolID != common.TCPProtocolIdentifier {
				t.logger.Error(ctx, "Invalid protocol ID: %d", protocolID)
				t.processError(transactionID, common.ErrInvalidProtocolHeader)
				continue
			}

			// Length is the number of bytes following (Unit ID + PDU)
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1
			// We already read the unit ID, so we need length-1 more bytes
			bodyLength := int(length) - 1
			if bodyLength <= 0 {
				t.logger.Error(ctx, "Invalid response length: %d", length)
				t.processError(transactionID, common.ErrInvalidResponseLength)
				continue
			}

			// Read the function code and data (PDU)
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (MODBUS Function Codes)
			body := make([]byte, bodyLength)
			_, err = io.ReadFull(t.reader, body)
			if err != nil {
				// Check if this is a timeout or if we're shutting down
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// This is a timeout, check if we should exit
					select {
					case <-t.done:
						return
					default:
						// Otherwise continue
						continue
					}
				}

				// If we're shutting down, just exit
				select {
				case <-t.done:
					return
				default:
					// Otherwise, log and report the error
					t.logger.Error(ctx, "Error reading body: %v", err)
					t.processError(transactionID, fmt.Errorf("read body error: %w", err))
					t.setDisconnected(err)
					return
				}
			}

			// If logger implements Hexdump and we're at trace level, log the body
			if hexLogger, ok := t.logger.(common.LoggerInterfaceHexdump); ok {
				hexLogger.Hexdump(ctx, body)
			}

			// Create a response
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (MODBUS Function Codes)
			// The first byte of the PDU is the function code
			functionCode := common.FunctionCode(body[0])
			// The rest is the response data specific to that function code
			responseData := body[1:]
			response := NewResponse(transactionID, unitID, functionCode, responseData)

			// Find and complete the transaction
			tx, ok := t.transactionPool.Release(transactionID)
			if !ok {
				t.logger.Warn(ctx, "Received response for unknown transaction ID: %d", transactionID)
				continue
			}

			t.logger.Debug(ctx, "Completing transaction %d", transactionID)
			// Complete the transaction with the response
			tx.Complete(response, nil)
		}
	}
}

// writeLoop continuously processes requests from the writeChan
// This implements the client side of sending Modbus TCP requests
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
func (t *TCPTransport) writeLoop() {
	ctx := context.Background()
	t.logger.Debug(ctx, "Starting write loop")

	defer func() {
		t.logger.Debug(ctx, "Exiting write loop")
		t.setDisconnected(fmt.Errorf("write loop exited"))
	}()

	for {
		// First check if we're still connected
		if !t.IsConnected() {
			return
		}

		select {
		case <-t.done:
			return
		case tx, ok := <-t.writeChan:
			// Check if the channel was closed
			if !ok {
				return
			}

			// Check if we're still connected
			if !t.IsConnected() {
				tx.Complete(nil, common.ErrNotConnected)
				return
			}

			// Check if the transaction is still valid
			select {
			case <-tx.Context().Done():
				t.logger.Debug(ctx, "Transaction %d was cancelled before writing",
					tx.Request.GetTransactionID())
				continue
			case <-t.done:
				// Transport is shutting down
				tx.Complete(nil, common.ErrTransportClosing)
				return
			default:
				// Transaction is still valid
			}

			t.logger.Debug(ctx, "Writing request for transaction %d",
				tx.Request.GetTransactionID())

			// Encode the request
			// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
			// This will create the MBAP header and PDU according to the Modbus specification
			data, err := tx.Request.Encode()
			if err != nil {
				t.logger.Error(ctx, "Error encoding request: %v", err)
				tx.Complete(nil, err)
				continue
			}

			// If logger implements Hexdump and we're at trace level, log the encoded request
			if hexLogger, ok := t.logger.(common.LoggerInterfaceHexdump); ok {
				hexLogger.Hexdump(ctx, data)
			}

			// Check again if we should exit before writing
			select {
			case <-t.done:
				tx.Complete(nil, common.ErrTransportClosing)
				return
			default:
				// Continue with the write
			}

			// Write the request
			_, err = t.writer.Write(data)
			if err != nil {
				// If we're shutting down, don't report the error
				select {
				case <-t.done:
					tx.Complete(nil, common.ErrTransportClosing)
					return
				default:
					// Otherwise, log and report the error
					t.logger.Error(ctx, "Error writing request: %v", err)
					tx.Complete(nil, err)
					t.setDisconnected(fmt.Errorf("write error: %w", err))
					return
				}
			}

			t.logger.Debug(ctx, "Wrote request for transaction %d",
				tx.Request.GetTransactionID())
		}
	}
}

// processError handles errors for a specific transaction
func (t *TCPTransport) processError(txID common.TransactionID, err error) {
	ctx := context.Background()
	// Try to find the transaction and complete it with error
	if tx, ok := t.transactionPool.Release(txID); ok {
		t.logger.Debug(ctx, "Processing error for transaction %d: %v", txID, err)
		tx.Complete(nil, err)
	} else {
		t.logger.Warn(ctx, "Error for unknown transaction %d: %v", txID, err)
	}
}

// setDisconnected marks the transport as disconnected
func (t *TCPTransport) setDisconnected(err error) {
	ctx := context.Background()
	t.mutex.Lock()
	wasConnected := t.connected
	t.connected = false
	t.mutex.Unlock()

	if wasConnected {
		t.logger.Error(ctx, "Transport disconnected: %v", err)

		// Reset the transaction pool to clean state for next reconnection
		t.transactionPool.transactionsMu.Lock()
		t.transactionPool.unsafeReset() // This will cancel all transactions
		t.transactionPool.transactionsMu.Unlock()
	}
}

// Send sends a request and returns the response
// This implements the client-side request/response pattern for Modbus TCP
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4 (MODBUS Data Model)
func (t *TCPTransport) Send(ctx context.Context, request common.Request) (common.Response, error) {
	if !t.IsConnected() {
		return nil, common.ErrNotConnected
	}

	// Log the function code being sent
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6 (MODBUS Function Codes)
	t.logger.Debug(ctx, "Sending request: function=%d", request.GetPDU().FunctionCode)

	// Create a transaction and add it to the pool
	// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
	// The transaction ID will be assigned by the pool and used to match the response
	tx, err := t.transactionPool.Place(ctx, request)
	if err != nil {
		t.logger.Error(ctx, "Failed to create transaction: %v", err)
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	t.logger.Debug(ctx, "Created transaction %d", request.GetTransactionID())

	// Send the transaction to the write loop
	select {
	case t.writeChan <- tx:
		t.logger.Debug(ctx, "Queued transaction %d for writing", request.GetTransactionID())
	case <-ctx.Done():
		// Context cancelled before we could queue
		t.logger.Debug(ctx, "Context cancelled before queueing transaction %d",
			request.GetTransactionID())
		t.transactionPool.Release(request.GetTransactionID())
		return nil, ctx.Err()
	case <-t.done:
		// Transport is shutting down
		t.logger.Debug(ctx, "Transport shutting down, cancelling transaction %d",
			request.GetTransactionID())
		t.transactionPool.Release(request.GetTransactionID())
		return nil, common.ErrTransportClosing
	}

	// Wait for the response
	select {
	case response := <-tx.ResponseCh:
		t.logger.Debug(ctx, "Received response for transaction %d", request.GetTransactionID())
		return response, nil
	case err := <-tx.ErrCh:
		t.logger.Debug(ctx, "Received error for transaction %d: %v",
			request.GetTransactionID(), err)
		return nil, err
	case <-ctx.Done():
		// Context cancelled while waiting for response
		t.logger.Debug(ctx, "Context cancelled while waiting for transaction %d",
			request.GetTransactionID())
		// Transaction will be cleaned up by timeout monitor
		return nil, ctx.Err()
	}
}
