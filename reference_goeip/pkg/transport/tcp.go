package transport

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/iceisfun/goeip/pkg/eip"
)

// Transport defines the interface for sending and receiving EIP packets
type Transport interface {
	Send(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error
	Receive() (*eip.EncapsulationHeader, []byte, error)
	Close() error
}

// TCPTransport implements Transport using TCP
type TCPTransport struct {
	conn net.Conn
	wmu  sync.Mutex // protects concurrent writes
}

// NewTCPTransport creates a new TCP transport
func NewTCPTransport(address string) (*TCPTransport, error) {
	if !strings.Contains(address, ":") {
		address = address + ":44818"
	}

	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &TCPTransport{conn: conn}, nil
}

// Send sends an EIP packet
func (t *TCPTransport) Send(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error {
	header := eip.EncapsulationHeader{
		Command:       cmd,
		Length:        uint16(len(data)),
		SessionHandle: sessionHandle,
		Status:        0,
		SenderContext: [8]byte{}, // TODO: Allow setting context?
		Options:       0,
	}

	t.wmu.Lock()
	defer t.wmu.Unlock()

	// Write Header
	if err := header.Encode(t.conn); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write Data
	if len(data) > 0 {
		if _, err := t.conn.Write(data); err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}

	return nil
}

// Receive receives an EIP packet
func (t *TCPTransport) Receive() (*eip.EncapsulationHeader, []byte, error) {
	header := &eip.EncapsulationHeader{}
	if err := header.Decode(t.conn); err != nil {
		return nil, nil, fmt.Errorf("failed to read header: %w", err)
	}

	var data []byte
	if header.Length > 0 {
		data = make([]byte, header.Length)
		if _, err := io.ReadFull(t.conn, data); err != nil {
			return nil, nil, fmt.Errorf("failed to read data: %w", err)
		}
	}

	return header, data, nil
}

// Close closes the connection
func (t *TCPTransport) Close() error {
	return t.conn.Close()
}
