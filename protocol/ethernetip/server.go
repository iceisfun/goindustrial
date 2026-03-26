package ethernetip

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)

// Server implements an EtherNet/IP Server (Adapter).
type Server struct {
	router       *cip.MessageRouter
	logger       logging.Logger
	ln           net.Listener
	done         chan struct{}
	mu           sync.Mutex
	injectedConn net.Conn // for single pre-established connections (net.Pipe)
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

// NewServer creates a new Server backed by the given message router.
func NewServer(router *cip.MessageRouter, opts ...ServerOption) *Server {
	s := &Server{
		router: router,
		logger: logging.NewNopLogger(),
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

// Stop gracefully stops the server.
func (s *Server) Stop() error {
	select {
	case <-s.done:
		return nil // already stopped
	default:
	}
	close(s.done)
	if s.ln != nil {
		return s.ln.Close()
	}
	if s.injectedConn != nil {
		return s.injectedConn.Close()
	}
	return nil
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

	var sessionHandle uint32

	headerBuf := make([]byte, eip.HeaderSize)

	for {
		// Check if we're done
		select {
		case <-s.done:
			return
		default:
		}

		// Read Header
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}

		// Parse Header
		command := eip.Command(binary.LittleEndian.Uint16(headerBuf[0:2]))
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		session := binary.LittleEndian.Uint32(headerBuf[4:8])
		senderContext := headerBuf[12:20]

		// Check Max Packet Size
		const MaxPacketSize = 4096
		if dataLen > MaxPacketSize {
			return
		}

		// Read Data
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
			sessionHandle = 0x01020304
			session = sessionHandle
			respData = make([]byte, 4)
			binary.LittleEndian.PutUint16(respData[0:], 1) // Protocol Version 1
			binary.LittleEndian.PutUint16(respData[2:], 0) // Options 0

		case eip.CommandUnregisterSession:
			return // Close connection

		case eip.CommandSendRRData:
			respData, err = s.handleSendRRData(data)
			if err != nil {
				status = 0x0001
			}

		case eip.CommandSendUnitData:
			respData, err = s.handleSendUnitData(data)
			if err != nil {
				status = 0x0001
			}

		default:
			status = 0x0001
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

func (s *Server) handleSendRRData(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("short data")
	}

	cpfData := data[6:]
	cpf, err := eip.DecodeCommonPacketFormat(cpfData)
	if err != nil {
		return nil, err
	}

	item := cpf.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		return nil, fmt.Errorf("no unconnected message item")
	}

	// Decode Message Router Request
	mrReq := &cip.MessageRouterRequest{}
	buf := bytes.NewReader(item.Data)
	if err := binary.Read(buf, binary.LittleEndian, &mrReq.Service); err != nil {
		return nil, err
	}
	var pathSizeWords uint8
	if err := binary.Read(buf, binary.LittleEndian, &pathSizeWords); err != nil {
		return nil, err
	}
	pathBytes := make([]byte, int(pathSizeWords)*2)
	if _, err := buf.Read(pathBytes); err != nil {
		return nil, err
	}
	mrReq.RequestPath = cip.Path(pathBytes)

	remaining := buf.Len()
	if remaining > 0 {
		mrReq.RequestData = make([]byte, remaining)
		if _, err := buf.Read(mrReq.RequestData); err != nil {
			return nil, err
		}
	}

	// Dispatch
	mrResp, err := s.router.Dispatch(mrReq)
	if err != nil {
		return nil, err
	}

	// Encode Response
	respBuf := new(bytes.Buffer)
	binary.Write(respBuf, binary.LittleEndian, mrResp.Service)
	binary.Write(respBuf, binary.LittleEndian, mrResp.Reserved)
	binary.Write(respBuf, binary.LittleEndian, mrResp.GeneralStatus)
	binary.Write(respBuf, binary.LittleEndian, mrResp.ExtStatusSize)
	for _, ext := range mrResp.ExtStatus {
		binary.Write(respBuf, binary.LittleEndian, ext)
	}
	respBuf.Write(mrResp.ResponseData)

	// Construct Response CPF
	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, respBuf.Bytes()),
	)

	respCPFData, err := respCPF.Encode()
	if err != nil {
		return nil, err
	}

	// Prepend Interface Handle (0) and Timeout (0)
	finalResp := make([]byte, 6+len(respCPFData))
	copy(finalResp[6:], respCPFData)

	return finalResp, nil
}

func (s *Server) handleSendUnitData(data []byte) ([]byte, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("short data")
	}

	cpfData := data[6:]
	cpf, err := eip.DecodeCommonPacketFormat(cpfData)
	if err != nil {
		return nil, err
	}

	addrItem := cpf.FindItemByType(eip.ItemIDConnectedAddress)
	if addrItem == nil {
		return nil, fmt.Errorf("no connected address item")
	}

	if len(addrItem.Data) < 4 {
		return nil, fmt.Errorf("short address item data")
	}

	dataItem := cpf.FindItemByType(eip.ItemIDConnectedData)
	if dataItem == nil {
		return nil, fmt.Errorf("no connected data item")
	}

	if len(dataItem.Data) < 2 {
		return nil, fmt.Errorf("short data item data")
	}
	pdu := dataItem.Data[2:]

	// Decode Message Router Request from PDU (Class 3 explicit message)
	mrReq := &cip.MessageRouterRequest{}
	buf := bytes.NewReader(pdu)
	if err := binary.Read(buf, binary.LittleEndian, &mrReq.Service); err != nil {
		return nil, err
	}
	var pathSizeWords uint8
	if err := binary.Read(buf, binary.LittleEndian, &pathSizeWords); err != nil {
		return nil, err
	}
	pathBytes := make([]byte, int(pathSizeWords)*2)
	if _, err := buf.Read(pathBytes); err != nil {
		return nil, err
	}
	mrReq.RequestPath = cip.Path(pathBytes)

	remaining := buf.Len()
	if remaining > 0 {
		mrReq.RequestData = make([]byte, remaining)
		if _, err := buf.Read(mrReq.RequestData); err != nil {
			return nil, err
		}
	}

	// Dispatch
	mrResp, err := s.router.Dispatch(mrReq)
	if err != nil {
		return nil, err
	}

	// Encode Response
	respBuf := new(bytes.Buffer)
	binary.Write(respBuf, binary.LittleEndian, mrResp.Service)
	binary.Write(respBuf, binary.LittleEndian, mrResp.Reserved)
	binary.Write(respBuf, binary.LittleEndian, mrResp.GeneralStatus)
	binary.Write(respBuf, binary.LittleEndian, mrResp.ExtStatusSize)
	for _, ext := range mrResp.ExtStatus {
		binary.Write(respBuf, binary.LittleEndian, ext)
	}
	respBuf.Write(mrResp.ResponseData)

	// Connected response
	respAddrData := addrItem.Data

	respDataBuf := new(bytes.Buffer)
	seqCount := binary.LittleEndian.Uint16(dataItem.Data[0:2])
	binary.Write(respDataBuf, binary.LittleEndian, seqCount)
	respDataBuf.Write(respBuf.Bytes())

	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDConnectedAddress, respAddrData),
		eip.NewCPFItem(eip.ItemIDConnectedData, respDataBuf.Bytes()),
	)

	respCPFData, err := respCPF.Encode()
	if err != nil {
		return nil, err
	}

	finalResp := make([]byte, 6+len(respCPFData))
	copy(finalResp[6:], respCPFData)

	return finalResp, nil
}
