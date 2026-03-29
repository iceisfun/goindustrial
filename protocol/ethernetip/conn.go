package ethernetip

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
)

// TCPConn implements EtherNet/IP encapsulation packet transport over a single
// TCP connection. It serializes writes with an internal mutex so it is safe
// for concurrent use.
type TCPConn struct {
	conn   net.Conn
	reader io.Reader
	writer io.Writer
	wmu    sync.Mutex // protects concurrent writes
}

// NewTCPConn dials a TCP connection to the given address for EtherNet/IP
// communication. If no port is specified in the address, the default EIP port
// 44818 is appended. Use [WithConn] to inject a pre-existing net.Conn
// (e.g. from net.Pipe) for testing.
func NewTCPConn(address string, opts ...ConnOption) (*TCPConn, error) {
	cfg := connConfig{
		dialTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	// If a connection was injected, use it directly.
	var rawConn net.Conn
	if cfg.conn != nil {
		rawConn = cfg.conn
	} else {
		if !strings.Contains(address, ":") {
			address = address + ":44818"
		}
		c, err := net.DialTimeout("tcp", address, cfg.dialTimeout)
		if err != nil {
			return nil, err
		}
		rawConn = c
	}

	tc := &TCPConn{
		conn:   rawConn,
		reader: rawConn,
		writer: rawConn,
	}
	if cfg.hexDumper != nil {
		tc.reader = cfg.hexDumper.WrapReader(rawConn)
		tc.writer = cfg.hexDumper.WrapWriter(rawConn)
	}
	return tc, nil
}

// Send writes a complete EIP encapsulation packet (24-byte header + data) to
// the TCP connection using the given command code and session handle.
func (t *TCPConn) Send(cmd eip.Command, data []byte, sessionHandle eip.SessionHandle) error {
	header := eip.EncapsulationHeader{
		Command:       cmd,
		Length:        uint16(len(data)),
		SessionHandle: sessionHandle,
		Status:        0,
		SenderContext: [8]byte{},
		Options:       0,
	}

	t.wmu.Lock()
	defer t.wmu.Unlock()

	// Write Header
	if err := header.Encode(t.writer); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write Data
	if len(data) > 0 {
		if _, err := t.writer.Write(data); err != nil {
			return fmt.Errorf("failed to write data: %w", err)
		}
	}

	return nil
}

// Receive reads a complete EIP encapsulation packet from the connection and
// returns the parsed header and the command-specific data payload.
func (t *TCPConn) Receive() (*eip.EncapsulationHeader, []byte, error) {
	header := &eip.EncapsulationHeader{}
	if err := header.Decode(t.reader); err != nil {
		return nil, nil, fmt.Errorf("failed to read header: %w", err)
	}

	var data []byte
	if header.Length > 0 {
		data = make([]byte, header.Length)
		if _, err := io.ReadFull(t.reader, data); err != nil {
			return nil, nil, fmt.Errorf("failed to read data: %w", err)
		}
	}

	return header, data, nil
}

// Close closes the underlying TCP connection.
func (t *TCPConn) Close() error {
	return t.conn.Close()
}
