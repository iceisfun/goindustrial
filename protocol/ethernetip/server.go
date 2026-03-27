package ethernetip

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)

// ConnectedClient describes an active EIP connection.
type ConnectedClient struct {
	RemoteAddr    string
	SessionHandle uint32
	ConnectedAt   time.Time
}

// Server implements an EtherNet/IP Server (Adapter).
type Server struct {
	router       *cip.MessageRouter
	logger       logging.Logger
	ln           net.Listener
	done         chan struct{}
	mu           sync.Mutex
	injectedConn net.Conn // for single pre-established connections (net.Pipe)

	// Session management — atomic counter ensures unique handles across connections.
	nextSession atomic.Uint32

	// Client tracking.
	clientsMu          sync.RWMutex
	clients            map[net.Conn]*connState
	onClientConnect    func(ConnectedClient)
	onClientDisconnect func(ConnectedClient)

	// Identity for ListIdentity responses.
	identity eip.ListIdentityItem
}

type connState struct {
	remoteAddr    string
	sessionHandle uint32
	connectedAt   time.Time
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithServerListener injects a pre-created net.Listener.
func WithServerListener(l net.Listener) ServerOption {
	return func(s *Server) {
		s.ln = l
	}
}

// WithServerLogger sets the logger for the server.
func WithServerLogger(l logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = l
	}
}

// WithServerConn injects a single pre-established connection (e.g. net.Pipe).
// When set, Start skips the accept loop and handles this single connection directly.
func WithServerConn(conn net.Conn) ServerOption {
	return func(s *Server) {
		s.injectedConn = conn
	}
}

// WithIdentity configures the device identity returned by ListIdentity.
func WithIdentity(id eip.ListIdentityItem) ServerOption {
	return func(s *Server) {
		s.identity = id
	}
}

// WithOnClientConnect sets a callback fired when a client connection is accepted.
func WithOnClientConnect(fn func(ConnectedClient)) ServerOption {
	return func(s *Server) {
		s.onClientConnect = fn
	}
}

// WithOnClientDisconnect sets a callback fired when a client disconnects.
func WithOnClientDisconnect(fn func(ConnectedClient)) ServerOption {
	return func(s *Server) {
		s.onClientDisconnect = fn
	}
}

// NewServer creates a new Server backed by the given message router.
func NewServer(router *cip.MessageRouter, opts ...ServerOption) *Server {
	s := &Server{
		router:  router,
		logger:  logging.NewNopLogger(),
		clients: make(map[net.Conn]*connState),
		identity: eip.ListIdentityItem{
			TypeID:        eip.ItemIDListIdentity,
			EncapsVersion: 1,
			ProductName:   "GoIndustrial EIP Server",
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start starts the server. If WithServerConn was used, it handles that single
// connection in a goroutine and returns. If WithServerListener was used, it
// uses that listener. Otherwise it creates a TCP listener on the given address.
func (s *Server) Start(ctx context.Context, address string) error {
	s.done = make(chan struct{})

	// Single injected connection mode (net.Pipe)
	if s.injectedConn != nil {
		go s.HandleConn(s.injectedConn)
		return nil
	}

	// Listener mode
	if s.ln == nil {
		ln, err := net.Listen("tcp", address)
		if err != nil {
			return err
		}
		s.ln = ln
	}

	go s.acceptLoop()
	return nil
}

// Stop gracefully stops the server. It closes the listener and all tracked
// client connections so that HandleConn goroutines exit cleanly.
func (s *Server) Stop() error {
	select {
	case <-s.done:
		return nil // already stopped
	default:
	}
	close(s.done)

	// Close listener first to stop accepting new connections.
	var firstErr error
	if s.ln != nil {
		firstErr = s.ln.Close()
	}

	// Close all tracked client connections so HandleConn goroutines unblock.
	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clientsMu.Unlock()

	// Also close the injected conn in case it was not yet tracked.
	if s.injectedConn != nil {
		s.injectedConn.Close()
	}

	return firstErr
}

// ConnectedClients returns a snapshot of all active client connections.
func (s *Server) ConnectedClients() []ConnectedClient {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	result := make([]ConnectedClient, 0, len(s.clients))
	for _, cs := range s.clients {
		result = append(result, ConnectedClient{
			RemoteAddr:    cs.remoteAddr,
			SessionHandle: cs.sessionHandle,
			ConnectedAt:   cs.connectedAt,
		})
	}
	return result
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			s.logger.Error(context.Background(), "accept error: %v", err)
			continue
		}
		go s.HandleConn(conn)
	}
}

// HandleConn handles a single EIP connection. It is exported so tests can call
// it directly with one end of a net.Pipe.
func (s *Server) HandleConn(conn net.Conn) {
	defer conn.Close()

	cs := &connState{
		remoteAddr:  conn.RemoteAddr().String(),
		connectedAt: time.Now(),
	}

	s.clientsMu.Lock()
	s.clients[conn] = cs
	s.clientsMu.Unlock()

	if s.onClientConnect != nil {
		s.onClientConnect(ConnectedClient{
			RemoteAddr:  cs.remoteAddr,
			ConnectedAt: cs.connectedAt,
		})
	}

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()

		if s.onClientDisconnect != nil {
			s.onClientDisconnect(ConnectedClient{
				RemoteAddr:    cs.remoteAddr,
				SessionHandle: cs.sessionHandle,
				ConnectedAt:   cs.connectedAt,
			})
		}
	}()

	var sessionHandle uint32

	headerBuf := make([]byte, eip.HeaderSize)

	for {
		select {
		case <-s.done:
			return
		default:
		}

		// Read Header
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}

		command := eip.Command(binary.LittleEndian.Uint16(headerBuf[0:2]))
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		session := binary.LittleEndian.Uint32(headerBuf[4:8])
		senderContext := headerBuf[12:20]

		const maxPacketSize = 4096
		if dataLen > maxPacketSize {
			return
		}

		data := make([]byte, dataLen)
		if dataLen > 0 {
			if _, err := io.ReadFull(conn, data); err != nil {
				return
			}
		}

		var respData []byte
		var err error
		var status uint32

		switch command {
		case eip.CommandRegisterSession:
			sessionHandle = s.nextSession.Add(1)
			session = sessionHandle
			cs.sessionHandle = sessionHandle
			respData = make([]byte, 4)
			binary.LittleEndian.PutUint16(respData[0:], 1) // Protocol Version 1
			binary.LittleEndian.PutUint16(respData[2:], 0) // Options 0

		case eip.CommandUnregisterSession:
			return // Close connection

		case eip.CommandListIdentity:
			respData, err = s.handleListIdentity()
			if err != nil {
				status = eip.StatusInvalidCommand
			}

		case eip.CommandListServices:
			respData, err = s.handleListServices()
			if err != nil {
				status = eip.StatusInvalidCommand
			}

		case eip.CommandSendRRData:
			if session != sessionHandle || sessionHandle == 0 {
				status = eip.StatusInvalidSessionHandle
			} else {
				respData, err = s.handleSendRRData(data)
				if err != nil {
					status = eip.StatusInvalidCommand
				}
			}

		case eip.CommandSendUnitData:
			if session != sessionHandle || sessionHandle == 0 {
				status = eip.StatusInvalidSessionHandle
			} else {
				respData, err = s.handleSendUnitData(data)
				if err != nil {
					status = eip.StatusInvalidCommand
				}
			}

		default:
			status = eip.StatusInvalidCommand
		}

		// Send Response
		respHeader := make([]byte, eip.HeaderSize)
		binary.LittleEndian.PutUint16(respHeader[0:], uint16(command))
		binary.LittleEndian.PutUint16(respHeader[2:], uint16(len(respData)))
		binary.LittleEndian.PutUint32(respHeader[4:], session)
		binary.LittleEndian.PutUint32(respHeader[8:], status)
		copy(respHeader[12:], senderContext)
		binary.LittleEndian.PutUint32(respHeader[20:], 0)

		if _, err := conn.Write(respHeader); err != nil {
			return
		}
		if len(respData) > 0 {
			if _, err := conn.Write(respData); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleListIdentity() ([]byte, error) {
	return eip.EncodeListIdentityResponse([]eip.ListIdentityItem{s.identity})
}

func (s *Server) handleListServices() ([]byte, error) {
	return eip.EncodeListServicesResponse([]eip.ListServicesItem{{
		TypeID:          eip.ItemIDListServices,
		Version:         1,
		CapabilityFlags: 0x0020, // Supports CIP encapsulation via TCP
		Name:            "Communications",
	}})
}

func (s *Server) handleSendRRData(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, errShortData
	}

	cpf, err := eip.DecodeCommonPacketFormat(data[6:])
	if err != nil {
		return nil, err
	}

	item := cpf.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		return nil, errNoCPFItem
	}

	mrReq, err := cip.DecodeMessageRouterRequest(item.Data)
	if err != nil {
		return nil, err
	}

	mrResp, err := s.router.Dispatch(mrReq)
	if err != nil {
		return nil, err
	}

	respBytes, err := mrResp.Encode()
	if err != nil {
		return nil, err
	}

	return wrapUnconnectedResponse(respBytes)
}

func (s *Server) handleSendUnitData(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, errShortData
	}

	cpf, err := eip.DecodeCommonPacketFormat(data[6:])
	if err != nil {
		return nil, err
	}

	addrItem := cpf.FindItemByType(eip.ItemIDConnectedAddress)
	if addrItem == nil || len(addrItem.Data) < 4 {
		return nil, errNoCPFItem
	}

	dataItem := cpf.FindItemByType(eip.ItemIDConnectedData)
	if dataItem == nil || len(dataItem.Data) < 2 {
		return nil, errNoCPFItem
	}

	seqCount := binary.LittleEndian.Uint16(dataItem.Data[0:2])
	pdu := dataItem.Data[2:]

	mrReq, err := cip.DecodeMessageRouterRequest(pdu)
	if err != nil {
		return nil, err
	}

	mrResp, err := s.router.Dispatch(mrReq)
	if err != nil {
		return nil, err
	}

	respBytes, err := mrResp.Encode()
	if err != nil {
		return nil, err
	}

	return wrapConnectedResponse(addrItem.Data, seqCount, respBytes)
}

// wrapUnconnectedResponse wraps MR response bytes in a SendRRData CPF envelope.
func wrapUnconnectedResponse(mrRespBytes []byte) ([]byte, error) {
	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrRespBytes),
	)
	cpfData, err := respCPF.Encode()
	if err != nil {
		return nil, err
	}
	resp := make([]byte, 6+len(cpfData))
	copy(resp[6:], cpfData)
	return resp, nil
}

// wrapConnectedResponse wraps MR response bytes in a SendUnitData CPF envelope.
func wrapConnectedResponse(addrData []byte, seqCount uint16, mrRespBytes []byte) ([]byte, error) {
	dataBuf := make([]byte, 2+len(mrRespBytes))
	binary.LittleEndian.PutUint16(dataBuf[0:2], seqCount)
	copy(dataBuf[2:], mrRespBytes)

	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDConnectedAddress, addrData),
		eip.NewCPFItem(eip.ItemIDConnectedData, dataBuf),
	)
	cpfData, err := respCPF.Encode()
	if err != nil {
		return nil, err
	}
	resp := make([]byte, 6+len(cpfData))
	copy(resp[6:], cpfData)
	return resp, nil
}

var (
	errShortData = cip.Error{Status: cip.StatusPathSegmentError}
	errNoCPFItem = cip.Error{Status: cip.StatusPathSegmentError}
)
