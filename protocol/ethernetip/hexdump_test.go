package ethernetip

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// TestHexDumpEIPRegisterSession verifies that WithHexDump captures the
// EtherNet/IP RegisterSession handshake traffic over net.Pipe.
func TestHexDumpEIPRegisterSession(t *testing.T) {
	router := cip.NewMessageRouter()
	_, clientConn := setupPipePair(t, router)

	var hexBuf bytes.Buffer

	tc, err := NewTCPConn("", WithConn(clientConn), WithHexDump(&hexBuf))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}

	sess := NewSession(tc, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	dump := hexBuf.String()

	// Must contain both directions from the RegisterSession handshake.
	if !strings.Contains(dump, ">>> WRITE") {
		t.Error("hex dump missing WRITE direction")
	}
	if !strings.Contains(dump, "<<< READ") {
		t.Error("hex dump missing READ direction")
	}

	// Must contain hex offset column.
	if !strings.Contains(dump, "00000000") {
		t.Error("hex dump missing offset column")
	}

	// Must contain ASCII column delimiters.
	if !strings.Contains(dump, "|") {
		t.Error("hex dump missing ASCII column")
	}

	sess.Unregister(ctx)
	tc.Close()

	t.Logf("Hex dump output:\n%s", dump)
}

// TestHexDumpEIPCIPRequest verifies that WithHexDump captures CIP
// request/response traffic (SendRRData) through a full client round-trip.
func TestHexDumpEIPCIPRequest(t *testing.T) {
	router := cip.NewMessageRouter()

	// Register a mock object that returns fixed data.
	router.RegisterObject(cip.UINT(0x66), &mockObject{
		handleFunc: func(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
			return []byte{0xDE, 0xAD, 0xBE, 0xEF}, nil
		},
	})

	_, clientConn := setupPipePair(t, router)

	var hexBuf bytes.Buffer

	tc, err := NewTCPConn("", WithConn(clientConn), WithHexDump(&hexBuf))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}

	sess := NewSession(tc, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Send a CIP request.
	p := cip.NewPath()
	p.AddClass(0x66)
	p.AddInstance(1)
	req := &cip.MessageRouterRequest{
		Service:     0x01, // Get Attributes All
		RequestPath: p,
	}

	resp, err := sess.SendCIPRequest(ctx, req)
	if err != nil {
		t.Fatalf("SendCIPRequest: %v", err)
	}
	if !resp.IsSuccess() {
		t.Fatalf("CIP request failed: %v", resp.Error())
	}

	sess.Unregister(ctx)
	tc.Close()

	dump := hexBuf.String()

	// Should have multiple WRITE/READ pairs (RegisterSession + SendRRData).
	writeCount := strings.Count(dump, ">>> WRITE")
	readCount := strings.Count(dump, "<<< READ")

	if writeCount < 2 {
		t.Errorf("expected at least 2 WRITE blocks (register + rr), got %d", writeCount)
	}
	if readCount < 2 {
		t.Errorf("expected at least 2 READ blocks (register + rr), got %d", readCount)
	}

	t.Logf("Hex dump output:\n%s", dump)
}

// TestHexDumpEIPAlignedColumns verifies that all hex dump lines in an
// EtherNet/IP exchange have consistent column alignment.
func TestHexDumpEIPAlignedColumns(t *testing.T) {
	router := cip.NewMessageRouter()
	_, clientConn := setupPipePair(t, router)

	var hexBuf bytes.Buffer

	tc, err := NewTCPConn("", WithConn(clientConn), WithHexDump(&hexBuf))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}

	sess := NewSession(tc, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sess.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	sess.Unregister(ctx)
	tc.Close()

	dump := hexBuf.String()
	lines := strings.Split(strings.TrimRight(dump, "\n"), "\n")

	// Collect hex data lines (starting with an offset).
	var hexLines []string
	for _, line := range lines {
		if len(line) >= 8 && line[0] == '0' {
			hexLines = append(hexLines, line)
		}
	}

	if len(hexLines) == 0 {
		t.Fatal("no hex data lines found in dump")
	}

	// All hex lines should have the same length.
	expectedLen := len(hexLines[0])
	for i, line := range hexLines {
		if len(line) != expectedLen {
			t.Errorf("hex line %d has length %d, expected %d:\n%q", i, len(line), expectedLen, line)
		}
	}
}
