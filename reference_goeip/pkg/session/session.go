package session

import (
	"fmt"
	"sync"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
	"github.com/iceisfun/goeip/pkg/eip"
	"github.com/iceisfun/goeip/pkg/transport"
	"github.com/iceisfun/goeip/pkg/utils"
)

// Session represents an EIP session
type Session struct {
	transport     transport.Transport
	sessionHandle eip.SessionHandle
	logger        internal.Logger
	mu            sync.Mutex // protects sessionHandle
}

// NewSession creates a new session
func NewSession(t transport.Transport, l internal.Logger) *Session {
	if l == nil {
		l = internal.NopLogger()
	}
	return &Session{
		transport: t,
		logger:    l,
	}
}

// Register registers the session with the target
func (s *Session) Register() error {
	regData := eip.NewRegisterSessionData()
	data, err := regData.Encode()
	if err != nil {
		return err
	}

	s.logger.Infof("Sending RegisterSession command")
	if err := s.transport.Send(eip.CommandRegisterSession, data, 0); err != nil {
		return err
	}

	header, respData, err := s.transport.Receive()
	if err != nil {
		return err
	}

	if header.Status != eip.StatusSuccess {
		return fmt.Errorf("register session failed with status: 0x%08X", header.Status)
	}

	s.mu.Lock()
	s.sessionHandle = header.SessionHandle
	s.mu.Unlock()
	s.logger.Infof("Session registered. Handle: 0x%08X", header.SessionHandle)
	s.logger.Debugf("RegisterSession Response Data:\n%s", utils.HexDump(respData))

	return nil
}

// Unregister unregisters the session
func (s *Session) Unregister() error {
	s.mu.Lock()
	handle := s.sessionHandle
	s.mu.Unlock()
	s.logger.Infof("Sending UnregisterSession command")
	return s.transport.Send(eip.CommandUnregisterSession, nil, handle)
}

// Close closes the underlying transport
func (s *Session) Close() error {
	return s.transport.Close()
}

// SendRRData sends a Request/Response Data packet (Unconnected Message)
func (s *Session) SendRRData(request []byte) ([]byte, error) {
	// Construct CPF
	// Item 0: Null Address (0x0000) - Length 0
	// Item 1: Unconnected Data (0x00B2) - Length len(request)
	cpf := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, request),
	)

	cpfData, err := cpf.Encode()
	if err != nil {
		return nil, err
	}

	// Prepend Interface Handle (0) and Timeout (0)
	// Interface Handle: 4 bytes
	// Timeout: 2 bytes
	rrData := make([]byte, 6+len(cpfData))
	copy(rrData[6:], cpfData)

	// Send CommandSendRRData
	s.mu.Lock()
	handle := s.sessionHandle
	s.mu.Unlock()
	s.logger.Debugf("Sending RRData (len=%d)", len(rrData))
	if err := s.transport.Send(eip.CommandSendRRData, rrData, handle); err != nil {
		return nil, err
	}

	// Receive Response
	header, respData, err := s.transport.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("RRData command failed with status: 0x%08X", header.Status)
	}

	// Response also contains Interface Handle (4 bytes) and Timeout (2 bytes)
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

// SendCIPRequest sends a CIP request via SendRRData and returns the CIP response
func (s *Session) SendCIPRequest(req *cip.MessageRouterRequest) (*cip.MessageRouterResponse, error) {
	reqBytes, err := req.Encode()
	if err != nil {
		return nil, err
	}

	s.logger.Debugf("Sending CIP Request:\n%s", utils.HexDump(reqBytes))

	respBytes, err := s.SendRRData(reqBytes)
	if err != nil {
		return nil, err
	}

	s.logger.Debugf("Received CIP Response:\n%s", utils.HexDump(respBytes))

	return cip.DecodeMessageRouterResponse(respBytes)
}

// ListIdentity sends the ListIdentity command
func (s *Session) ListIdentity() ([]eip.ListIdentityItem, error) {
	s.logger.Infof("Sending ListIdentity command")
	// ListIdentity (0x63)
	if err := s.transport.Send(eip.CommandListIdentity, nil, 0); err != nil {
		return nil, err
	}

	header, respData, err := s.transport.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("ListIdentity command failed with status: 0x%08X", header.Status)
	}

	s.logger.Debugf("ListIdentity Response Data:\n%s", utils.HexDump(respData))

	return eip.DecodeListIdentityResponse(respData)
}

// ListServices sends the ListServices command
func (s *Session) ListServices() ([]eip.ListServicesItem, error) {
	s.logger.Infof("Sending ListServices command")
	// ListServices (0x04)
	if err := s.transport.Send(eip.CommandListServices, nil, 0); err != nil {
		return nil, err
	}

	header, respData, err := s.transport.Receive()
	if err != nil {
		return nil, err
	}

	if header.Status != eip.StatusSuccess {
		return nil, fmt.Errorf("ListServices command failed with status: 0x%08X", header.Status)
	}

	return eip.DecodeListServicesResponse(respData)
}
