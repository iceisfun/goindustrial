package modbus

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"
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
