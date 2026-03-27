package ethernetip

import (
	"context"
	"fmt"
	"sync"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)

// Session represents a registered EtherNet/IP session over a TCP connection.
// A session is established by sending a RegisterSession command, which returns
// a session handle used to identify all subsequent requests. Use [NewSession]
// and [Session.Register] to create one, or let [SessionConnector] do it
// automatically.
//
// All methods that perform a send/receive exchange are serialized by an
// internal mutex, so a Session is safe for concurrent use from multiple
// goroutines.
type Session struct {
	conn   *TCPConn
	handle eip.SessionHandle
	logger logging.Logger
	mu     sync.Mutex // protects handle
	ioMu   sync.Mutex // serializes send/receive exchanges
}

// NewSession creates a new unregistered Session on top of the given [TCPConn].
// Call [Session.Register] to obtain a session handle from the remote device.
// If logger is nil a no-op logger is used.
func NewSession(conn *TCPConn, logger logging.Logger) *Session {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &Session{
		conn:   conn,
		logger: logger,
	}
}

// Register sends the EIP RegisterSession command and stores the returned
// session handle for use in all subsequent requests on this session.
func (s *Session) Register(ctx context.Context) error {
	regData := eip.NewRegisterSessionData()
	data, err := regData.Encode()
	if err != nil {
		return err
	}

	s.ioMu.Lock()
	defer s.ioMu.Unlock()

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

// Unregister sends the EIP UnregisterSession command to release the session
// handle on the remote device.
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

// SendRRData sends a SendRRData (Request/Response Data) command carrying an
// unconnected CIP message. The request bytes are wrapped in a Common Packet
// Format (CPF) envelope with a Null Address item and an Unconnected Message
// item. The returned bytes are the Unconnected Message data extracted from the
// response CPF.
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

	s.ioMu.Lock()
	defer s.ioMu.Unlock()

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

// SendCIPRequest encodes a [cip.MessageRouterRequest], sends it via
// [Session.SendRRData], and decodes the CIP message router response.
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

// ListIdentity sends the EIP ListIdentity command and returns the identity
// items reported by the remote device.
func (s *Session) ListIdentity(ctx context.Context) ([]eip.ListIdentityItem, error) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

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

// ListServices sends the EIP ListServices command and returns the service
// items describing the communication capabilities of the remote device.
func (s *Session) ListServices(ctx context.Context) ([]eip.ListServicesItem, error) {
	s.ioMu.Lock()
	defer s.ioMu.Unlock()

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
