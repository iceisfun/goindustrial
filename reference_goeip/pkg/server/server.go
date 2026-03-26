package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/eip"
)

// Server implements an EtherNet/IP Server (Adapter)
type Server struct {
	router *cip.MessageRouter
	mu     sync.Mutex
	ln     net.Listener
	done   chan struct{}
}

// NewServer creates a new Server
func NewServer(router *cip.MessageRouter) *Server {
	return &Server{
		router: router,
	}
}

// Start starts the TCP listener
func (s *Server) Start(address string) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	s.ln = ln
	s.done = make(chan struct{})
	go s.acceptLoop(ln)
	return nil
}

// Stop gracefully stops the server by closing the listener and stopping the accept loop
func (s *Server) Stop() error {
	close(s.done)
	return s.ln.Close()
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			// Log error
			continue
		}
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Session Handle
	var sessionHandle uint32 = 0

	headerBuf := make([]byte, 24) // EIP Header is 24 bytes

	for {
		// Read Header
		if _, err := io.ReadFull(conn, headerBuf); err != nil {
			return
		}

		// Parse Header
		// Command (2), Length (2), Session (4), Status (4), Context (8), Options (4)
		command := eip.Command(binary.LittleEndian.Uint16(headerBuf[0:2]))
		dataLen := binary.LittleEndian.Uint16(headerBuf[2:4])
		session := binary.LittleEndian.Uint32(headerBuf[4:8])
		// status := binary.LittleEndian.Uint32(headerBuf[8:12])
		senderContext := headerBuf[12:20]
		// options := binary.LittleEndian.Uint32(headerBuf[20:24])

		// Check Max Packet Size
		const MaxPacketSize = 4096
		if dataLen > MaxPacketSize {
			// Too large, close connection
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
		var status uint32 = 0

		switch command {
		case eip.CommandRegisterSession:
			// Generate Session Handle
			sessionHandle = 0x01020304 // Static for now, or random
			session = sessionHandle
			// Response data: Protocol Version (2), Options (2)
			respData = make([]byte, 4)
			binary.LittleEndian.PutUint16(respData[0:], 1) // Protocol Version 1
			binary.LittleEndian.PutUint16(respData[2:], 0) // Options 0

		case eip.CommandUnregisterSession:
			return // Close connection

		case eip.CommandSendRRData:
			respData, err = s.handleSendRRData(data)
			if err != nil {
				status = 0x0001 // Fail
			}

		case eip.CommandSendUnitData:
			respData, err = s.handleSendUnitData(data)
			if err != nil {
				status = 0x0001 // Fail
			}

		default:
			// Not supported
			status = 0x0001 // Fail
		}

		// Send Response
		respHeader := make([]byte, 24)
		binary.LittleEndian.PutUint16(respHeader[0:], uint16(command))
		binary.LittleEndian.PutUint16(respHeader[2:], uint16(len(respData)))
		binary.LittleEndian.PutUint32(respHeader[4:], session)
		binary.LittleEndian.PutUint32(respHeader[8:], status)
		copy(respHeader[12:], senderContext)
		binary.LittleEndian.PutUint32(respHeader[20:], 0) // Options

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
	// Parse Interface Handle (4) + Timeout (2) + CPF
	if len(data) < 6 {
		return nil, fmt.Errorf("short data")
	}
	// interfaceHandle := binary.LittleEndian.Uint32(data[0:4])
	// timeout := binary.LittleEndian.Uint16(data[4:6])

	cpfData := data[6:]
	cpf, err := eip.DecodeCommonPacketFormat(cpfData)
	if err != nil {
		return nil, err
	}

	// Find Unconnected Message Item
	item := cpf.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		return nil, fmt.Errorf("no unconnected message item")
	}

	// Decode Message Router Request
	mrReq := &cip.MessageRouterRequest{}
	// We need to decode manually as we don't have DecodeMessageRouterRequest exposed?
	// Wait, we have DecodeMessageRouterResponse but not Request in `pkg/cip/message.go`?
	// Let's check `pkg/cip/message.go`.
	// It has `Encode` for Request, and `DecodeMessageRouterResponse`.
	// I need to implement `DecodeMessageRouterRequest` or do it manually.

	// Manual decode for now:
	// Service (1), PathSize (1), Path (Words*2), Data...
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
	// Service (1), Reserved (1), Status (1), ExtStatusSize (1), ExtStatus (N*2), Data...
	// Wait, MessageRouterResponse struct has these fields.
	// But we need to encode it to bytes to put in CPF.
	// `pkg/cip/message.go` doesn't have `Encode` for Response?
	// I should check.

	// Assuming I need to encode it manually.
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
	// Parse Interface Handle (4) + Timeout (2) + CPF
	if len(data) < 6 {
		return nil, fmt.Errorf("short data")
	}

	cpfData := data[6:]
	cpf, err := eip.DecodeCommonPacketFormat(cpfData)
	if err != nil {
		return nil, err
	}

	// Find Connected Address Item
	addrItem := cpf.FindItemByType(eip.ItemIDConnectedAddress) // 0xA1
	if addrItem == nil {
		return nil, fmt.Errorf("no connected address item")
	}

	// Connection ID is in the Address Item Data (4 bytes)
	if len(addrItem.Data) < 4 {
		return nil, fmt.Errorf("short address item data")
	}
	// connID := binary.LittleEndian.Uint32(addrItem.Data)

	// Find Connected Data Item
	dataItem := cpf.FindItemByType(eip.ItemIDConnectedData) // 0xB1
	if dataItem == nil {
		return nil, fmt.Errorf("no connected data item")
	}

	// For now, we don't have a Runtime linked to Server to handle connected messages.
	// We should return an error or implement dispatch if we had Runtime.
	// Since we can't easily add Runtime dependency without changing signature,
	// let's return an error indicating not supported for now, but at least we parsed it.
	// Or we can try to treat it as explicit message if it contains one?
	// Connected messages usually contain Transport Class 3 (Explicit) or Class 0/1 (Implicit).
	// If it's Explicit, it has a Sequence Count and then the PDU.

	// Let's assume it's Explicit Message over Connection (Class 3).
	// Sequence Count (2)
	if len(dataItem.Data) < 2 {
		return nil, fmt.Errorf("short data item data")
	}
	// seqCount := binary.LittleEndian.Uint16(dataItem.Data[0:2])
	pdu := dataItem.Data[2:]

	// The PDU is a Message Router Request?
	// Yes, for Class 3.
	// So we can dispatch to Router!

	// Decode Message Router Request
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

	// Construct Response CPF
	// Address Item is usually 0xA1 with same Connection ID? Or 0 (Null)?
	// For Connected Response, we usually use Connected Address Item.
	// But for simplicity, let's see if Null works (probably not for Connected).
	// We should use Connected Address Item with same ID.
	respAddrData := addrItem.Data // Copy Connection ID

	// Data Item: Sequence Count + Response PDU
	respDataBuf := new(bytes.Buffer)
	// Sequence Count (Echo) - we need to read it from request
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

	// Prepend Interface Handle (0) and Timeout (0)
	finalResp := make([]byte, 6+len(respCPFData))
	copy(finalResp[6:], respCPFData)

	return finalResp, nil
}
