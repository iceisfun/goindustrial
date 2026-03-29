package modbus

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/transport"
)

// TestHexDumpModbusReadRegisters verifies that WithHexDump captures Modbus TCP
// wire traffic (both WRITE and READ directions) when reading holding registers
// through a full client/server round-trip over net.Pipe.
func TestHexDumpModbusReadRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate holding registers.
	store.SetHoldingRegister(0, 0x0001)
	store.SetHoldingRegister(1, 0x0002)
	store.SetHoldingRegister(2, 0x0003)

	var hexBuf bytes.Buffer

	connector := NewTCPConnector("",
		WithConn(clientConn),
		WithHexDump(&hexBuf),
	)
	closer := NewTCPCloser()

	tp := transport.NewReconnectingTransport[*TCPConn](connector, closer)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient(tp, WithUnitID(1))
	defer client.Close()

	regs, err := client.ReadHoldingRegisters(ctx, 0, 3)
	if err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}

	// Verify actual data.
	if len(regs) != 3 || regs[0] != 1 || regs[1] != 2 || regs[2] != 3 {
		t.Fatalf("unexpected register values: %v", regs)
	}

	dump := hexBuf.String()

	// Must contain both directions.
	if !strings.Contains(dump, ">>> WRITE") {
		t.Error("hex dump missing WRITE direction")
	}
	if !strings.Contains(dump, "<<< READ") {
		t.Error("hex dump missing READ direction")
	}

	// Must contain hex offset.
	if !strings.Contains(dump, "00000000") {
		t.Error("hex dump missing offset column")
	}

	// Must contain ASCII column delimiters.
	if !strings.Contains(dump, "|") {
		t.Error("hex dump missing ASCII column")
	}

	t.Logf("Hex dump output:\n%s", dump)
}

// TestHexDumpModbusWriteRegister verifies that WithHexDump captures wire
// traffic for write operations.
func TestHexDumpModbusWriteRegister(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	var hexBuf bytes.Buffer

	connector := NewTCPConnector("",
		WithConn(clientConn),
		WithHexDump(&hexBuf),
	)
	closer := NewTCPCloser()

	tp := transport.NewReconnectingTransport[*TCPConn](connector, closer)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient(tp, WithUnitID(1))
	defer client.Close()

	if err := client.WriteSingleRegister(ctx, 100, 0xABCD); err != nil {
		t.Fatalf("WriteSingleRegister: %v", err)
	}

	dump := hexBuf.String()

	if !strings.Contains(dump, ">>> WRITE") {
		t.Error("hex dump missing WRITE direction for write request")
	}
	if !strings.Contains(dump, "<<< READ") {
		t.Error("hex dump missing READ direction for write response")
	}

	t.Logf("Hex dump output:\n%s", dump)
}

// TestHexDumpModbusAlignedColumns verifies that all hex dump lines in a
// Modbus exchange have consistent column alignment (short lines are padded).
func TestHexDumpModbusAlignedColumns(t *testing.T) {
	_, clientConn, store := setupPipePair(t)
	store.SetHoldingRegister(0, 42)

	var hexBuf bytes.Buffer

	connector := NewTCPConnector("",
		WithConn(clientConn),
		WithHexDump(&hexBuf),
	)
	closer := NewTCPCloser()

	tp := transport.NewReconnectingTransport[*TCPConn](connector, closer)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient(tp, WithUnitID(1))
	defer client.Close()

	if _, err := client.ReadHoldingRegisters(ctx, 0, 1); err != nil {
		t.Fatalf("ReadHoldingRegisters: %v", err)
	}

	dump := hexBuf.String()
	lines := strings.Split(strings.TrimRight(dump, "\n"), "\n")

	// Collect all hex data lines (lines starting with an offset).
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
