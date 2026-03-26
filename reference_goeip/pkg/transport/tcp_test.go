package transport

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/iceisfun/goeip/pkg/eip"
)

func TestTCPTransportSend(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	transport := &TCPTransport{conn: client}

	cmd := eip.Command(0x0065)
	payload := []byte{0x01, 0x02, 0x03}
	session := eip.SessionHandle(0x12345678)

	errCh := make(chan error, 1)
	go func() {
		header := &eip.EncapsulationHeader{}
		if err := header.Decode(server); err != nil {
			errCh <- fmt.Errorf("decode header: %w", err)
			return
		}

		if header.Command != cmd {
			errCh <- fmt.Errorf("command mismatch: got 0x%04X want 0x%04X", header.Command, cmd)
			return
		}
		if header.Length != uint16(len(payload)) {
			errCh <- fmt.Errorf("length mismatch: got %d want %d", header.Length, len(payload))
			return
		}
		if header.SessionHandle != eip.SessionHandle(session) {
			errCh <- fmt.Errorf("session mismatch: got 0x%08X want 0x%08X", header.SessionHandle, session)
			return
		}
		if header.Status != 0 {
			errCh <- fmt.Errorf("status mismatch: got %d want 0", header.Status)
			return
		}
		if header.Options != 0 {
			errCh <- fmt.Errorf("options mismatch: got %d want 0", header.Options)
			return
		}
		if header.SenderContext != ([8]byte{}) {
			errCh <- fmt.Errorf("sender context mismatch: got %v want zero", header.SenderContext)
			return
		}

		buf := make([]byte, header.Length)
		if _, err := io.ReadFull(server, buf); err != nil {
			errCh <- fmt.Errorf("read data: %w", err)
			return
		}
		if !bytes.Equal(buf, payload) {
			errCh <- fmt.Errorf("payload mismatch: got %v want %v", buf, payload)
			return
		}
		errCh <- nil
	}()

	if err := transport.Send(cmd, payload, session); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("unexpected server side failure: %v", err)
	}
}

func TestTCPTransportReceive(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	transport := &TCPTransport{conn: client}

	payload := []byte{0x0A, 0x0B, 0x0C, 0x0D}
	expectedHeader := eip.EncapsulationHeader{
		Command:       eip.Command(0x006F),
		Length:        uint16(len(payload)),
		SessionHandle: 0xCAFEBABE,
		Status:        0x0001,
		SenderContext: [8]byte{0, 1, 2, 3, 4, 5, 6, 7},
		Options:       0x0002,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := expectedHeader.Encode(server); err != nil {
			errCh <- fmt.Errorf("encode header: %w", err)
			return
		}
		if _, err := server.Write(payload); err != nil {
			errCh <- fmt.Errorf("write payload: %w", err)
			return
		}
		errCh <- nil
	}()

	header, data, err := transport.Receive()
	if err != nil {
		t.Fatalf("Receive returned error: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("server send failed: %v", err)
	}

	if header.Command != expectedHeader.Command {
		t.Fatalf("command mismatch: got 0x%04X want 0x%04X", header.Command, expectedHeader.Command)
	}
	if header.Length != expectedHeader.Length {
		t.Fatalf("length mismatch: got %d want %d", header.Length, expectedHeader.Length)
	}
	if header.SessionHandle != expectedHeader.SessionHandle {
		t.Fatalf("session mismatch: got 0x%08X want 0x%08X", header.SessionHandle, expectedHeader.SessionHandle)
	}
	if header.Status != expectedHeader.Status {
		t.Fatalf("status mismatch: got %d want %d", header.Status, expectedHeader.Status)
	}
	if header.SenderContext != expectedHeader.SenderContext {
		t.Fatalf("sender context mismatch: got %v want %v", header.SenderContext, expectedHeader.SenderContext)
	}
	if header.Options != expectedHeader.Options {
		t.Fatalf("options mismatch: got %d want %d", header.Options, expectedHeader.Options)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("payload mismatch: got %v want %v", data, payload)
	}
}
