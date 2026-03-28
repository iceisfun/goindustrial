package modbus

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/transport"
)

// ---------------------------------------------------------------------------
// pipeListener implements net.Listener by returning a pre-created conn on the
// first Accept() call, then blocking until Close() is called.
// This enables the full Server.Start() path with net.Pipe.
// ---------------------------------------------------------------------------

type pipeListener struct {
	connCh chan net.Conn
	done   chan struct{}
	once   sync.Once
}

func newPipeListener(serverConn net.Conn) *pipeListener {
	ch := make(chan net.Conn, 1)
	ch <- serverConn
	return &pipeListener{
		connCh: ch,
		done:   make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.connCh:
		return c, nil
	case <-l.done:
		return nil, &net.OpError{Op: "accept", Net: "pipe", Err: net.ErrClosed}
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// pipeAddr implements net.Addr for the pipe listener.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

// ---------------------------------------------------------------------------
// Helper: create a server + client pair over net.Pipe using WithServerConn.
// Returns the server, the client-side conn, and the MemoryStore for
// pre-populating / inspecting data.
// ---------------------------------------------------------------------------

func setupPipePair(t *testing.T, opts ...ServerOption) (*Server, net.Conn, *MemoryStore) {
	t.Helper()

	serverConn, clientConn := net.Pipe()

	store := NewMemoryStore()
	allOpts := append([]ServerOption{
		WithServerDataStore(store),
		WithServerConn(serverConn),
	}, opts...)

	srv := NewServer("test", allOpts...)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	t.Cleanup(func() {
		clientConn.Close()
		srv.Stop(context.Background())
	})

	return srv, clientConn, store
}

// ---------------------------------------------------------------------------
// Helper: create a server + client pair using a pipeListener, which exercises
// the full Server.Start() -> acceptLoop path.
// ---------------------------------------------------------------------------

func setupPipeListenerPair(t *testing.T, opts ...ServerOption) (*Server, net.Conn, *MemoryStore) {
	t.Helper()

	serverConn, clientConn := net.Pipe()

	store := NewMemoryStore()
	pl := newPipeListener(serverConn)

	allOpts := append([]ServerOption{
		WithServerDataStore(store),
		WithServerListener(pl),
	}, opts...)

	srv := NewServer("test", allOpts...)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	// Give the accept loop a moment to accept the connection.
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		clientConn.Close()
		srv.Stop(context.Background())
	})

	return srv, clientConn, store
}

// ---------------------------------------------------------------------------
// Wire-level helpers: build raw Modbus TCP frames and read responses.
// ---------------------------------------------------------------------------

// sendRawRequest encodes and sends a Modbus TCP request over a conn.
func sendRawRequest(t *testing.T, conn net.Conn, txID TransactionID, unitID UnitID, fc FunctionCode, pduData []byte) {
	t.Helper()
	req := NewRequest(unitID, fc, pduData)
	req.SetTransactionID(txID)
	data, err := req.Encode()
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

// readRawResponse reads a full Modbus TCP response from a conn.
func readRawResponse(t *testing.T, conn net.Conn) *Response {
	t.Helper()

	header := make([]byte, TCPHeaderLength)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read response header: %v", err)
	}

	length := binary.BigEndian.Uint16(header[4:6])
	body := make([]byte, int(length)-1) // -1 for unitID already in header
	if _, err := io.ReadFull(conn, body); err != nil {
		t.Fatalf("read response body: %v", err)
	}

	// Reconstruct full frame for Decode.
	full := make([]byte, 0, TCPHeaderLength+len(body))
	full = append(full, header...)
	full = append(full, body...)

	resp := &Response{}
	if err := resp.Decode(full); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// makeReadRegistersPDU builds the PDU data for a ReadHoldingRegisters request.
func makeReadRegistersPDU(address Address, quantity Quantity) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], uint16(quantity))
	return data
}

// makeWriteSingleRegisterPDU builds the PDU data for a WriteSingleRegister request.
func makeWriteSingleRegisterPDU(address Address, value RegisterValue) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], value)
	return data
}

// makeWriteMultipleRegistersPDU builds the PDU data for a WriteMultipleRegisters request.
func makeWriteMultipleRegistersPDU(address Address, values []RegisterValue) []byte {
	quantity := uint16(len(values))
	byteCount := byte(len(values) * 2)
	data := make([]byte, 5+int(byteCount))
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], quantity)
	data[4] = byteCount
	for i, v := range values {
		binary.BigEndian.PutUint16(data[5+i*2:5+i*2+2], v)
	}
	return data
}

// makeReadCoilsPDU builds the PDU data for a ReadCoils request.
func makeReadCoilsPDU(address Address, quantity Quantity) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], uint16(quantity))
	return data
}

// makeWriteSingleCoilPDU builds the PDU data for a WriteSingleCoil request.
func makeWriteSingleCoilPDU(address Address, on bool) []byte {
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	if on {
		binary.BigEndian.PutUint16(data[2:4], CoilOnU16)
	} else {
		binary.BigEndian.PutUint16(data[2:4], CoilOffU16)
	}
	return data
}

// makeWriteMultipleCoilsPDU builds the PDU data for a WriteMultipleCoils request.
func makeWriteMultipleCoilsPDU(address Address, values []bool) []byte {
	quantity := uint16(len(values))
	byteCount := (len(values) + 7) / 8
	data := make([]byte, 5+byteCount)
	binary.BigEndian.PutUint16(data[0:2], uint16(address))
	binary.BigEndian.PutUint16(data[2:4], quantity)
	data[4] = byte(byteCount)
	for i, v := range values {
		if v {
			data[5+i/8] |= 1 << uint(i%8)
		}
	}
	return data
}

// ===========================================================================
// Tests
// ===========================================================================

func TestReadHoldingRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate registers.
	store.SetHoldingRegister(100, 0x1234)
	store.SetHoldingRegister(101, 0x5678)
	store.SetHoldingRegister(102, 0x9ABC)

	sendRawRequest(t, clientConn, 1, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(100, 3))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	if resp.GetTransactionID() != 1 {
		t.Fatalf("transaction ID mismatch: got %d, want 1", resp.GetTransactionID())
	}
	if resp.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Fatalf("function code mismatch: got %d, want %d", resp.GetPDU().FunctionCode, FuncReadHoldingRegisters)
	}

	// Response data: byte count (1) + 3 registers (6 bytes)
	pduData := resp.GetPDU().Data
	if len(pduData) < 7 {
		t.Fatalf("response data too short: %d bytes", len(pduData))
	}
	if pduData[0] != 6 {
		t.Fatalf("byte count mismatch: got %d, want 6", pduData[0])
	}
	reg0 := binary.BigEndian.Uint16(pduData[1:3])
	reg1 := binary.BigEndian.Uint16(pduData[3:5])
	reg2 := binary.BigEndian.Uint16(pduData[5:7])
	if reg0 != 0x1234 || reg1 != 0x5678 || reg2 != 0x9ABC {
		t.Fatalf("register values mismatch: got %04X %04X %04X, want 1234 5678 9ABC", reg0, reg1, reg2)
	}
}

func TestWriteSingleRegister(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	sendRawRequest(t, clientConn, 2, 1, FuncWriteSingleRegister,
		makeWriteSingleRegisterPDU(200, 0xABCD))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	if resp.GetTransactionID() != 2 {
		t.Fatalf("transaction ID mismatch: got %d, want 2", resp.GetTransactionID())
	}

	// Verify the echo.
	pduData := resp.GetPDU().Data
	addr := binary.BigEndian.Uint16(pduData[0:2])
	val := binary.BigEndian.Uint16(pduData[2:4])
	if addr != 200 || val != 0xABCD {
		t.Fatalf("echo mismatch: addr=%d val=%04X", addr, val)
	}

	// Verify in the data store.
	v, ok := store.GetHoldingRegister(200)
	if !ok || v != 0xABCD {
		t.Fatalf("data store: got %v/%04X, want true/ABCD", ok, v)
	}
}

func TestWriteMultipleRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	values := []RegisterValue{0x0001, 0x0002, 0x0003, 0x0004}
	sendRawRequest(t, clientConn, 3, 1, FuncWriteMultipleRegisters,
		makeWriteMultipleRegistersPDU(300, values))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	// Verify response: address + quantity echo.
	pduData := resp.GetPDU().Data
	respAddr := binary.BigEndian.Uint16(pduData[0:2])
	respQty := binary.BigEndian.Uint16(pduData[2:4])
	if respAddr != 300 || respQty != 4 {
		t.Fatalf("response mismatch: addr=%d qty=%d", respAddr, respQty)
	}

	// Read them back.
	sendRawRequest(t, clientConn, 4, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(300, 4))

	resp2 := readRawResponse(t, clientConn)
	if resp2.IsException() {
		t.Fatalf("read-back exception: %s", resp2.GetException())
	}

	pduData2 := resp2.GetPDU().Data
	for i := 0; i < 4; i++ {
		got := binary.BigEndian.Uint16(pduData2[1+i*2 : 1+i*2+2])
		if got != values[i] {
			t.Fatalf("register %d: got %04X, want %04X", 300+i, got, values[i])
		}
	}

	// Also verify directly in the store.
	for i, want := range values {
		v, ok := store.GetHoldingRegister(Address(300 + i))
		if !ok || v != want {
			t.Fatalf("store register %d: got %v/%04X, want true/%04X", 300+i, ok, v, want)
		}
	}
}

func TestReadCoils(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	store.SetCoil(0, true)
	store.SetCoil(1, false)
	store.SetCoil(2, true)
	store.SetCoil(3, true)
	store.SetCoil(4, false)
	store.SetCoil(5, true)
	store.SetCoil(6, false)
	store.SetCoil(7, true)
	store.SetCoil(8, true)
	store.SetCoil(9, false)

	sendRawRequest(t, clientConn, 5, 1, FuncReadCoils,
		makeReadCoilsPDU(0, 10))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	byteCount := pduData[0]
	if byteCount != 2 { // ceil(10/8) = 2
		t.Fatalf("byte count mismatch: got %d, want 2", byteCount)
	}

	// Verify bit packing: coils 0-7 in first byte, coils 8-9 in second byte.
	// Coil 0=1, 1=0, 2=1, 3=1, 4=0, 5=1, 6=0, 7=1 => 0b10101101 = 0xAD
	// Coil 8=1, 9=0 => 0b00000001 = 0x01
	if pduData[1] != 0xAD {
		t.Fatalf("byte 1 mismatch: got 0x%02X, want 0xAD", pduData[1])
	}
	if pduData[2] != 0x01 {
		t.Fatalf("byte 2 mismatch: got 0x%02X, want 0x01", pduData[2])
	}
}

func TestWriteSingleCoil(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Write ON.
	sendRawRequest(t, clientConn, 6, 1, FuncWriteSingleCoil,
		makeWriteSingleCoilPDU(50, true))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	v, ok := store.GetCoil(50)
	if !ok || !v {
		t.Fatalf("coil 50: got %v/%v, want true/true", ok, v)
	}

	// Write OFF.
	sendRawRequest(t, clientConn, 7, 1, FuncWriteSingleCoil,
		makeWriteSingleCoilPDU(50, false))

	resp2 := readRawResponse(t, clientConn)
	if resp2.IsException() {
		t.Fatalf("unexpected exception: %s", resp2.GetException())
	}

	v2, _ := store.GetCoil(50)
	if v2 {
		t.Fatalf("coil 50 should be OFF after second write")
	}
}

func TestWriteMultipleCoils(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	coils := []bool{true, false, true, true, false}
	sendRawRequest(t, clientConn, 8, 1, FuncWriteMultipleCoils,
		makeWriteMultipleCoilsPDU(60, coils))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	for i, want := range coils {
		got, ok := store.GetCoil(Address(60 + i))
		if !ok || got != want {
			t.Fatalf("coil %d: got %v/%v, want true/%v", 60+i, ok, got, want)
		}
	}
}

func TestExceptionUnsupportedFunction(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Send a function code 0x07 (ReadExceptionStatus) which has no handler.
	// First, remove the default handler if any.
	// ReadExceptionStatus (0x07) is not set up by default, so it should trigger exception.
	sendRawRequest(t, clientConn, 10, 1, FuncReadExceptionStatus, nil)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatalf("expected exception response, got normal response")
	}
	if resp.GetException() != ExceptionFunctionCodeNotSupported {
		t.Fatalf("exception code mismatch: got %d, want %d",
			resp.GetException(), ExceptionFunctionCodeNotSupported)
	}
	// Exception function code should have high bit set.
	if resp.GetPDU().FunctionCode != FunctionCode(byte(FuncReadExceptionStatus)|ExceptionBit) {
		t.Fatalf("exception function code mismatch: got 0x%02X, want 0x%02X",
			resp.GetPDU().FunctionCode, byte(FuncReadExceptionStatus)|ExceptionBit)
	}
}

func TestExceptionInvalidData(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Send a ReadHoldingRegisters with quantity 0 (invalid).
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], 0)   // address
	binary.BigEndian.PutUint16(data[2:4], 0)   // quantity = 0 (invalid)
	sendRawRequest(t, clientConn, 11, 1, FuncReadHoldingRegisters, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatalf("expected exception response for quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception code mismatch: got %d, want %d",
			resp.GetException(), ExceptionInvalidDataValue)
	}
}

func TestProtocolEncodingRoundTrip(t *testing.T) {
	// Test that encoding a request and decoding it back produces the same fields.
	original := NewRequest(UnitID(5), FuncReadHoldingRegisters, makeReadRegistersPDU(100, 10))
	original.SetTransactionID(42)

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded := &Request{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.GetTransactionID() != 42 {
		t.Fatalf("transaction ID: got %d, want 42", decoded.GetTransactionID())
	}
	if decoded.GetUnitID() != 5 {
		t.Fatalf("unit ID: got %d, want 5", decoded.GetUnitID())
	}
	if decoded.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Fatalf("function code: got %d, want %d", decoded.GetPDU().FunctionCode, FuncReadHoldingRegisters)
	}

	pduData := decoded.GetPDU().Data
	addr := binary.BigEndian.Uint16(pduData[0:2])
	qty := binary.BigEndian.Uint16(pduData[2:4])
	if addr != 100 || qty != 10 {
		t.Fatalf("PDU data: addr=%d qty=%d, want 100/10", addr, qty)
	}
}

func TestResponseEncodingRoundTrip(t *testing.T) {
	original := NewResponse(TransactionID(99), UnitID(3), FuncReadHoldingRegisters,
		[]byte{4, 0x12, 0x34, 0x56, 0x78})

	encoded, err := original.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded := &Response{}
	if err := decoded.Decode(encoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.GetTransactionID() != 99 {
		t.Fatalf("transaction ID: got %d, want 99", decoded.GetTransactionID())
	}
	if decoded.GetUnitID() != 3 {
		t.Fatalf("unit ID: got %d, want 3", decoded.GetUnitID())
	}
	if decoded.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Fatalf("function code mismatch")
	}
	if len(decoded.GetPDU().Data) != 5 {
		t.Fatalf("data length: got %d, want 5", len(decoded.GetPDU().Data))
	}
}

func TestMBAPEncode(t *testing.T) {
	m := MBAP{
		TransactionID: 0x0C05,
		ProtocolID:    TCPProtocolIdentifier,
		UnitID:        0x11,
	}
	var buf bytes.Buffer
	// pduLength = FC (1) + data (4) = 5
	if err := m.Encode(&buf, 5); err != nil {
		t.Fatalf("encode: %v", err)
	}

	got := buf.Bytes()
	// Expected: TID(2) + Proto(2) + Length(2) + Unit(1) = 7 bytes
	// Length = 1 (unit ID counted in MBAP) + 5 (PDU) = 6
	want := []byte{
		0x0C, 0x05, // Transaction ID = 3077
		0x00, 0x00, // Protocol ID = 0
		0x00, 0x06, // Length = 6
		0x11,       // Unit ID = 17
	}
	if len(got) != TCPHeaderLength {
		t.Fatalf("length: got %d, want %d", len(got), TCPHeaderLength)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, got[i], w)
		}
	}
}

func TestMBAPDecode(t *testing.T) {
	raw := []byte{
		0x0C, 0x05, // Transaction ID = 3077
		0x00, 0x00, // Protocol ID = 0
		0x00, 0x07, // Length = 7
		0x11,       // Unit ID = 17
	}
	reader := bytes.NewReader(raw)

	var m MBAP
	length, err := m.Decode(reader)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.TransactionID != 3077 {
		t.Fatalf("TID: got %d, want 3077", m.TransactionID)
	}
	if m.ProtocolID != TCPProtocolIdentifier {
		t.Fatalf("protocol ID: got %d, want 0", m.ProtocolID)
	}
	if m.UnitID != 0x11 {
		t.Fatalf("unit ID: got %d, want 0x11", m.UnitID)
	}
	if length != 7 {
		t.Fatalf("length: got %d, want 7", length)
	}
}

func TestMBAPRoundTrip(t *testing.T) {
	original := MBAP{
		TransactionID: 0xABCD,
		ProtocolID:    TCPProtocolIdentifier,
		UnitID:        42,
	}

	var buf bytes.Buffer
	if err := original.Encode(&buf, 10); err != nil {
		t.Fatalf("encode: %v", err)
	}

	reader := bytes.NewReader(buf.Bytes())
	var decoded MBAP
	length, err := decoded.Decode(reader)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.TransactionID != original.TransactionID {
		t.Fatalf("TID: got %d, want %d", decoded.TransactionID, original.TransactionID)
	}
	if decoded.ProtocolID != original.ProtocolID {
		t.Fatalf("protocol ID: got %d, want %d", decoded.ProtocolID, original.ProtocolID)
	}
	if decoded.UnitID != original.UnitID {
		t.Fatalf("unit ID: got %d, want %d", decoded.UnitID, original.UnitID)
	}
	// Length = 1 (unit ID) + 10 (pduLength) = 11
	if length != 11 {
		t.Fatalf("length: got %d, want 11", length)
	}
}

func TestMBAPDecodeShortData(t *testing.T) {
	// Only 4 bytes — not enough for the full 7-byte header.
	reader := bytes.NewReader([]byte{0x00, 0x01, 0x00, 0x00})
	var m MBAP
	_, err := m.Decode(reader)
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestMBAPHeaderEncoding(t *testing.T) {
	req := NewRequest(UnitID(7), FuncWriteSingleRegister, makeWriteSingleRegisterPDU(400, 0xBEEF))
	req.SetTransactionID(0x1234)

	encoded, err := req.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Verify MBAP header fields.
	txID := binary.BigEndian.Uint16(encoded[0:2])
	protoID := binary.BigEndian.Uint16(encoded[2:4])
	length := binary.BigEndian.Uint16(encoded[4:6])
	unitID := encoded[6]

	if txID != 0x1234 {
		t.Fatalf("transaction ID: got 0x%04X, want 0x1234", txID)
	}
	if protoID != 0 {
		t.Fatalf("protocol ID: got %d, want 0", protoID)
	}
	// Length = 1 (unit ID) + 1 (function code) + 4 (PDU data) = 6
	if length != 6 {
		t.Fatalf("length: got %d, want 6", length)
	}
	if unitID != 7 {
		t.Fatalf("unit ID: got %d, want 7", unitID)
	}

	// Verify function code after header.
	fc := encoded[7]
	if FunctionCode(fc) != FuncWriteSingleRegister {
		t.Fatalf("function code: got 0x%02X, want 0x%02X", fc, FuncWriteSingleRegister)
	}
}

func TestConcurrentOperations(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate some registers.
	for i := 0; i < 50; i++ {
		store.SetHoldingRegister(Address(i), RegisterValue(i*100))
	}

	const numWorkers = 10
	const opsPerWorker = 5

	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers*opsPerWorker)

	// We need to serialize actual I/O on the pipe since it is a single
	// stream. Use a mutex to ensure one request-response at a time.
	var mu sync.Mutex

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for op := 0; op < opsPerWorker; op++ {
				txID := TransactionID(workerID*1000 + op)
				addr := Address(workerID)

				mu.Lock()

				// Write a register.
				writeVal := RegisterValue(workerID*100 + op)
				sendRawRequest(t, clientConn, txID, 1, FuncWriteSingleRegister,
					makeWriteSingleRegisterPDU(addr, writeVal))

				resp := readRawResponse(t, clientConn)
				if resp.IsException() {
					errCh <- resp.ToError()
					mu.Unlock()
					return
				}
				if resp.GetTransactionID() != txID {
					errCh <- nil // signal issue
					mu.Unlock()
					return
				}

				// Read it back.
				readTxID := TransactionID(workerID*1000 + op + 500)
				sendRawRequest(t, clientConn, readTxID, 1, FuncReadHoldingRegisters,
					makeReadRegistersPDU(addr, 1))

				resp2 := readRawResponse(t, clientConn)
				if resp2.IsException() {
					errCh <- resp2.ToError()
					mu.Unlock()
					return
				}

				mu.Unlock()
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent operation error: %v", err)
		} else {
			t.Fatal("concurrent operation: unexpected mismatch")
		}
	}
}

func TestPipeListenerAcceptPath(t *testing.T) {
	// This test exercises the full Server.Start() -> acceptLoop path using
	// a pipeListener.
	_, clientConn, store := setupPipeListenerPair(t)

	store.SetHoldingRegister(500, 0xFACE)

	sendRawRequest(t, clientConn, 20, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(500, 1))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	val := binary.BigEndian.Uint16(pduData[1:3])
	if val != 0xFACE {
		t.Fatalf("register value: got 0x%04X, want 0xFACE", val)
	}
}

func TestServerStopClosesConnection(t *testing.T) {
	srv, clientConn, _ := setupPipePair(t)

	// Verify server is running.
	if !srv.IsRunning() {
		t.Fatal("server should be running")
	}

	// Stop the server.
	srv.Stop(context.Background())

	if srv.IsRunning() {
		t.Fatal("server should not be running after Stop")
	}

	// The client conn should now get an error on read (pipe closed).
	buf := make([]byte, 1)
	clientConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, err := clientConn.Read(buf)
	if err == nil {
		t.Fatal("expected error reading from client after server stop")
	}
}

func TestReadWriteMultipleRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate some registers.
	store.SetHoldingRegister(10, 0xAAAA)
	store.SetHoldingRegister(11, 0xBBBB)

	// Build ReadWriteMultipleRegisters PDU:
	// Read starting address: 10, quantity to read: 2
	// Write starting address: 20, quantity to write: 2, values: 0x1111, 0x2222
	pduData := make([]byte, 9+4)
	binary.BigEndian.PutUint16(pduData[0:2], 10) // read address
	binary.BigEndian.PutUint16(pduData[2:4], 2)  // read quantity
	binary.BigEndian.PutUint16(pduData[4:6], 20) // write address
	binary.BigEndian.PutUint16(pduData[6:8], 2)  // write quantity
	pduData[8] = 4                                 // byte count
	binary.BigEndian.PutUint16(pduData[9:11], 0x1111)
	binary.BigEndian.PutUint16(pduData[11:13], 0x2222)

	sendRawRequest(t, clientConn, 30, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	// Response should contain the read values.
	respData := resp.GetPDU().Data
	if respData[0] != 4 { // byte count = 2 registers * 2
		t.Fatalf("byte count: got %d, want 4", respData[0])
	}
	r0 := binary.BigEndian.Uint16(respData[1:3])
	r1 := binary.BigEndian.Uint16(respData[3:5])
	if r0 != 0xAAAA || r1 != 0xBBBB {
		t.Fatalf("read values: got %04X %04X, want AAAA BBBB", r0, r1)
	}

	// Verify the write happened.
	w0, _ := store.GetHoldingRegister(20)
	w1, _ := store.GetHoldingRegister(21)
	if w0 != 0x1111 || w1 != 0x2222 {
		t.Fatalf("written values: got %04X %04X, want 1111 2222", w0, w1)
	}
}

func TestWriteSingleCoilInvalidValue(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Send an invalid coil value (neither 0x0000 nor 0xFF00).
	data := make([]byte, 4)
	binary.BigEndian.PutUint16(data[0:2], 10)
	binary.BigEndian.PutUint16(data[2:4], 0x1234) // invalid
	sendRawRequest(t, clientConn, 40, 1, FuncWriteSingleCoil, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for invalid coil value")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception code: got %d, want %d", resp.GetException(), ExceptionInvalidDataValue)
	}
}

func TestConnectedClients(t *testing.T) {
	srv, clientConn, _ := setupPipePair(t)

	// Send a request to ensure the connection is tracked.
	sendRawRequest(t, clientConn, 50, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0, 1))
	_ = readRawResponse(t, clientConn)

	// Allow server goroutine to process.
	time.Sleep(20 * time.Millisecond)

	clients := srv.ConnectedClients()
	if len(clients) != 1 {
		t.Fatalf("expected 1 connected client, got %d", len(clients))
	}
	if clients[0].RxTransactions != 1 {
		t.Fatalf("rx transactions: got %d, want 1", clients[0].RxTransactions)
	}
	if clients[0].TxTransactions != 1 {
		t.Fatalf("tx transactions: got %d, want 1", clients[0].TxTransactions)
	}
}

func TestDataStoreDirectAccess(t *testing.T) {
	store := NewMemoryStore()

	// Test all direct accessors.
	store.SetCoil(0, true)
	v, ok := store.GetCoil(0)
	if !ok || !v {
		t.Fatal("SetCoil/GetCoil failed")
	}

	store.SetDiscreteInput(0, true)
	dv, ok := store.GetDiscreteInput(0)
	if !ok || !dv {
		t.Fatal("SetDiscreteInput/GetDiscreteInput failed")
	}

	store.SetHoldingRegister(0, 12345)
	rv, ok := store.GetHoldingRegister(0)
	if !ok || rv != 12345 {
		t.Fatal("SetHoldingRegister/GetHoldingRegister failed")
	}

	store.SetInputRegister(0, 54321)
	iv, ok := store.GetInputRegister(0)
	if !ok || iv != 54321 {
		t.Fatal("SetInputRegister/GetInputRegister failed")
	}
}

func TestMemoryStoreReadQuantityValidation(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Quantity 0 should fail.
	_, err := store.ReadCoils(ctx, 0, 0)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for coils qty=0, got %v", err)
	}

	_, err = store.ReadHoldingRegisters(ctx, 0, 0)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for registers qty=0, got %v", err)
	}

	// Quantity exceeding max should fail.
	_, err = store.ReadCoils(ctx, 0, MaxCoilCount+1)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for coils qty=%d, got %v", MaxCoilCount+1, err)
	}

	_, err = store.ReadHoldingRegisters(ctx, 0, MaxRegisterCount+1)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for registers qty=%d, got %v", MaxRegisterCount+1, err)
	}
}

// ===========================================================================
// Tests mined from libmodbus (https://github.com/stephane/libmodbus)
// These cover protocol conformance, boundary conditions, and edge cases
// from the authoritative C Modbus library.
// ===========================================================================

// --- FC 0x02: Read Discrete Inputs (missing from original test suite) ---

func TestReadDiscreteInputs(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate discrete inputs using libmodbus test vector pattern.
	// libmodbus: UT_INPUT_BITS_ADDRESS=0x1C4, UT_INPUT_BITS_NB=0x16 (22 bits)
	// Expected bytes: {0xAC, 0xDB, 0x35}
	inputBits := []bool{
		false, false, true, true, false, true, false, true, // 0xAC
		true, true, false, true, true, false, true, true, // 0xDB
		true, false, true, false, true, true, // 0x35 (6 bits)
	}
	for i, v := range inputBits {
		store.SetDiscreteInput(Address(0x1C4+i), v)
	}

	sendRawRequest(t, clientConn, 1, 1, FuncReadDiscreteInputs,
		makeReadCoilsPDU(0x1C4, 22)) // same PDU format as ReadCoils

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	if resp.GetPDU().FunctionCode != FuncReadDiscreteInputs {
		t.Fatalf("function code: got 0x%02X, want 0x02", resp.GetPDU().FunctionCode)
	}

	pduData := resp.GetPDU().Data
	if pduData[0] != 3 { // ceil(22/8) = 3 bytes
		t.Fatalf("byte count: got %d, want 3", pduData[0])
	}
	// Verify against libmodbus expected bytes.
	if pduData[1] != 0xAC {
		t.Fatalf("byte 1: got 0x%02X, want 0xAC", pduData[1])
	}
	if pduData[2] != 0xDB {
		t.Fatalf("byte 2: got 0x%02X, want 0xDB", pduData[2])
	}
	if pduData[3] != 0x35 {
		t.Fatalf("byte 3: got 0x%02X, want 0x35", pduData[3])
	}
}

// --- FC 0x04: Read Input Registers (missing from original test suite) ---

func TestReadInputRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// libmodbus: UT_INPUT_REGISTERS_ADDRESS=0x108, value={0x000A}
	store.SetInputRegister(0x108, 0x000A)

	sendRawRequest(t, clientConn, 2, 1, FuncReadInputRegisters,
		makeReadRegistersPDU(0x108, 1))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	if resp.GetPDU().FunctionCode != FuncReadInputRegisters {
		t.Fatalf("function code: got 0x%02X, want 0x04", resp.GetPDU().FunctionCode)
	}

	pduData := resp.GetPDU().Data
	if pduData[0] != 2 { // 1 register * 2 bytes
		t.Fatalf("byte count: got %d, want 2", pduData[0])
	}
	val := binary.BigEndian.Uint16(pduData[1:3])
	if val != 0x000A {
		t.Fatalf("register value: got 0x%04X, want 0x000A", val)
	}
}

func TestReadInputRegistersMultiple(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Pre-populate with libmodbus-style test values.
	store.SetInputRegister(0x108, 0x000A)
	store.SetInputRegister(0x109, 0x1234)
	store.SetInputRegister(0x10A, 0xFFFF)

	sendRawRequest(t, clientConn, 3, 1, FuncReadInputRegisters,
		makeReadRegistersPDU(0x108, 3))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	if pduData[0] != 6 {
		t.Fatalf("byte count: got %d, want 6", pduData[0])
	}
	expected := []uint16{0x000A, 0x1234, 0xFFFF}
	for i, want := range expected {
		got := binary.BigEndian.Uint16(pduData[1+i*2 : 3+i*2])
		if got != want {
			t.Fatalf("register %d: got 0x%04X, want 0x%04X", i, got, want)
		}
	}
}

// --- Bit packing test vectors ---

func TestReadCoilsBitPacking(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// libmodbus: UT_BITS_ADDRESS=0x130, UT_BITS_NB=0x25 (37 coils)
	// Expected bytes: {0xCD, 0x6B, 0xB2, 0x0E, 0x1B}
	//
	// Unpack 0xCD = 10110011 reversed = bit0=1,1=0,2=1,3=1,4=0,5=0,6=1,7=1
	// Wait - libmodbus packs LSB first. Let me unpack properly.
	// 0xCD = 11001101 -> bit0=1, bit1=0, bit2=1, bit3=1, bit4=0, bit5=0, bit6=1, bit7=1
	coilBytes := []byte{0xCD, 0x6B, 0xB2, 0x0E, 0x1B}
	for i := 0; i < 37; i++ {
		byteIdx := i / 8
		bitIdx := i % 8
		val := (coilBytes[byteIdx] >> uint(bitIdx)) & 1
		store.SetCoil(Address(0x130+i), val == 1)
	}

	sendRawRequest(t, clientConn, 10, 1, FuncReadCoils,
		makeReadCoilsPDU(0x130, 37))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	if pduData[0] != 5 { // ceil(37/8) = 5
		t.Fatalf("byte count: got %d, want 5", pduData[0])
	}
	for i, want := range coilBytes {
		got := pduData[1+i]
		if got != want {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, got, want)
		}
	}
}

// --- Register value test vectors ---

func TestReadHoldingRegistersMultipleValues(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// libmodbus: UT_REGISTERS_ADDRESS=0x160, values={0x022B, 0x0001, 0x0064}
	store.SetHoldingRegister(0x160, 0x022B)
	store.SetHoldingRegister(0x161, 0x0001)
	store.SetHoldingRegister(0x162, 0x0064)

	sendRawRequest(t, clientConn, 11, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0x160, 3))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	expected := []uint16{0x022B, 0x0001, 0x0064}
	for i, want := range expected {
		got := binary.BigEndian.Uint16(pduData[1+i*2 : 3+i*2])
		if got != want {
			t.Fatalf("register %d: got 0x%04X, want 0x%04X", 0x160+i, got, want)
		}
	}
}

// --- Quantity boundary tests ---

func TestReadCoilsMaxQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Read exactly MaxCoilCount (2000) — should succeed.
	sendRawRequest(t, clientConn, 20, 1, FuncReadCoils,
		makeReadCoilsPDU(0, MaxCoilCount))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("reading %d coils should succeed, got exception: %s", MaxCoilCount, resp.GetException())
	}
	byteCount := resp.GetPDU().Data[0]
	if byteCount != 250 { // ceil(2000/8) = 250
		t.Fatalf("byte count: got %d, want 250", byteCount)
	}
}

func TestReadCoilsExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus: MODBUS_MAX_READ_BITS+1 (2001) → exception
	sendRawRequest(t, clientConn, 21, 1, FuncReadCoils,
		makeReadCoilsPDU(0, MaxCoilCount+1))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for quantity > MaxCoilCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadDiscreteInputsExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 22, 1, FuncReadDiscreteInputs,
		makeReadCoilsPDU(0, MaxCoilCount+1))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for discrete inputs quantity > MaxCoilCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadHoldingRegistersMaxQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Read exactly MaxRegisterCount (125) — should succeed.
	sendRawRequest(t, clientConn, 23, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0, MaxRegisterCount))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("reading %d registers should succeed, got exception: %s", MaxRegisterCount, resp.GetException())
	}
	byteCount := resp.GetPDU().Data[0]
	if byteCount != 250 { // 125 * 2 = 250
		t.Fatalf("byte count: got %d, want 250", byteCount)
	}
}

func TestReadHoldingRegistersExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus: MODBUS_MAX_READ_REGISTERS+1 (126) → exception
	sendRawRequest(t, clientConn, 24, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0, MaxRegisterCount+1))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for quantity > MaxRegisterCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadInputRegistersExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 25, 1, FuncReadInputRegisters,
		makeReadRegistersPDU(0, MaxRegisterCount+1))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for input registers quantity > MaxRegisterCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestWriteMultipleCoilsExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus: MODBUS_MAX_WRITE_BITS+1 (1969) → exception
	coils := make([]bool, MaxWriteCoilCount+1)
	sendRawRequest(t, clientConn, 26, 1, FuncWriteMultipleCoils,
		makeWriteMultipleCoilsPDU(0, coils))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for writing > MaxWriteCoilCount coils")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestWriteMultipleRegistersExceedsMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus: MODBUS_MAX_WRITE_REGISTERS+1 (124) → exception
	regs := make([]RegisterValue, MaxWriteRegisterCount+1)
	sendRawRequest(t, clientConn, 27, 1, FuncWriteMultipleRegisters,
		makeWriteMultipleRegistersPDU(0, regs))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for writing > MaxWriteRegisterCount registers")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

// --- Zero quantity tests ---

func TestReadCoilsZeroQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 30, 1, FuncReadCoils,
		makeReadCoilsPDU(0, 0))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadDiscreteInputsZeroQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 31, 1, FuncReadDiscreteInputs,
		makeReadCoilsPDU(0, 0))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadInputRegistersZeroQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 32, 1, FuncReadInputRegisters,
		makeReadRegistersPDU(0, 0))

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestWriteMultipleCoilsZeroQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Craft a raw PDU with quantity=0 (can't use helper which derives from slice length).
	data := make([]byte, 6)
	binary.BigEndian.PutUint16(data[0:2], 0) // address
	binary.BigEndian.PutUint16(data[2:4], 0) // quantity = 0
	data[4] = 0                               // byte count
	sendRawRequest(t, clientConn, 33, 1, FuncWriteMultipleCoils, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for write multiple coils quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestWriteMultipleRegistersZeroQuantity(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	data := make([]byte, 5)
	binary.BigEndian.PutUint16(data[0:2], 0) // address
	binary.BigEndian.PutUint16(data[2:4], 0) // quantity = 0
	data[4] = 0                               // byte count
	sendRawRequest(t, clientConn, 34, 1, FuncWriteMultipleRegisters, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for write multiple registers quantity=0")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

// --- Invalid function code (FC 0x42 → exception) ---

func TestInvalidFunctionCode(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus tests FC 0x42 which should return illegal function exception.
	sendRawRequest(t, clientConn, 40, 1, FunctionCode(0x42), nil)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for invalid function code 0x42")
	}
	if resp.GetException() != ExceptionFunctionCodeNotSupported {
		t.Fatalf("exception: got %s, want FunctionCodeNotSupported", resp.GetException())
	}
	// Exception FC should be 0x42 | 0x80 = 0xC2.
	if byte(resp.GetPDU().FunctionCode) != 0xC2 {
		t.Fatalf("exception FC: got 0x%02X, want 0xC2", resp.GetPDU().FunctionCode)
	}
}

// --- Malformed PDU data tests ---

func TestReadCoilsShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Send only 2 bytes of PDU data where 4 are required.
	sendRawRequest(t, clientConn, 41, 1, FuncReadCoils, []byte{0x00, 0x00})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short PDU")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadHoldingRegistersShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 42, 1, FuncReadHoldingRegisters, []byte{0x00})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short PDU")
	}
}

func TestWriteSingleRegisterShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 43, 1, FuncWriteSingleRegister, []byte{0x00, 0x00})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short PDU")
	}
}

func TestWriteSingleCoilShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	sendRawRequest(t, clientConn, 44, 1, FuncWriteSingleCoil, []byte{0x00})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short PDU")
	}
}

func TestWriteMultipleCoilsByteCountMismatch(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Claim 8 coils (1 byte) but provide byteCount=2 with actual data.
	data := make([]byte, 7)
	binary.BigEndian.PutUint16(data[0:2], 0) // address
	binary.BigEndian.PutUint16(data[2:4], 8) // quantity = 8
	data[4] = 2                               // byte count = 2 (should be 1)
	data[5] = 0xFF
	data[6] = 0x00
	sendRawRequest(t, clientConn, 45, 1, FuncWriteMultipleCoils, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for byte count mismatch")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestWriteMultipleRegistersByteCountMismatch(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Claim 2 registers but provide byteCount=2 (should be 4).
	data := make([]byte, 7)
	binary.BigEndian.PutUint16(data[0:2], 0) // address
	binary.BigEndian.PutUint16(data[2:4], 2) // quantity = 2
	data[4] = 2                               // byte count = 2 (should be 4)
	data[5] = 0x12
	data[6] = 0x34
	sendRawRequest(t, clientConn, 46, 1, FuncWriteMultipleRegisters, data)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for byte count mismatch")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

// --- Read/Write Multiple Registers edge cases ---

func TestReadWriteMultipleRegistersShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// PDU requires at least 9 bytes; send only 6.
	sendRawRequest(t, clientConn, 47, 1, FuncReadWriteMultipleRegisters,
		[]byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short ReadWriteMultipleRegisters PDU")
	}
}

func TestReadWriteMultipleRegistersExceedsReadMax(t *testing.T) {
	_, clientConn, store := setupPipePair(t)
	store.SetHoldingRegister(0, 0x1111)

	// Read quantity = MaxReadWriteReadCount+1 (126), write quantity = 1
	pduData := make([]byte, 9+2)
	binary.BigEndian.PutUint16(pduData[0:2], 0)                           // read address
	binary.BigEndian.PutUint16(pduData[2:4], uint16(MaxReadWriteReadCount+1)) // read qty
	binary.BigEndian.PutUint16(pduData[4:6], 100)                         // write address
	binary.BigEndian.PutUint16(pduData[6:8], 1)                           // write qty
	pduData[8] = 2                                                         // byte count
	binary.BigEndian.PutUint16(pduData[9:11], 0xAAAA)

	sendRawRequest(t, clientConn, 48, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for read qty > MaxReadWriteReadCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadWriteMultipleRegistersExceedsWriteMax(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Write quantity = MaxReadWriteWriteCount+1 (122)
	writeQty := MaxReadWriteWriteCount + 1
	byteCount := int(writeQty) * 2
	pduData := make([]byte, 9+byteCount)
	binary.BigEndian.PutUint16(pduData[0:2], 0)                   // read address
	binary.BigEndian.PutUint16(pduData[2:4], 1)                   // read qty
	binary.BigEndian.PutUint16(pduData[4:6], 0)                   // write address
	binary.BigEndian.PutUint16(pduData[6:8], uint16(writeQty))    // write qty
	pduData[8] = byte(byteCount)

	sendRawRequest(t, clientConn, 49, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for write qty > MaxReadWriteWriteCount")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

// --- FC 0x2B/0x0E: Read Device Identification ---

func TestReadDeviceIdentificationBasic(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// MEI type 0x0E, Read Device ID code 0x01 (basic), starting object 0x00
	pduData := []byte{byte(MEIReadDeviceID), byte(ReadDeviceIDBasicStream), byte(DeviceIDVendorName)}
	sendRawRequest(t, clientConn, 50, 1, FuncReadDeviceIdentification, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	data := resp.GetPDU().Data
	// Verify MEI type echo.
	if data[0] != byte(MEIReadDeviceID) {
		t.Fatalf("MEI type: got 0x%02X, want 0x0E", data[0])
	}
	// Verify Read Device ID code echo.
	if data[1] != byte(ReadDeviceIDBasicStream) {
		t.Fatalf("Read Device ID code: got 0x%02X, want 0x01", data[1])
	}
	// Conformity level should be basic (0x01).
	if data[2] != byte(ConformityLevelBasic) {
		t.Fatalf("conformity level: got 0x%02X, want 0x01", data[2])
	}
	// More follows = no.
	if data[3] != byte(MoreFollowsNo) {
		t.Fatalf("more follows: got 0x%02X, want 0x00", data[3])
	}
	// 3 basic objects (VendorName, ProductCode, Revision).
	numObjects := data[5]
	if numObjects != 3 {
		t.Fatalf("number of objects: got %d, want 3", numObjects)
	}

	// Parse first object — VendorName.
	if data[6] != byte(DeviceIDVendorName) {
		t.Fatalf("first object ID: got 0x%02X, want 0x00", data[6])
	}
	objLen := int(data[7])
	vendorName := string(data[8 : 8+objLen])
	if vendorName != "goindustrial" {
		t.Fatalf("vendor name: got %q, want %q", vendorName, "goindustrial")
	}
}

func TestReadDeviceIdentificationSpecificObject(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Request a specific object: ProductName (0x04).
	pduData := []byte{byte(MEIReadDeviceID), byte(ReadDeviceIDSpecificObject), byte(DeviceIDProductName)}
	sendRawRequest(t, clientConn, 51, 1, FuncReadDeviceIdentification, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	data := resp.GetPDU().Data
	numObjects := data[5]
	if numObjects != 1 {
		t.Fatalf("number of objects: got %d, want 1", numObjects)
	}
	if data[6] != byte(DeviceIDProductName) {
		t.Fatalf("object ID: got 0x%02X, want 0x04", data[6])
	}
	objLen := int(data[7])
	productName := string(data[8 : 8+objLen])
	if productName != "goindustrial Modbus Server" {
		t.Fatalf("product name: got %q, want %q", productName, "goindustrial Modbus Server")
	}
}

func TestReadDeviceIdentificationRegularStream(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	pduData := []byte{byte(MEIReadDeviceID), byte(ReadDeviceIDRegularStream), byte(DeviceIDVendorName)}
	sendRawRequest(t, clientConn, 52, 1, FuncReadDeviceIdentification, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	data := resp.GetPDU().Data
	// Regular stream should include basic (3) + regular (4) = 7 objects.
	numObjects := data[5]
	if numObjects != 7 {
		t.Fatalf("number of objects: got %d, want 7", numObjects)
	}
}

func TestReadDeviceIdentificationInvalidMEI(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Use invalid MEI type (0x0D instead of 0x0E).
	pduData := []byte{0x0D, byte(ReadDeviceIDBasicStream), 0x00}
	sendRawRequest(t, clientConn, 53, 1, FuncReadDeviceIdentification, pduData)

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for invalid MEI type")
	}
	if resp.GetException() != ExceptionInvalidDataValue {
		t.Fatalf("exception: got %s, want InvalidDataValue", resp.GetException())
	}
}

func TestReadDeviceIdentificationShortPDU(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Send only 2 bytes (need 3: MEI type, read device ID code, object ID).
	sendRawRequest(t, clientConn, 54, 1, FuncReadDeviceIdentification, []byte{0x0E, 0x01})

	resp := readRawResponse(t, clientConn)
	if !resp.IsException() {
		t.Fatal("expected exception for short device ID PDU")
	}
}

// --- Write then read-back verification ---

func TestWriteAndReadBackCoils(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Write 37 coils matching libmodbus test vector.
	coilBytes := []byte{0xCD, 0x6B, 0xB2, 0x0E, 0x1B}
	coils := make([]bool, 37)
	for i := 0; i < 37; i++ {
		coils[i] = (coilBytes[i/8]>>uint(i%8))&1 == 1
	}

	sendRawRequest(t, clientConn, 60, 1, FuncWriteMultipleCoils,
		makeWriteMultipleCoilsPDU(0x130, coils))
	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("write exception: %s", resp.GetException())
	}

	// Read them back.
	sendRawRequest(t, clientConn, 61, 1, FuncReadCoils,
		makeReadCoilsPDU(0x130, 37))
	resp2 := readRawResponse(t, clientConn)
	if resp2.IsException() {
		t.Fatalf("read exception: %s", resp2.GetException())
	}

	pduData := resp2.GetPDU().Data
	for i, want := range coilBytes {
		if pduData[1+i] != want {
			t.Fatalf("read-back byte %d: got 0x%02X, want 0x%02X", i, pduData[1+i], want)
		}
	}
}

func TestWriteAndReadBackRegisters(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Write libmodbus test values.
	values := []RegisterValue{0x022B, 0x0001, 0x0064}
	sendRawRequest(t, clientConn, 62, 1, FuncWriteMultipleRegisters,
		makeWriteMultipleRegistersPDU(0x160, values))
	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("write exception: %s", resp.GetException())
	}

	// Read back.
	sendRawRequest(t, clientConn, 63, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0x160, 3))
	resp2 := readRawResponse(t, clientConn)
	if resp2.IsException() {
		t.Fatalf("read exception: %s", resp2.GetException())
	}

	pduData := resp2.GetPDU().Data
	for i, want := range values {
		got := binary.BigEndian.Uint16(pduData[1+i*2 : 3+i*2])
		if got != want {
			t.Fatalf("register %d: got 0x%04X, want 0x%04X", 0x160+i, got, want)
		}
	}
}

// --- Single coil invalid values ---

func TestWriteSingleCoilInvalidValues(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// libmodbus spec: only 0xFF00 (ON) and 0x0000 (OFF) are valid.
	invalidValues := []uint16{0x00FF, 0xFFFF, 0x0100, 0x7F00, 0x0001}
	for i, val := range invalidValues {
		data := make([]byte, 4)
		binary.BigEndian.PutUint16(data[0:2], 10)
		binary.BigEndian.PutUint16(data[2:4], val)
		sendRawRequest(t, clientConn, TransactionID(70+i), 1, FuncWriteSingleCoil, data)

		resp := readRawResponse(t, clientConn)
		if !resp.IsException() {
			t.Fatalf("expected exception for coil value 0x%04X", val)
		}
		if resp.GetException() != ExceptionInvalidDataValue {
			t.Fatalf("coil value 0x%04X: exception got %s, want InvalidDataValue", val, resp.GetException())
		}
	}
}

// --- Coil bit boundary edge cases ---

func TestReadCoilsBitBoundaries(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// Test exact byte boundaries: 1, 7, 8, 9, 16, 17 coils.
	for i := 0; i < 17; i++ {
		store.SetCoil(Address(i), true)
	}

	cases := []struct {
		quantity    Quantity
		wantBytes  byte
	}{
		{1, 1},
		{7, 1},
		{8, 1},
		{9, 2},
		{16, 2},
		{17, 3},
	}

	for i, tc := range cases {
		sendRawRequest(t, clientConn, TransactionID(80+i), 1, FuncReadCoils,
			makeReadCoilsPDU(0, tc.quantity))

		resp := readRawResponse(t, clientConn)
		if resp.IsException() {
			t.Fatalf("qty=%d: unexpected exception: %s", tc.quantity, resp.GetException())
		}

		byteCount := resp.GetPDU().Data[0]
		if byteCount != tc.wantBytes {
			t.Fatalf("qty=%d: byte count got %d, want %d", tc.quantity, byteCount, tc.wantBytes)
		}
	}
}

// --- Multiple sequential operations ---

func TestSequentialWriteReadOperations(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Write a series of single registers, then read them all back.
	for i := 0; i < 10; i++ {
		sendRawRequest(t, clientConn, TransactionID(90+i), 1, FuncWriteSingleRegister,
			makeWriteSingleRegisterPDU(Address(i), RegisterValue(i*1000)))
		resp := readRawResponse(t, clientConn)
		if resp.IsException() {
			t.Fatalf("write %d: unexpected exception: %s", i, resp.GetException())
		}
	}

	// Read all 10 back in one request.
	sendRawRequest(t, clientConn, 100, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(0, 10))
	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("read-back exception: %s", resp.GetException())
	}

	pduData := resp.GetPDU().Data
	for i := 0; i < 10; i++ {
		got := binary.BigEndian.Uint16(pduData[1+i*2 : 3+i*2])
		want := uint16(i * 1000)
		if got != want {
			t.Fatalf("register %d: got %d, want %d", i, got, want)
		}
	}
}

// --- DataStore: all four read paths via protocol ---

func TestMemoryStoreDiscreteInputsQuantityValidation(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Quantity 0 for discrete inputs.
	_, err := store.ReadDiscreteInputs(ctx, 0, 0)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for discrete inputs qty=0, got %v", err)
	}

	_, err = store.ReadDiscreteInputs(ctx, 0, MaxCoilCount+1)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for discrete inputs qty=%d, got %v", MaxCoilCount+1, err)
	}

	// Quantity 0 for input registers.
	_, err = store.ReadInputRegisters(ctx, 0, 0)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for input registers qty=0, got %v", err)
	}

	_, err = store.ReadInputRegisters(ctx, 0, MaxRegisterCount+1)
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for input registers qty=%d, got %v", MaxRegisterCount+1, err)
	}
}

func TestMemoryStoreWriteMultipleQuantityValidation(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Empty coils.
	err := store.WriteMultipleCoils(ctx, 0, []CoilValue{})
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for empty coils, got %v", err)
	}

	// Exceeding max write coils.
	err = store.WriteMultipleCoils(ctx, 0, make([]CoilValue, MaxWriteCoilCount+1))
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for coils > max, got %v", err)
	}

	// Empty registers.
	err = store.WriteMultipleRegisters(ctx, 0, []RegisterValue{})
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for empty registers, got %v", err)
	}

	// Exceeding max write registers.
	err = store.WriteMultipleRegisters(ctx, 0, make([]RegisterValue, MaxWriteRegisterCount+1))
	if err != ErrInvalidQuantity {
		t.Fatalf("expected ErrInvalidQuantity for registers > max, got %v", err)
	}
}

// --- Error helpers and type stringers ---

func TestModbusErrorHelpers(t *testing.T) {
	err := NewModbusError(FuncReadCoils, ExceptionServerDeviceBusy)

	if !IsModbusError(err) {
		t.Fatal("IsModbusError should return true")
	}
	if !IsExceptionError(err, ExceptionServerDeviceBusy) {
		t.Fatal("IsExceptionError should match ServerDeviceBusy")
	}
	if IsExceptionError(err, ExceptionInvalidDataValue) {
		t.Fatal("IsExceptionError should not match InvalidDataValue")
	}
	if IsFunctionNotSupportedError(err) {
		t.Fatal("IsFunctionNotSupportedError should be false")
	}

	fnErr := NewModbusError(FuncReadCoils, ExceptionFunctionCodeNotSupported)
	if !IsFunctionNotSupportedError(fnErr) {
		t.Fatal("IsFunctionNotSupportedError should be true")
	}

	// Test error string contains meaningful info and a numeric exception code,
	// not a hex-encoded string name (e.g. "0x06" not "0x53657276657244657669636542757379").
	errStr := err.Error()
	if !strings.Contains(errStr, "0x6") {
		t.Fatalf("error string should contain numeric exception code 0x6, got: %s", errStr)
	}
}

func TestExceptionCodeStrings(t *testing.T) {
	codes := []struct {
		code ExceptionCode
		want string
	}{
		{ExceptionFunctionCodeNotSupported, "FunctionCodeNotSupported"},
		{ExceptionDataAddressNotAvailable, "DataAddressNotAvailable"},
		{ExceptionInvalidDataValue, "InvalidDataValue"},
		{ExceptionServerDeviceFailure, "ServerDeviceFailure"},
		{ExceptionAcknowledge, "Acknowledge"},
		{ExceptionServerDeviceBusy, "ServerDeviceBusy"},
		{ExceptionMemoryParityError, "MemoryParityError"},
		{ExceptionGatewayPathUnavailable, "GatewayPathUnavailable"},
		{ExceptionGatewayTargetNoResponse, "GatewayTargetNoResponse"},
	}
	for _, tc := range codes {
		got := tc.code.String()
		if got != tc.want {
			t.Fatalf("ExceptionCode(%d).String(): got %q, want %q", tc.code, got, tc.want)
		}
	}

	// Unknown code.
	unknown := ExceptionCode(0xFF).String()
	if unknown == "" {
		t.Fatal("unknown exception code should have a string representation")
	}
}

func TestFunctionCodeStrings(t *testing.T) {
	codes := []struct {
		fc   FunctionCode
		want string
	}{
		{FuncReadCoils, "ReadCoils"},
		{FuncReadDiscreteInputs, "ReadDiscreteInputs"},
		{FuncReadHoldingRegisters, "ReadHoldingRegisters"},
		{FuncReadInputRegisters, "ReadInputRegisters"},
		{FuncWriteSingleCoil, "WriteSingleCoil"},
		{FuncWriteSingleRegister, "WriteSingleRegister"},
		{FuncReadExceptionStatus, "ReadExceptionStatus"},
		{FuncWriteMultipleCoils, "WriteMultipleCoils"},
		{FuncWriteMultipleRegisters, "WriteMultipleRegisters"},
		{FuncReadWriteMultipleRegisters, "ReadWriteMultipleRegisters"},
		{FuncReadDeviceIdentification, "ReadDeviceIdentification"},
	}
	for _, tc := range codes {
		got := tc.fc.String()
		if got != tc.want {
			t.Fatalf("FunctionCode(0x%02X).String(): got %q, want %q", tc.fc, got, tc.want)
		}
	}
}

func TestIsExceptionHelpers(t *testing.T) {
	// Regular function codes should not be exceptions.
	if IsException(0x03) {
		t.Fatal("0x03 should not be an exception")
	}
	// Exception function codes have high bit set.
	if !IsException(0x83) {
		t.Fatal("0x83 should be an exception")
	}
	// Extract original function code.
	if GetOriginalFunctionCode(0x83) != 0x03 {
		t.Fatal("original function code of 0x83 should be 0x03")
	}

	if !IsFunctionException(FunctionCode(0x81)) {
		t.Fatal("FunctionCode 0x81 should be an exception")
	}
	if IsFunctionException(FuncReadCoils) {
		t.Fatal("FuncReadCoils should not be an exception")
	}
	if GetOriginalFunction(FunctionCode(0x81)) != FuncReadCoils {
		t.Fatal("original function of 0x81 should be FuncReadCoils")
	}
}

// --- Request/Response decode edge cases ---

func TestRequestDecodeShortData(t *testing.T) {
	// Less than TCPHeaderLength (7 bytes).
	req := &Request{}
	err := req.Decode([]byte{0x00, 0x01, 0x00, 0x00})
	if err != ErrInvalidResponseLength {
		t.Fatalf("expected ErrInvalidResponseLength, got %v", err)
	}
}

func TestResponseDecodeShortData(t *testing.T) {
	resp := &Response{}
	err := resp.Decode([]byte{0x00, 0x01, 0x00})
	if err != ErrInvalidResponseLength {
		t.Fatalf("expected ErrInvalidResponseLength, got %v", err)
	}
}

func TestResponseDecodeNegativeDataLength(t *testing.T) {
	// Craft a response where length field = 1 (just unit ID, no function code).
	// This means pduDataLength = 1 - 2 = -1.
	data := make([]byte, 8)
	binary.BigEndian.PutUint16(data[0:2], 1)    // transaction ID
	binary.BigEndian.PutUint16(data[2:4], 0)    // protocol ID
	binary.BigEndian.PutUint16(data[4:6], 1)    // length = 1 (too small)
	data[6] = 1                                  // unit ID
	data[7] = 0x03                               // function code

	resp := &Response{}
	err := resp.Decode(data)
	if err != ErrInvalidResponseLength {
		t.Fatalf("expected ErrInvalidResponseLength for length=1, got %v", err)
	}
}

// --- Protocol constant verification ---

func TestProtocolConstants(t *testing.T) {
	// Verify constants match the Modbus specification and libmodbus values.
	if MaxCoilCount != 2000 {
		t.Fatalf("MaxCoilCount: got %d, want 2000", MaxCoilCount)
	}
	if MaxWriteCoilCount != 1968 {
		t.Fatalf("MaxWriteCoilCount: got %d, want 1968", MaxWriteCoilCount)
	}
	if MaxRegisterCount != 125 {
		t.Fatalf("MaxRegisterCount: got %d, want 125", MaxRegisterCount)
	}
	if MaxWriteRegisterCount != 123 {
		t.Fatalf("MaxWriteRegisterCount: got %d, want 123", MaxWriteRegisterCount)
	}
	if MaxReadWriteReadCount != 125 {
		t.Fatalf("MaxReadWriteReadCount: got %d, want 125", MaxReadWriteReadCount)
	}
	if MaxReadWriteWriteCount != 121 {
		t.Fatalf("MaxReadWriteWriteCount: got %d, want 121", MaxReadWriteWriteCount)
	}
	if MaxPDULength != 253 {
		t.Fatalf("MaxPDULength: got %d, want 253", MaxPDULength)
	}
	if MaxADULength != 260 {
		t.Fatalf("MaxADULength: got %d, want 260", MaxADULength)
	}
	if TCPHeaderLength != 7 {
		t.Fatalf("TCPHeaderLength: got %d, want 7", TCPHeaderLength)
	}
	if DefaultTCPPort != 502 {
		t.Fatalf("DefaultTCPPort: got %d, want 502", DefaultTCPPort)
	}
	if CoilOnU16 != 0xFF00 {
		t.Fatalf("CoilOnU16: got 0x%04X, want 0xFF00", CoilOnU16)
	}
	if CoilOffU16 != 0x0000 {
		t.Fatalf("CoilOffU16: got 0x%04X, want 0x0000", CoilOffU16)
	}
}

// --- Transaction ID preservation across operations ---

func TestTransactionIDPreservation(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Test boundary transaction IDs: 0, 1, 0x7FFF, 0xFFFF.
	txIDs := []TransactionID{0, 1, 0x7FFF, 0xFFFF}
	for _, txID := range txIDs {
		sendRawRequest(t, clientConn, txID, 1, FuncReadHoldingRegisters,
			makeReadRegistersPDU(0, 1))

		resp := readRawResponse(t, clientConn)
		if resp.GetTransactionID() != txID {
			t.Fatalf("txID mismatch: sent %d, got %d", txID, resp.GetTransactionID())
		}
	}
}

// --- Custom handler registration ---

func TestCustomHandlerRegistration(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	store := NewMemoryStore()
	srv := NewServer("test",
		WithServerDataStore(store),
		WithServerConn(serverConn),
	)

	// Override ReadCoils handler with a custom one that always returns exception.
	srv.SetHandler(FuncReadCoils, func(ctx context.Context, req *Request) (*Response, error) {
		return nil, NewModbusError(FuncReadCoils, ExceptionServerDeviceBusy)
	})

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() {
		clientConn.Close()
		srv.Stop(context.Background())
	})

	sendRawRequest(t, clientConn, 1, 1, FuncReadCoils, makeReadCoilsPDU(0, 1))
	resp := readRawResponse(t, clientConn)

	if !resp.IsException() {
		t.Fatal("expected exception from custom handler")
	}
	if resp.GetException() != ExceptionServerDeviceBusy {
		t.Fatalf("exception: got %s, want ServerDeviceBusy", resp.GetException())
	}
}

// --- ReadWriteMultipleRegisters: write-before-read semantics (Modbus spec) ---

func TestReadWriteMultipleRegistersWriteBeforeRead(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Write to address 0, then read from address 0 in the same request.
	// Since write happens before read, the read should see the written value.
	pduData := make([]byte, 9+2)
	binary.BigEndian.PutUint16(pduData[0:2], 0)      // read address = 0
	binary.BigEndian.PutUint16(pduData[2:4], 1)      // read quantity = 1
	binary.BigEndian.PutUint16(pduData[4:6], 0)      // write address = 0
	binary.BigEndian.PutUint16(pduData[6:8], 1)      // write quantity = 1
	pduData[8] = 2                                     // byte count
	binary.BigEndian.PutUint16(pduData[9:11], 0xBEEF) // write value

	sendRawRequest(t, clientConn, 110, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	// The read result should contain the value we just wrote.
	respData := resp.GetPDU().Data
	val := binary.BigEndian.Uint16(respData[1:3])
	if val != 0xBEEF {
		t.Fatalf("read should see written value: got 0x%04X, want 0xBEEF", val)
	}
}

// --- Uninitialized address reads (sparse store behavior) ---

func TestReadUninitializedAddresses(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// Read coils from addresses that were never written.
	// Should return all false/zero (sparse map default).
	sendRawRequest(t, clientConn, 120, 1, FuncReadCoils,
		makeReadCoilsPDU(5000, 16))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	pduData := resp.GetPDU().Data
	if pduData[1] != 0x00 || pduData[2] != 0x00 {
		t.Fatalf("uninitialized coils should be 0: got 0x%02X 0x%02X", pduData[1], pduData[2])
	}

	// Read registers from addresses that were never written.
	sendRawRequest(t, clientConn, 121, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(5000, 3))

	resp2 := readRawResponse(t, clientConn)
	if resp2.IsException() {
		t.Fatalf("unexpected exception: %s", resp2.GetException())
	}
	pduData2 := resp2.GetPDU().Data
	for i := 0; i < 3; i++ {
		val := binary.BigEndian.Uint16(pduData2[1+i*2 : 3+i*2])
		if val != 0 {
			t.Fatalf("uninitialized register %d should be 0, got %d", i, val)
		}
	}
}

// ===========================================================================
// Tests mined from pymodbus (https://github.com/pymodbus-dev/pymodbus)
// Authoritative Python Modbus library — PDU wire format and protocol tests
// ===========================================================================

// --- PDU wire-format verification ---

func TestPDUWireFormat(t *testing.T) {
	// Verify our server produces responses matching pymodbus expected byte patterns.
	// pymodbus test_pdu.py and test_decoders.py vectors.

	_, clientConn, store := setupPipePair(t)

	// pymodbus: ReadCoilsRequest(address=117, count=3)
	// PDU bytes: b'\x01\x00\x75\x00\x03' (FC + addr_hi + addr_lo + qty_hi + qty_lo)
	store.SetCoil(117, true)
	store.SetCoil(118, true)
	store.SetCoil(119, false)

	sendRawRequest(t, clientConn, 1, 1, FuncReadCoils, makeReadCoilsPDU(117, 3))
	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	// pymodbus: ReadCoilsResponse bits=[True,True,False] → byte=0x03
	pdu := resp.GetPDU().Data
	if pdu[0] != 1 { // byte count
		t.Fatalf("byte count: got %d, want 1", pdu[0])
	}
	if pdu[1] != 0x03 { // coils 117=1, 118=1, 119=0 → 0b00000011 = 0x03
		t.Fatalf("coil byte: got 0x%02X, want 0x03", pdu[1])
	}
}

func TestPDUReadHoldingRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: ReadHoldingRegistersResponse(registers=[3, 17])
	// PDU bytes: b'\x03\x04\x00\x03\x00\x11'
	store.SetHoldingRegister(0, 3)
	store.SetHoldingRegister(1, 17)

	sendRawRequest(t, clientConn, 2, 1, FuncReadHoldingRegisters, makeReadRegistersPDU(0, 2))
	resp := readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data

	if pdu[0] != 4 { // byte count = 2 registers * 2
		t.Fatalf("byte count: got %d, want 4", pdu[0])
	}
	r0 := binary.BigEndian.Uint16(pdu[1:3])
	r1 := binary.BigEndian.Uint16(pdu[3:5])
	if r0 != 3 || r1 != 17 {
		t.Fatalf("registers: got [%d, %d], want [3, 17]", r0, r1)
	}
}

func TestPDUReadInputRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: ReadInputRegistersResponse(registers=[3, 17])
	// PDU bytes: b'\x04\x04\x00\x03\x00\x11'
	store.SetInputRegister(0, 3)
	store.SetInputRegister(1, 17)

	sendRawRequest(t, clientConn, 3, 1, FuncReadInputRegisters, makeReadRegistersPDU(0, 2))
	resp := readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data

	if pdu[0] != 4 {
		t.Fatalf("byte count: got %d, want 4", pdu[0])
	}
	r0 := binary.BigEndian.Uint16(pdu[1:3])
	r1 := binary.BigEndian.Uint16(pdu[3:5])
	if r0 != 3 || r1 != 17 {
		t.Fatalf("input registers: got [%d, %d], want [3, 17]", r0, r1)
	}
}

func TestPDUWriteSingleRegister(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: WriteSingleRegisterRequest(address=1, registers=[0xABCD])
	// PDU bytes: b'\x06\x00\x01\xab\xcd'
	sendRawRequest(t, clientConn, 4, 1, FuncWriteSingleRegister,
		makeWriteSingleRegisterPDU(1, 0xABCD))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	// Echo response should match
	pdu := resp.GetPDU().Data
	addr := binary.BigEndian.Uint16(pdu[0:2])
	val := binary.BigEndian.Uint16(pdu[2:4])
	if addr != 1 || val != 0xABCD {
		t.Fatalf("echo: addr=%d val=0x%04X, want 1/0xABCD", addr, val)
	}
	// Verify in store
	v, ok := store.GetHoldingRegister(1)
	if !ok || v != 0xABCD {
		t.Fatalf("store: got %v/0x%04X, want true/0xABCD", ok, v)
	}
}

func TestPDUWriteMultipleRegisters(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: WriteMultipleRegistersRequest(address=117, registers=[111, 121, 131])
	// PDU bytes: b'\x10\x00\x75\x00\x03\x06\x00\x6f\x00\x79\x00\x83'
	values := []RegisterValue{111, 121, 131}
	sendRawRequest(t, clientConn, 5, 1, FuncWriteMultipleRegisters,
		makeWriteMultipleRegistersPDU(117, values))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	// Response: address echo + quantity echo
	pdu := resp.GetPDU().Data
	respAddr := binary.BigEndian.Uint16(pdu[0:2])
	respQty := binary.BigEndian.Uint16(pdu[2:4])
	if respAddr != 117 || respQty != 3 {
		t.Fatalf("response: addr=%d qty=%d, want 117/3", respAddr, respQty)
	}
	// Verify store
	for i, want := range values {
		v, _ := store.GetHoldingRegister(Address(117 + i))
		if v != want {
			t.Fatalf("store[%d]: got %d, want %d", 117+i, v, want)
		}
	}
}

func TestPDUWriteSingleCoil(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: WriteSingleCoilRequest(address=117, bits=[True])
	// PDU bytes: b'\x05\x00\x75\xff\x00'
	sendRawRequest(t, clientConn, 6, 1, FuncWriteSingleCoil,
		makeWriteSingleCoilPDU(117, true))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	v, ok := store.GetCoil(117)
	if !ok || !v {
		t.Fatalf("coil 117: got %v/%v, want true/true", ok, v)
	}
}

func TestPDUWriteMultipleCoils(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: WriteMultipleCoilsRequest(address=1, bits=[True]*5)
	// PDU bytes: b'\x0f\x00\x01\x00\x05\x01\x1f'
	// 5 coils all true → byte=0x1F (0b00011111)
	coils := []bool{true, true, true, true, true}
	sendRawRequest(t, clientConn, 7, 1, FuncWriteMultipleCoils,
		makeWriteMultipleCoilsPDU(1, coils))

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}
	for i := 0; i < 5; i++ {
		v, ok := store.GetCoil(Address(1 + i))
		if !ok || !v {
			t.Fatalf("coil %d should be true", 1+i)
		}
	}
}

// --- ReadWriteMultipleRegisters with specific addresses ---

func TestReadWriteMultipleRegistersAddressed(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: ReadWriteMultipleRegistersRequest(
	//   read_address=17, read_count=2,
	//   write_address=25, write_registers=[111, 112])
	// PDU bytes: b'\x17\x00\x11\x00\x02\x00\x19\x00\x02\x04\x00\x6f\x00\x70'
	store.SetHoldingRegister(17, 0xAAAA)
	store.SetHoldingRegister(18, 0xBBBB)

	pduData := make([]byte, 9+4)
	binary.BigEndian.PutUint16(pduData[0:2], 17)    // read address
	binary.BigEndian.PutUint16(pduData[2:4], 2)     // read quantity
	binary.BigEndian.PutUint16(pduData[4:6], 25)    // write address
	binary.BigEndian.PutUint16(pduData[6:8], 2)     // write quantity
	pduData[8] = 4                                    // byte count
	binary.BigEndian.PutUint16(pduData[9:11], 111)   // write value 1
	binary.BigEndian.PutUint16(pduData[11:13], 112)  // write value 2

	sendRawRequest(t, clientConn, 8, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	respData := resp.GetPDU().Data
	if respData[0] != 4 { // byte count = 2 registers * 2
		t.Fatalf("byte count: got %d, want 4", respData[0])
	}
	r0 := binary.BigEndian.Uint16(respData[1:3])
	r1 := binary.BigEndian.Uint16(respData[3:5])
	if r0 != 0xAAAA || r1 != 0xBBBB {
		t.Fatalf("read values: got [0x%04X, 0x%04X], want [0xAAAA, 0xBBBB]", r0, r1)
	}

	// Verify writes happened
	w0, _ := store.GetHoldingRegister(25)
	w1, _ := store.GetHoldingRegister(26)
	if w0 != 111 || w1 != 112 {
		t.Fatalf("written values: got [%d, %d], want [111, 112]", w0, w1)
	}
}

// --- Exception response wire format for all exception codes ---

func TestAllExceptionCodesWireFormat(t *testing.T) {
	_, clientConn, _ := setupPipePair(t)

	// pymodbus tests exception responses: FC|0x80 followed by exception code byte.
	// Test each standard function code produces correct exception format.
	funcCodes := []FunctionCode{
		FuncReadCoils,             // 0x01 → exception 0x81
		FuncReadDiscreteInputs,    // 0x02 → exception 0x82
		FuncReadHoldingRegisters,  // 0x03 → exception 0x83
		FuncReadInputRegisters,    // 0x04 → exception 0x84
	}

	for _, fc := range funcCodes {
		// Send with quantity=0 to trigger InvalidDataValue exception
		sendRawRequest(t, clientConn, TransactionID(200+fc), 1, fc,
			makeReadRegistersPDU(0, 0))

		resp := readRawResponse(t, clientConn)
		if !resp.IsException() {
			t.Fatalf("FC 0x%02X: expected exception for qty=0", fc)
		}
		// Exception FC should be original | 0x80
		wantFC := FunctionCode(byte(fc) | ExceptionBit)
		if resp.GetPDU().FunctionCode != wantFC {
			t.Fatalf("FC 0x%02X: exception FC got 0x%02X, want 0x%02X",
				fc, resp.GetPDU().FunctionCode, wantFC)
		}
		if resp.GetException() != ExceptionInvalidDataValue {
			t.Fatalf("FC 0x%02X: exception code got %s, want InvalidDataValue",
				fc, resp.GetException())
		}
	}
}

// --- Full MBAP frame encode/decode ---

func TestMBAPFrameEncode(t *testing.T) {
	// pymodbus framer test: ReadHoldingRegistersRequest(address=0x7C, count=2)
	// Full MBAP frame: b'\x00\x00\x00\x00\x00\x06\x00\x03\x00\x7c\x00\x02'
	// TID=0, Protocol=0, Length=6, Unit=0, FC=0x03, Addr=0x007C, Qty=0x0002
	req := NewRequest(UnitID(0), FuncReadHoldingRegisters, makeReadRegistersPDU(0x7C, 2))
	req.SetTransactionID(0)

	encoded, err := req.Encode()
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte{
		0x00, 0x00, // Transaction ID = 0
		0x00, 0x00, // Protocol ID = 0
		0x00, 0x06, // Length = 6 (unit + fc + 4 bytes pdu)
		0x00,       // Unit ID = 0
		0x03,       // Function Code = Read Holding Registers
		0x00, 0x7C, // Address = 124
		0x00, 0x02, // Quantity = 2
	}
	if len(encoded) != len(expected) {
		t.Fatalf("encoded length: got %d, want %d", len(encoded), len(expected))
	}
	for i, want := range expected {
		if encoded[i] != want {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X", i, encoded[i], want)
		}
	}
}

func TestMBAPFrameWithTID(t *testing.T) {
	// pymodbus: TID=3077 (0x0C05), Unit=0, FC=0x03, Addr=0x7C, Qty=2
	// Frame: b'\x0c\x05\x00\x00\x00\x06\x00\x03\x00\x7c\x00\x02'
	req := NewRequest(UnitID(0), FuncReadHoldingRegisters, makeReadRegistersPDU(0x7C, 2))
	req.SetTransactionID(3077)

	encoded, err := req.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// Verify TID bytes
	tid := binary.BigEndian.Uint16(encoded[0:2])
	if tid != 3077 {
		t.Fatalf("TID: got %d, want 3077", tid)
	}
}

func TestMBAPResponseFrame(t *testing.T) {
	// pymodbus response: TID=3077, Unit=0x11, FC=0x03, data=[0x8D, 0x8E]
	// Frame: b'\x0c\x05\x00\x00\x00\x07\x11\x03\x04\x00\x8d\x00\x8e'
	raw := []byte{
		0x0C, 0x05, // TID = 3077
		0x00, 0x00, // Protocol ID = 0
		0x00, 0x07, // Length = 7
		0x11,       // Unit ID = 17
		0x03,       // FC = ReadHoldingRegisters
		0x04,       // Byte count = 4
		0x00, 0x8D, // Register 0 = 141
		0x00, 0x8E, // Register 1 = 142
	}

	resp := &Response{}
	if err := resp.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if resp.GetTransactionID() != 3077 {
		t.Fatalf("TID: got %d, want 3077", resp.GetTransactionID())
	}
	if resp.GetUnitID() != 17 {
		t.Fatalf("UnitID: got %d, want 17", resp.GetUnitID())
	}
	if resp.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Fatalf("FC: got 0x%02X, want 0x03", resp.GetPDU().FunctionCode)
	}
	pdu := resp.GetPDU().Data
	if pdu[0] != 4 {
		t.Fatalf("byte count: got %d, want 4", pdu[0])
	}
	r0 := binary.BigEndian.Uint16(pdu[1:3])
	r1 := binary.BigEndian.Uint16(pdu[3:5])
	if r0 != 0x8D || r1 != 0x8E {
		t.Fatalf("registers: got [0x%04X, 0x%04X], want [0x008D, 0x008E]", r0, r1)
	}
}

func TestMBAPExceptionResponse(t *testing.T) {
	// pymodbus: Exception response for FC 0x01 with Illegal Function (0x01)
	// Wire: TID(2) + Proto(2) + Len(2) + Unit(1) + FC|0x80(1) + ExCode(1)
	raw := []byte{
		0x00, 0x01, // TID = 1
		0x00, 0x00, // Protocol ID
		0x00, 0x03, // Length = 3 (unit + FC + exception)
		0x01,       // Unit ID
		0x81,       // FC = 0x01 | 0x80
		0x01,       // Exception code = Illegal Function
	}

	resp := &Response{}
	if err := resp.Decode(raw); err != nil {
		t.Fatal(err)
	}
	if !resp.IsException() {
		t.Fatal("expected exception response")
	}
	if resp.GetException() != ExceptionFunctionCodeNotSupported {
		t.Fatalf("exception: got %s, want FunctionCodeNotSupported", resp.GetException())
	}
	origFC := GetOriginalFunctionCode(byte(resp.GetPDU().FunctionCode))
	if FunctionCode(origFC) != FuncReadCoils {
		t.Fatalf("original FC: got 0x%02X, want 0x01", origFC)
	}
}

// --- Register value test patterns ---

func TestRegisterValuesPattern0x0A0B0C(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus TEST_MESSAGE: b"\x06\x00\x0a\x00\x0b\x00\x0c"
	// Registers [0x0A, 0x0B, 0x0C] at address 1
	store.SetHoldingRegister(1, 0x0A)
	store.SetHoldingRegister(2, 0x0B)
	store.SetHoldingRegister(3, 0x0C)

	sendRawRequest(t, clientConn, 10, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(1, 3))

	resp := readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data

	// Verify byte-for-byte against pymodbus expected: 0x06 0x00 0x0A 0x00 0x0B 0x00 0x0C
	if pdu[0] != 0x06 {
		t.Fatalf("byte count: got 0x%02X, want 0x06", pdu[0])
	}
	expectedBytes := []byte{0x00, 0x0A, 0x00, 0x0B, 0x00, 0x0C}
	for i, want := range expectedBytes {
		if pdu[1+i] != want {
			t.Fatalf("pdu[%d]: got 0x%02X, want 0x%02X", 1+i, pdu[1+i], want)
		}
	}
}

// --- Bit packing vectors ---

func TestBitPackingAllTrue(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: 5 coils all true at address 1
	// Write then read, verify packed byte = 0x1F (0b00011111)
	for i := 0; i < 5; i++ {
		store.SetCoil(Address(1+i), true)
	}

	sendRawRequest(t, clientConn, 11, 1, FuncReadCoils, makeReadCoilsPDU(1, 5))
	resp := readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data

	if pdu[0] != 1 { // ceil(5/8) = 1
		t.Fatalf("byte count: got %d, want 1", pdu[0])
	}
	if pdu[1] != 0x1F { // 0b00011111
		t.Fatalf("packed bits: got 0x%02X, want 0x1F", pdu[1])
	}
}

func TestBitPackingMixed(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus bit pattern from test_pdu.py:
	// b"\x05\x81" → bits: [True,False,True] + [False]*5 + [True] + [False]*6 + [True]
	// This is a 16-bit read: coils packed as 0x05 (byte 0) and 0x81 (byte 1)
	// Byte 0: 0x05 = 00000101 → bit0=1, bit1=0, bit2=1, bit3-7=0
	// Byte 1: 0x81 = 10000001 → bit8=1, bit9-14=0, bit15=1
	bits := []bool{
		true, false, true, false, false, false, false, false, // 0x05
		true, false, false, false, false, false, false, true,  // 0x81
	}
	for i, v := range bits {
		store.SetCoil(Address(i), v)
	}

	sendRawRequest(t, clientConn, 12, 1, FuncReadCoils, makeReadCoilsPDU(0, 16))
	resp := readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data

	if pdu[0] != 2 {
		t.Fatalf("byte count: got %d, want 2", pdu[0])
	}
	if pdu[1] != 0x05 {
		t.Fatalf("byte 0: got 0x%02X, want 0x05", pdu[1])
	}
	if pdu[2] != 0x81 {
		t.Fatalf("byte 1: got 0x%02X, want 0x81", pdu[2])
	}
}

// --- ReadWriteMultipleRegisters response verification ---

func TestReadWriteMultipleRegistersResponse(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus: ReadWriteMultipleRegistersResponse(registers=[1, 2])
	// PDU bytes: b'\x17\x04\x00\x01\x00\x02'
	store.SetHoldingRegister(0, 1)
	store.SetHoldingRegister(1, 2)

	pduData := make([]byte, 9+2)
	binary.BigEndian.PutUint16(pduData[0:2], 0)   // read address
	binary.BigEndian.PutUint16(pduData[2:4], 2)   // read quantity
	binary.BigEndian.PutUint16(pduData[4:6], 100) // write address
	binary.BigEndian.PutUint16(pduData[6:8], 1)   // write quantity
	pduData[8] = 2                                  // byte count
	binary.BigEndian.PutUint16(pduData[9:11], 0x1234)

	sendRawRequest(t, clientConn, 13, 1, FuncReadWriteMultipleRegisters, pduData)

	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("unexpected exception: %s", resp.GetException())
	}

	// Response: byte_count + register data
	respData := resp.GetPDU().Data
	if respData[0] != 4 { // 2 registers * 2 bytes
		t.Fatalf("byte count: got %d, want 4", respData[0])
	}
	r0 := binary.BigEndian.Uint16(respData[1:3])
	r1 := binary.BigEndian.Uint16(respData[3:5])
	if r0 != 1 || r1 != 2 {
		t.Fatalf("registers: got [%d, %d], want [1, 2]", r0, r1)
	}
}

// --- Address 117 pattern (standard test address) ---

func TestAddress117Pattern(t *testing.T) {
	_, clientConn, store := setupPipePair(t)

	// pymodbus uses address=117 (0x0075) as a standard test address.
	// Verify all basic operations at this address.

	// Write single register at 117 with value 0x0070
	sendRawRequest(t, clientConn, 20, 1, FuncWriteSingleRegister,
		makeWriteSingleRegisterPDU(117, 0x0070))
	resp := readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("write single reg: %s", resp.GetException())
	}

	// Read it back
	sendRawRequest(t, clientConn, 21, 1, FuncReadHoldingRegisters,
		makeReadRegistersPDU(117, 1))
	resp = readRawResponse(t, clientConn)
	pdu := resp.GetPDU().Data
	val := binary.BigEndian.Uint16(pdu[1:3])
	if val != 0x0070 {
		t.Fatalf("register 117: got 0x%04X, want 0x0070", val)
	}

	// Write single coil ON at 117
	sendRawRequest(t, clientConn, 22, 1, FuncWriteSingleCoil,
		makeWriteSingleCoilPDU(117, true))
	resp = readRawResponse(t, clientConn)
	if resp.IsException() {
		t.Fatalf("write single coil: %s", resp.GetException())
	}

	// Read discrete input at 117 (should be false/unset since we wrote coil, not discrete)
	store.SetDiscreteInput(117, true)
	sendRawRequest(t, clientConn, 23, 1, FuncReadDiscreteInputs,
		makeReadCoilsPDU(117, 1))
	resp = readRawResponse(t, clientConn)
	pdu = resp.GetPDU().Data
	if pdu[1] != 0x01 {
		t.Fatalf("discrete input 117: got 0x%02X, want 0x01", pdu[1])
	}
}

// ===========================================================================
// Cancellation and shutdown tests
// ===========================================================================

// TestClientContextCanceledDuringRead tests that a canceled context during a
// read operation returns promptly with a context error. Uses net.Pipe so no
// data is ever written on the server side, causing the client to block until
// the context is canceled.
func TestClientContextCanceledDuringRead(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	conn := NewTCPConn("test", WithConn(clientConn))
	if err := conn.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := conn.Send(ctx, NewRequest(0, FuncReadHoldingRegisters, makeReadRegistersPDU(0, 1)))
	if err == nil {
		t.Fatal("expected error from canceled context")
	}

	// The error should be a context deadline exceeded or context canceled.
	if err != context.DeadlineExceeded && err != context.Canceled {
		// Also accept wrapped errors.
		if ctx.Err() == nil {
			t.Fatalf("expected context error, got: %v", err)
		}
	}
}

// TestTransactionTimeout verifies that the transaction pool's timeout monitor
// cancels transactions that exceed the configured timeout duration.
func TestTransactionTimeout(t *testing.T) {
	pool := NewTransactionPool(WithPoolTimeout(100 * time.Millisecond))
	defer pool.Close()

	req := NewRequest(0, FuncReadHoldingRegisters, makeReadRegistersPDU(0, 1))
	tx, err := pool.Place(context.Background(), req)
	if err != nil {
		t.Fatalf("place: %v", err)
	}

	// Wait for the timeout monitor to fire. It checks every second, so
	// be patient but bounded.
	select {
	case err := <-tx.ErrCh:
		if err != ErrTransactionTimeout {
			t.Fatalf("expected ErrTransactionTimeout, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transaction was not timed out within expected window")
	}

	// The transaction should have been removed from the pool.
	if pool.GetCount() != 0 {
		t.Fatalf("expected 0 active transactions, got %d", pool.GetCount())
	}
}

// TestServerStopDuringAcceptLoop tests that Stop() cleanly shuts down the
// server while the acceptLoop is running with a pipeListener.
func TestServerStopDuringAcceptLoop(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	pl := newPipeListener(serverConn)
	store := NewMemoryStore()

	srv := NewServer("test",
		WithServerDataStore(store),
		WithServerListener(pl),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	// Give the accept loop time to accept the connection.
	time.Sleep(50 * time.Millisecond)

	// Stopping should not hang or race.
	done := make(chan struct{})
	go func() {
		srv.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within timeout")
	}

	if srv.IsRunning() {
		t.Fatal("server should not be running after Stop")
	}
}

// TestClientCallbacksOnConnect verifies the OnClientConnect and
// OnClientDisconnect callbacks fire correctly.
func TestClientCallbacksOnConnect(t *testing.T) {
	var connectCalled, disconnectCalled bool
	var connectMu sync.Mutex

	serverConn, clientConn := net.Pipe()

	srv := NewServer("test",
		WithServerDataStore(NewMemoryStore()),
		WithServerConn(serverConn),
		WithOnClientConnect(func(c ConnectedClient) {
			connectMu.Lock()
			connectCalled = true
			connectMu.Unlock()
		}),
		WithOnClientDisconnect(func(c ConnectedClient) {
			connectMu.Lock()
			disconnectCalled = true
			connectMu.Unlock()
		}),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	// Give the connection handler time to start and fire connect callback.
	time.Sleep(50 * time.Millisecond)

	connectMu.Lock()
	if !connectCalled {
		t.Error("OnClientConnect was not called")
	}
	connectMu.Unlock()

	// Close client side to trigger disconnect callback.
	clientConn.Close()
	time.Sleep(100 * time.Millisecond)

	connectMu.Lock()
	if !disconnectCalled {
		t.Error("OnClientDisconnect was not called")
	}
	connectMu.Unlock()

	srv.Stop(context.Background())
}

// ---------------------------------------------------------------------------
// chanListener accepts many pipe connections via a buffered channel.
// ---------------------------------------------------------------------------

type chanListener struct {
	ch   chan net.Conn
	done chan struct{}
	once sync.Once
}

func newChanListener(buffer int) *chanListener {
	return &chanListener{
		ch:   make(chan net.Conn, buffer),
		done: make(chan struct{}),
	}
}

func (l *chanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.done:
		return nil, &net.OpError{Op: "accept", Net: "pipe", Err: net.ErrClosed}
	}
}

func (l *chanListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *chanListener) Addr() net.Addr { return pipeAddr{} }

// ===========================================================================
// Stress Test: 1 writer + 1000 readers through loopback server
// ===========================================================================

func TestStressConcurrentClients(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const (
		numReaders = 1000
		numWrites  = 20
		writeDelay = 2 * time.Millisecond
		registerAt = Address(0)
	)

	// Create server with a MemoryStore.
	store := NewMemoryStore()
	ln := newChanListener(numReaders + 1)

	srv := NewServer("test",
		WithServerDataStore(store),
		WithServerListener(ln),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}

	// dial creates a pipe pair, hands the server side to the listener,
	// and returns the client-side conn.
	dial := func() net.Conn {
		t.Helper()
		serverConn, clientConn := net.Pipe()
		ln.ch <- serverConn
		return clientConn
	}

	// --- Writer: uses a real Client through the full Modbus stack ---

	writerConn := dial()
	writerTC := NewTCPConn("test", WithConn(writerConn))
	if err := writerTC.Connect(context.Background()); err != nil {
		t.Fatalf("writer connect: %v", err)
	}
	writerTP, err := transport.NewDirectTransport(context.Background(),
		transport.ConnectorFunc[*TCPConn](func(ctx context.Context) (*TCPConn, error) {
			return writerTC, nil
		}),
		transport.CloserFunc[*TCPConn](func(conn *TCPConn) error {
			return conn.Disconnect(context.Background())
		}),
	)
	if err != nil {
		t.Fatalf("writer transport: %v", err)
	}
	writer := NewClient(writerTP)

	// --- Readers: use raw wire-level I/O for lightweight concurrency ---
	// Each TCPConn creates a 65536-entry transaction pool, making 1000
	// Client instances prohibitively expensive. Raw connections are cheap.

	readerConns := make([]net.Conn, numReaders)
	for i := range readerConns {
		readerConns[i] = dial()
	}

	// Give accept loop time to process all connections.
	time.Sleep(200 * time.Millisecond)

	var wg sync.WaitGroup

	// --- Writer goroutine: write 1, 2, 3, ... numWrites with delays ---

	ctx := context.Background()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int32(1); i <= numWrites; i++ {
			if err := writer.WriteSingleRegister(ctx, registerAt, RegisterValue(i)); err != nil {
				t.Errorf("write %d: %v", i, err)
				return
			}
			time.Sleep(writeDelay)
		}
	}()

	// --- Reader goroutines: raw Modbus TCP read requests ---

	type readerResult struct {
		lastSeen atomic.Int32
		reads    atomic.Int32
	}
	results := make([]readerResult, numReaders)

	for i, conn := range readerConns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			txID := TransactionID(1)
			for {
				// Build ReadHoldingRegisters request (FC 0x03)
				pdu := makeReadRegistersPDU(registerAt, 1)
				req := NewRequest(1, FuncReadHoldingRegisters, pdu)
				req.SetTransactionID(txID)
				txID++

				data, err := req.Encode()
				if err != nil {
					return
				}
				if _, err := conn.Write(data); err != nil {
					return // connection closed
				}

				// Read MBAP header
				header := make([]byte, TCPHeaderLength)
				if _, err := io.ReadFull(conn, header); err != nil {
					return
				}
				length := binary.BigEndian.Uint16(header[4:6])
				body := make([]byte, int(length)-1)
				if _, err := io.ReadFull(conn, body); err != nil {
					return
				}

				// Parse register value from response PDU.
				// Response: [FC:1][ByteCount:1][RegHi:1][RegLo:1]
				if len(body) >= 3 {
					v := int32(binary.BigEndian.Uint16(body[1:3]))
					results[i].reads.Add(1)
					if v > results[i].lastSeen.Load() {
						results[i].lastSeen.Store(v)
					}
					if v >= numWrites {
						return
					}
				}
			}
		}()
	}

	// --- Wait for all goroutines with timeout ---

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for writer + readers to complete")
	}

	// --- Verify reader results ---

	for i := range results {
		if results[i].reads.Load() == 0 {
			t.Errorf("reader %d: completed 0 reads", i)
		}
		if results[i].lastSeen.Load() < numWrites {
			t.Errorf("reader %d: last seen %d, want >= %d",
				i, results[i].lastSeen.Load(), numWrites)
		}
	}

	// --- Clean shutdown: writer ---

	writer.Close()

	// --- Clean shutdown: all readers ---

	for _, conn := range readerConns {
		conn.Close()
	}

	// --- Give server time to clean up all handler goroutines ---

	time.Sleep(200 * time.Millisecond)

	// --- Verify server state is clean ---
	// Note: Modbus server tracks clients by RemoteAddr string. With net.Pipe
	// all connections share the same "pipe" address, so ConnectedClients
	// reflects only the last connection. We verify shutdown is clean instead.

	srv.Stop(context.Background())
}

// TestServerShutdownDrain verifies that Stop() actively closes all tracked
// client connections, causing handler goroutines to exit cleanly.
func TestServerShutdownDrain(t *testing.T) {
	serverConn, clientConn := net.Pipe()

	store := NewMemoryStore()
	store.SetHoldingRegister(0, 0x1234)

	srv := NewServer("test",
		WithServerDataStore(store),
		WithServerConn(serverConn),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	// Verify the server is running and the client is connected.
	if !srv.IsRunning() {
		t.Fatal("server should be running")
	}

	// Give handleConnection time to start.
	time.Sleep(50 * time.Millisecond)

	// Perform a request to prove the connection is fully active.
	sendRawRequest(t, clientConn, 1, 1, FuncReadHoldingRegisters, makeReadRegistersPDU(0, 1))
	resp := readRawResponse(t, clientConn)
	if resp.GetPDU().FunctionCode != FuncReadHoldingRegisters {
		t.Fatalf("unexpected function code: %s", resp.GetPDU().FunctionCode)
	}

	// Start a goroutine that blocks on Read — it should unblock when Stop()
	// closes the connection.
	var wg sync.WaitGroup
	wg.Add(1)
	var readErr error
	go func() {
		defer wg.Done()
		buf := make([]byte, 1)
		_, readErr = clientConn.Read(buf)
	}()

	// Stop the server — this should close the client connection.
	srv.Stop(context.Background())

	if srv.IsRunning() {
		t.Fatal("server should not be running after Stop")
	}

	// Wait for the blocked Read to unblock.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler goroutine did not exit within timeout after Stop()")
	}

	// The read should have returned an error (pipe closed).
	if readErr == nil {
		t.Error("expected read error after Stop(), got nil")
	}

	// Clean up.
	clientConn.Close()
}

// TestServerMaxClients verifies the server rejects connections once the
// configured max-client limit is reached, and allows new connections once
// an existing client disconnects.
func TestServerMaxClients(t *testing.T) {
	const maxClients = 2

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := NewServer("test",
		WithServerListener(ln),
		WithMaxClients(maxClients),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })

	addr := ln.Addr().String()

	conn1, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 1: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 2: %v", err)
	}
	defer conn2.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) >= maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != maxClients {
		t.Fatalf("expected %d connected clients, got %d", maxClients, got)
	}

	// 3rd connection should be rejected.
	conn3, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 3: %v", err)
	}
	defer conn3.Close()

	conn3.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, readErr := conn3.Read(buf)
	if readErr == nil {
		t.Fatal("expected 3rd client to be rejected, but read succeeded")
	}

	// Disconnect one client to free a slot.
	conn1.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) < maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// New connection should now succeed.
	conn4, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial client 4: %v", err)
	}
	defer conn4.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.ConnectedClients()) >= maxClients {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(srv.ConnectedClients()); got != maxClients {
		t.Fatalf("expected %d connected clients after reconnect, got %d", maxClients, got)
	}
}
