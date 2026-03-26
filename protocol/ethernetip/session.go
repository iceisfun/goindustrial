package ethernetip

import (
	"context"
	"fmt"
	"sync"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)

// Session represents an EIP session over a TCP connection.
type Session struct {
	conn   *TCPConn
	handle eip.SessionHandle
	logger logging.Logger
	mu     sync.Mutex // protects handle
}

// NewSession creates a new session on top of the given TCPConn.
// If logger is nil, a NopLogger is used.
func NewSession(conn *TCPConn, logger logging.Logger) *Session {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &Session{
		conn:   conn,
		logger: logger,
	}
}

// Register sends the RegisterSession command and stores the session handle.
func (s *Session) Register(ctx context.Context) error {
	regData := eip.NewRegisterSessionData()
	data, err := regData.Encode()
	if err != nil {
		return err
	}

	s.logger.Info(ctx, "Sending RegisterSession command")
	if err := s.conn.Send(eip.CommandRegisterSession, data, 0); err != nil {
		return err
	}

	header, _, err := s.conn.Receive()
	if err != nil {
		return err
	}

	if header.Status != eip.StatusSuccess {
		return fmt.Errorf("register session failed with status: 0x%08X", header.Status)
	}

	s.mu.Lock()
	s.handle = header.SessionHandle
	s.mu.Unlock()
	s.logger.Info(ctx, "Session registered. Handle: 0x%08X", header.SessionHandle)

	return nil
}

// Unregister sends the UnregisterSession command.
func (s *Session) Unregister(ctx context.Context) error {
	s.mu.Lock()
	handle := s.handle
	s.mu.Unlock()
	s.logger.Info(ctx, "Sending UnregisterSession command")
	return s.conn.Send(eip.CommandUnregisterSession, nil, handle)
}

// Close closes the underlying TCP connection.
func (s *Session) Close() error {
	return s.conn.Close()
}

// SendRRData sends a Request/Response Data packet (unconnected message) and
// returns the unconnected message data from the response.
func (s *Session) SendRRData(ctx context.Context, request []byte) ([]byte, error) {
	// Construct CPF:
	//   Item 0: Null Address (0x0000) - Length 0
	//   Item 1: Unconnected Data (0x00B2) - Length len(request)
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, request),
	)

	cpfData, err := cpf.Encode()
	if err != nil {
		return nil, err
	}

	// Prepend Interface Handle (4 bytes, 0) and Timeout (2 bytes, 0)
	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	s.mu.Lock()
	handle := s.handle
	s.mu.Unlock()
	s.logger.Debug(ctx, "Sending RRData (len=%d)", len(rrData))
	if err := s.conn.Send(eip.CommandSendRRData, rrData, handle); err != nil {
		return nil, err
	}

	// Receive Response
	header, respData, err := s.conn.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("RRData command failed with status: 0x%08X", header.Status)
	}

	// Response contains Interface Handle (4 bytes) and Timeout (2 bytes)
	if len(respData) < 6 {
		return nil, fmt.Errorf("response data too short")
	}
	respCPFData := respData[6:]

	// Parse CPF from response
	respCPF, err := eip.DecodeCommonPacketFormat(respCPFData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CPF: %w", err)
	}

	// Find Unconnected Data Item
	item := respCPF.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		return nil, fmt.Errorf("response CPF missing Unconnected Message item")
	}

	return item.Data, nil
}

// SendCIPRequest sends a CIP message router request via SendRRData and decodes
// the CIP response.
func (s *Session) SendCIPRequest(ctx context.Context, req *cip.MessageRouterRequest) (*cip.MessageRouterResponse, error) {
	reqBytes, err := req.Encode()
	if err != nil {
		return nil, err
	}

	s.logger.Trace(ctx, "Sending CIP Request (%d bytes)", len(reqBytes))

	respBytes, err := s.SendRRData(ctx, reqBytes)
	if err != nil {
		return nil, err
	}

	s.logger.Trace(ctx, "Received CIP Response (%d bytes)", len(respBytes))

	return cip.DecodeMessageRouterResponse(respBytes)
}

// ListIdentity sends the ListIdentity command and returns the identity items.
func (s *Session) ListIdentity(ctx context.Context) ([]eip.ListIdentityItem, error) {
	s.logger.Info(ctx, "Sending ListIdentity command")
	if err := s.conn.Send(eip.CommandListIdentity, nil, 0); err != nil {
		return nil, err
	}

	header, respData, err := s.conn.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("ListIdentity command failed with status: 0x%08X", header.Status)
	}

	s.logger.Trace(ctx, "ListIdentity response (%d bytes)", len(respData))
	return eip.DecodeListIdentityResponse(respData)
}

// ListServices sends the ListServices command and returns the service items.
func (s *Session) ListServices(ctx context.Context) ([]eip.ListServicesItem, error) {
	s.logger.Info(ctx, "Sending ListServices command")
	if err := s.conn.Send(eip.CommandListServices, nil, 0); err != nil {
		return nil, err
	}

	header, respData, err := s.conn.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("ListServices command failed with status: 0x%08X", header.Status)
	}

	return eip.DecodeListServicesResponse(respData)
}
