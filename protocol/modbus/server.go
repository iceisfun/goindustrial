package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// Server implements a Modbus TCP server.
// It supports standard TCP listeners as well as any net.Listener (including
// those backed by net.Pipe for in-process testing).
type Server struct {
	// Server binding configuration
	address  string
	port     int
	listener net.Listener

	// Function code handlers map
	handlers     map[FunctionCode]HandlerFunc
	defaultStore DataStore

	// Server state
	running      bool
	clients      map[string]*clientConn
	clientsMutex sync.RWMutex
	mutex        sync.RWMutex
	logger       logging.Logger
	stopChan     chan struct{}

	// Client lifecycle callbacks
	onClientConnect    func(ConnectedClient)
	onClientDisconnect func(ConnectedClient)

	// Protocol handler for processing requests
	protocol *serverProtocolHandler

	// Maximum number of simultaneous client connections (0 = unlimited).
	maxClients int

	// injectedConn is a single pre-established connection supplied via
	// WithServerConn. When set, the server skips the accept loop and
	// handles this connection directly.
	injectedConn net.Conn
}

// ServerOption is a function type for configuring a Server.
type ServerOption func(*Server)

// WithServerPort sets the TCP port for the server.
func WithServerPort(port int) ServerOption {
	return func(s *Server) {
		s.port = port
	}
}

// WithServerLogger sets the logger for the server.
func WithServerLogger(logger logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithServerDataStore sets the data store for the server.
func WithServerDataStore(store DataStore) ServerOption {
	return func(s *Server) {
		s.defaultStore = store
	}
}

// WithServerListener sets a pre-configured listener for the server.
// This is the primary mechanism for using net.Pipe-backed listeners in tests.
func WithServerListener(listener net.Listener) ServerOption {
	return func(s *Server) {
		s.listener = listener
		if addr, ok := listener.Addr().(*net.TCPAddr); ok {
			s.port = addr.Port
			s.address = addr.IP.String()
		}
	}
}

// WithServerConn accepts a single pre-established connection (e.g., one end of
// a net.Pipe). The server handles this connection directly without an accept
// loop. Useful for deterministic in-process testing.
func WithServerConn(conn net.Conn) ServerOption {
	return func(s *Server) {
		s.injectedConn = conn
	}
}

// WithMaxClients sets the maximum number of simultaneous client connections.
// When the limit is reached, new connections are closed immediately.
// A value of 0 (the default) means unlimited.
func WithMaxClients(max int) ServerOption {
	return func(s *Server) {
		s.maxClients = max
	}
}

// WithOnClientConnect sets a callback that fires when a new client connects.
func WithOnClientConnect(fn func(ConnectedClient)) ServerOption {
	return func(s *Server) {
		s.onClientConnect = fn
	}
}

// WithOnClientDisconnect sets a callback that fires when a client disconnects.
func WithOnClientDisconnect(fn func(ConnectedClient)) ServerOption {
	return func(s *Server) {
		s.onClientDisconnect = fn
	}
}

// NewServer creates a new Modbus TCP server.
func NewServer(address string, options ...ServerOption) *Server {
	server := &Server{
		address:      address,
		port:         DefaultTCPPort,
		handlers:     make(map[FunctionCode]HandlerFunc),
		defaultStore: NewMemoryStore(),
		logger:       logging.NewNopLogger(),
		clients:      make(map[string]*clientConn),
		protocol:     newServerProtocolHandler(),
	}

	for _, option := range options {
		option(server)
	}

	// Setup default handlers based on data store
	server.setupDefaultHandlers()

	return server
}

// GetDataStore returns the server's data store.
func (s *Server) GetDataStore() DataStore {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.defaultStore
}

// setupDefaultHandlers configures handlers for standard Modbus functions.
func (s *Server) setupDefaultHandlers() {
	s.handlers = make(map[FunctionCode]HandlerFunc)

	s.SetHandler(FuncReadCoils, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadCoils(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncReadDiscreteInputs, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadDiscreteInputs(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncReadHoldingRegisters, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadHoldingRegisters(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncReadInputRegisters, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadInputRegisters(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncWriteSingleCoil, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleWriteSingleCoil(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncWriteSingleRegister, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleWriteSingleRegister(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncWriteMultipleCoils, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleWriteMultipleCoils(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncWriteMultipleRegisters, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleWriteMultipleRegisters(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncReadWriteMultipleRegisters, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadWriteMultipleRegisters(ctx, req, s.defaultStore)
	})

	s.SetHandler(FuncReadDeviceIdentification, func(ctx context.Context, req *Request) (*Response, error) {
		return s.protocol.HandleReadDeviceIdentification(ctx, req, s.defaultStore)
	})
}

// SetHandler sets the handler for a specific Modbus function code.
func (s *Server) SetHandler(functionCode FunctionCode, handler HandlerFunc) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.handlers[functionCode] = handler
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	s.mutex.Lock()
	if s.running {
		s.mutex.Unlock()
		return fmt.Errorf("server already running")
	}

	// If a single injected conn was provided, handle it directly.
	if s.injectedConn != nil {
		s.running = true
		s.stopChan = make(chan struct{})
		s.mutex.Unlock()

		client := &clientConn{
			remoteAddr:  s.injectedConn.RemoteAddr().String(),
			connectedAt: time.Now(),
			conn:        s.injectedConn,
		}

		s.clientsMutex.Lock()
		s.clients[client.remoteAddr] = client
		s.clientsMutex.Unlock()

		if s.onClientConnect != nil {
			s.onClientConnect(ConnectedClient{
				RemoteAddr:        client.remoteAddr,
				ConnectedAt:       client.connectedAt,
				FunctionCodeStats: make(map[FunctionCode]uint64),
			})
		}

		go s.handleConnection(client)
		return nil
	}

	// If no listener was provided via WithServerListener, create one.
	if s.listener == nil {
		addr := fmt.Sprintf("%s:%d", s.address, s.port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			s.mutex.Unlock()
			return err
		}
		s.listener = listener
	}

	// Update address/port from listener in case it was dynamic (port 0).
	if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
		s.port = addr.Port
		s.address = addr.IP.String()
	}

	s.running = true
	s.stopChan = make(chan struct{})
	s.mutex.Unlock()

	s.logger.Info(ctx, "Modbus TCP server started on %s:%d", s.address, s.port)

	go s.acceptLoop(ctx)

	return nil
}

// Stop stops the server.
func (s *Server) Stop(ctx context.Context) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopChan)

	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}

	// Close all client connections
	s.clientsMutex.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clients = make(map[string]*clientConn)
	s.clientsMutex.Unlock()

	// Clear injected conn
	s.injectedConn = nil

	s.running = false
	s.logger.Info(ctx, "Modbus TCP server stopped")
	return nil
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

// ConnectedClients returns a snapshot of all currently connected clients.
func (s *Server) ConnectedClients() []ConnectedClient {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	clients := make([]ConnectedClient, 0, len(s.clients))
	for _, c := range s.clients {
		clients = append(clients, ConnectedClient{
			RemoteAddr:        c.remoteAddr,
			ConnectedAt:       c.connectedAt,
			RxTransactions:    c.rxCount.Load(),
			TxTransactions:    c.txCount.Load(),
			FunctionCodeStats: fcSnapshot(c),
		})
	}
	return clients
}

// acceptLoop accepts incoming connections.
// It handles both TCP listeners (which support SetDeadline) and non-TCP
// listeners (e.g., pipe-based) gracefully.
func (s *Server) acceptLoop(ctx context.Context) {
	// Capture the listener locally so we never race with Stop() setting
	// s.listener = nil under s.mutex.
	s.mutex.RLock()
	ln := s.listener
	s.mutex.RUnlock()

	if ln == nil {
		return
	}

	// Check if the listener supports deadlines (TCP does, pipe-based does not).
	type deadliner interface {
		SetDeadline(t time.Time) error
	}
	dl, supportsDeadline := ln.(deadliner)

	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		if supportsDeadline {
			dl.SetDeadline(time.Now().Add(time.Second))
		}

		conn, err := ln.Accept()
		if err != nil {
			// Check for timeout (only possible when deadlines are supported).
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}

			// Check if shutting down.
			select {
			case <-s.stopChan:
				return
			default:
				s.logger.Error(ctx, "Error accepting connection: %v", err)
				continue
			}
		}

		remoteAddr := conn.RemoteAddr().String()

		// Enforce max-client limit.
		if s.maxClients > 0 {
			s.clientsMutex.RLock()
			count := len(s.clients)
			s.clientsMutex.RUnlock()
			if count >= s.maxClients {
				s.logger.Warn(ctx, "Max clients (%d) reached, rejecting %s", s.maxClients, remoteAddr)
				conn.Close()
				continue
			}
		}

		s.logger.Info(ctx, "New client connected: %s", remoteAddr)

		client := &clientConn{
			remoteAddr:  remoteAddr,
			connectedAt: time.Now(),
			conn:        conn,
		}
		s.clientsMutex.Lock()
		s.clients[remoteAddr] = client
		s.clientsMutex.Unlock()

		if s.onClientConnect != nil {
			s.onClientConnect(ConnectedClient{
				RemoteAddr:        remoteAddr,
				ConnectedAt:       client.connectedAt,
				FunctionCodeStats: make(map[FunctionCode]uint64),
			})
		}

		go s.handleConnection(client)
	}
}

// handleConnection handles a single client connection, reading MBAP headers and
// dispatching requests. Works with any net.Conn (TCP, pipe, etc.).
func (s *Server) handleConnection(client *clientConn) {
	ctx := context.Background()
	conn := client.conn
	remoteAddr := client.remoteAddr
	defer func() {
		if s.onClientDisconnect != nil {
			s.onClientDisconnect(ConnectedClient{
				RemoteAddr:        remoteAddr,
				ConnectedAt:       client.connectedAt,
				RxTransactions:    client.rxCount.Load(),
				TxTransactions:    client.txCount.Load(),
				FunctionCodeStats: fcSnapshot(client),
			})
		}

		s.clientsMutex.Lock()
		delete(s.clients, remoteAddr)
		s.clientsMutex.Unlock()

		conn.Close()
		s.logger.Info(ctx, "Client disconnected: %s", remoteAddr)
	}()

	for {
		// Only set read deadline if the conn supports it (TCP connections do,
		// net.Pipe connections do not).
		if tc, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			tc.SetReadDeadline(time.Now().Add(30 * time.Second))
		}

		// Read the MBAP header (7 bytes).
		header := make([]byte, TCPHeaderLength)
		_, err := io.ReadFull(conn, header)
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			s.logger.Error(ctx, "Error reading header from %s: %v", remoteAddr, err)
			return
		}

		// Parse MBAP header.
		transactionID := TransactionID(binary.BigEndian.Uint16(header[0:2]))
		protocolID := ProtocolID(binary.BigEndian.Uint16(header[2:4]))
		length := binary.BigEndian.Uint16(header[4:6])
		unitID := UnitID(header[6])

		// Validate protocol ID.
		if protocolID != TCPProtocolIdentifier {
			s.logger.Error(ctx, "Invalid protocol ID from %s: %d", remoteAddr, protocolID)
			continue
		}

		// Read the PDU (length - 1 bytes, already read unitID).
		dataLength := int(length) - 1
		if dataLength <= 0 {
			s.logger.Error(ctx, "Invalid data length from %s: %d", remoteAddr, length)
			continue
		}

		data := make([]byte, dataLength)
		_, err = io.ReadFull(conn, data)
		if err != nil {
			s.logger.Error(ctx, "Error reading data from %s: %v", remoteAddr, err)
			return
		}

		functionCode := FunctionCode(data[0])
		pduData := data[1:]

		request := NewRequest(unitID, functionCode, pduData)
		request.SetTransactionID(transactionID)

		// Count received transaction.
		client.rxCount.Add(1)
		client.fcCount[functionCode].Add(1)

		s.logger.Debug(ctx, "Received request from %s: txID=%d, unit=%d, function=%s",
			remoteAddr, transactionID, unitID, functionCode)

		response, err := s.dispatchRequest(ctx, request)
		if err != nil {
			if modbusErr, ok := err.(*ModbusError); ok {
				exceptionCode := modbusErr.ExceptionCode
				s.logger.Debug(ctx, "Modbus exception: %s", err.Error())

				exceptionResponse := NewResponse(
					transactionID,
					unitID,
					functionCode|FunctionCode(ExceptionBit),
					[]byte{byte(exceptionCode)},
				)
				s.sendResponse(conn, exceptionResponse)
				client.txCount.Add(1)
			} else {
				s.logger.Error(ctx, "Error processing request from %s: %v", remoteAddr, err)
				return
			}
			continue
		}

		s.sendResponse(conn, response)
		client.txCount.Add(1)
	}
}

// dispatchRequest dispatches a request to the appropriate handler.
func (s *Server) dispatchRequest(ctx context.Context, request *Request) (*Response, error) {
	functionCode := request.GetPDU().FunctionCode

	s.mutex.RLock()
	handler, exists := s.handlers[functionCode]
	s.mutex.RUnlock()

	if !exists {
		return nil, &ModbusError{
			FunctionCode:  functionCode,
			ExceptionCode: ExceptionFunctionCodeNotSupported,
		}
	}

	return handler(ctx, request)
}

// sendResponse sends a response back to the client.
func (s *Server) sendResponse(conn net.Conn, response *Response) {
	ctx := context.Background()
	data, err := response.Encode()
	if err != nil {
		s.logger.Error(ctx, "Error encoding response: %v", err)
		return
	}

	_, err = conn.Write(data)
	if err != nil {
		s.logger.Error(ctx, "Error sending response: %v", err)
		return
	}

	s.logger.Debug(ctx, "Sent response: txID=%d, function=%s",
		response.GetTransactionID(), response.GetPDU().FunctionCode)
}

// clientConn is the internal per-connection tracking state.
type clientConn struct {
	remoteAddr  string
	connectedAt time.Time
	conn        net.Conn
	rxCount     atomic.Uint64
	txCount     atomic.Uint64
	fcCount     [256]atomic.Uint64
}

// ConnectedClient is a snapshot of a connected client's state.
// Returned by Server.ConnectedClients(). Safe to copy and store.
type ConnectedClient struct {
	RemoteAddr        string
	ConnectedAt       time.Time
	RxTransactions    uint64
	TxTransactions    uint64
	FunctionCodeStats map[FunctionCode]uint64
}

// String returns a human-readable summary of the connected client.
func (c ConnectedClient) String() string {
	duration := time.Since(c.ConnectedAt).Truncate(time.Second)
	s := fmt.Sprintf("%s | connected %s | rx: %d tx: %d", c.RemoteAddr, duration, c.RxTransactions, c.TxTransactions)
	if len(c.FunctionCodeStats) > 0 {
		codes := make([]FunctionCode, 0, len(c.FunctionCodeStats))
		for fc := range c.FunctionCodeStats {
			codes = append(codes, fc)
		}
		sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

		parts := make([]string, 0, len(codes))
		for _, fc := range codes {
			parts = append(parts, fmt.Sprintf("%s=%d", fc, c.FunctionCodeStats[fc]))
		}
		s += " | fc: " + strings.Join(parts, " ")
	}
	return s
}

// fcSnapshot creates a FunctionCodeStats map from a clientConn's atomic counters.
func fcSnapshot(c *clientConn) map[FunctionCode]uint64 {
	stats := make(map[FunctionCode]uint64)
	for i := range c.fcCount {
		if v := c.fcCount[i].Load(); v > 0 {
			stats[FunctionCode(i)] = v
		}
	}
	return stats
}
