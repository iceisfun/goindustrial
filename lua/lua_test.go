package lua

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/golua/compiler"
	"github.com/iceisfun/golua/parser"
	"github.com/iceisfun/golua/stdlib"
	"github.com/iceisfun/golua/vm"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

// ---------------------------------------------------------------------------
// Lua VM helper: parse, compile, run with industrial globals
// ---------------------------------------------------------------------------

func runLua(t *testing.T, source string) []vm.Value {
	t.Helper()

	block, err := parser.Parse("test", source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	proto, err := compiler.Compile("test", block)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	v := vm.New(vm.WithContext(ctx))
	stdlib.Open(v)
	Open(v)

	results, err := v.Run(proto)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return results
}

// runLuaExpectPanic runs a Lua script that is expected to panic (error).
// Returns the panic message string if one occurs, or fails the test if no panic.
func runLuaExpectPanic(t *testing.T, source string) string {
	t.Helper()

	block, err := parser.Parse("test", source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	proto, err := compiler.Compile("test", block)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	v := vm.New(vm.WithContext(ctx))
	stdlib.Open(v)
	Open(v)

	_, err = v.Run(proto)
	if err != nil {
		return err.Error()
	}
	t.Fatal("expected Lua script to error, but it succeeded")
	return ""
}

// ---------------------------------------------------------------------------
// Modbus test server: start a real TCP server on localhost, return address
// ---------------------------------------------------------------------------

func startModbusServer(t *testing.T) (string, int, *modbus.MemoryStore) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)
	store := modbus.NewMemoryStore()

	srv := modbus.NewServer(addr.IP.String(),
		modbus.WithServerDataStore(store),
		modbus.WithServerListener(ln),
	)

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}

	t.Cleanup(func() {
		srv.Stop(context.Background())
	})

	return addr.IP.String(), addr.Port, store
}

// ---------------------------------------------------------------------------
// Mock EIP TCP server: handles session registration and CIP read/write tag
// requests at the wire level. This server handles symbolic segment paths
// (0x91) which the standard MessageRouter does not support.
// ---------------------------------------------------------------------------

type mockEIPServer struct {
	ln       net.Listener
	tags     map[string][]byte // tag name -> raw CIP response data (type+value)
	mu       sync.Mutex
	wg       sync.WaitGroup
	stopCh   chan struct{}
}

func newMockEIPServer(t *testing.T) *mockEIPServer {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("eip listen: %v", err)
	}

	s := &mockEIPServer{
		ln:     ln,
		tags:   make(map[string][]byte),
		stopCh: make(chan struct{}),
	}

	s.wg.Add(1)
	go s.acceptLoop()

	t.Cleanup(func() {
		close(s.stopCh)
		s.ln.Close()
		s.wg.Wait()
	})

	return s
}

func (s *mockEIPServer) addr() *net.TCPAddr {
	return s.ln.Addr().(*net.TCPAddr)
}

func (s *mockEIPServer) setTag(name string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tags[name] = data
}

func (s *mockEIPServer) getTag(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.tags[name]
	return d, ok
}

func (s *mockEIPServer) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
				return
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *mockEIPServer) handleConn(conn net.Conn) {
	defer conn.Close()

	var sessionHandle uint32 = 1

	for {
		// Read EIP header (24 bytes)
		header := make([]byte, eip.HeaderSize)
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}

		cmd := eip.Command(binary.LittleEndian.Uint16(header[0:2]))
		dataLen := binary.LittleEndian.Uint16(header[2:4])
		var senderCtx [8]byte
		copy(senderCtx[:], header[12:20])

		var data []byte
		if dataLen > 0 {
			data = make([]byte, dataLen)
			if _, err := io.ReadFull(conn, data); err != nil {
				return
			}
		}

		switch cmd {
		case eip.CommandRegisterSession:
			// Respond with session handle
			resp := make([]byte, eip.HeaderSize+4)
			binary.LittleEndian.PutUint16(resp[0:], uint16(eip.CommandRegisterSession))
			binary.LittleEndian.PutUint16(resp[2:], 4) // data length
			binary.LittleEndian.PutUint32(resp[4:], sessionHandle)
			binary.LittleEndian.PutUint32(resp[8:], 0) // status OK
			copy(resp[12:20], senderCtx[:])
			// Register session data: protocol version + options
			binary.LittleEndian.PutUint16(resp[24:], 1) // version
			binary.LittleEndian.PutUint16(resp[26:], 0) // options
			conn.Write(resp)

		case eip.CommandUnregisterSession:
			return

		case eip.CommandSendRRData:
			s.handleSendRRData(conn, sessionHandle, senderCtx, data)

		default:
			// Unknown command, send error
			resp := make([]byte, eip.HeaderSize)
			binary.LittleEndian.PutUint16(resp[0:], uint16(cmd))
			binary.LittleEndian.PutUint32(resp[4:], sessionHandle)
			binary.LittleEndian.PutUint32(resp[8:], 0x01) // invalid command
			copy(resp[12:20], senderCtx[:])
			conn.Write(resp)
		}
	}
}

func (s *mockEIPServer) handleSendRRData(conn net.Conn, sessionHandle uint32, senderCtx [8]byte, data []byte) {
	// Skip 6-byte SendRRData header (interface handle + timeout)
	if len(data) < 6 {
		return
	}

	// Decode CPF
	cpfData := data[6:]
	cpf, err := eip.DecodeCommonPacketFormat(cpfData)
	if err != nil {
		return
	}

	item := cpf.FindItemByType(eip.ItemIDUnconnectedMessage)
	if item == nil {
		return
	}

	// Decode the CIP message router request
	mrReq, err := cip.DecodeMessageRouterRequest(item.Data)
	if err != nil {
		return
	}

	// Handle based on service code
	var mrResp *cip.MessageRouterResponse
	switch mrReq.Service {
	case cip.ServiceReadTag:
		mrResp = s.handleReadTag(mrReq)
	case cip.ServiceWriteTag:
		mrResp = s.handleWriteTag(mrReq)
	default:
		mrResp = &cip.MessageRouterResponse{
			Service:       mrReq.Service | 0x80,
			GeneralStatus: cip.StatusServiceNotSupported,
		}
	}

	// Encode response
	mrRespBytes, err := mrResp.Encode()
	if err != nil {
		return
	}

	respCPF := eip.NewCommonPacketFormat(
		eip.NewCPFItem(eip.ItemIDNullAddress, nil),
		eip.NewCPFItem(eip.ItemIDUnconnectedMessage, mrRespBytes),
	)
	cpfRespBytes, err := respCPF.Encode()
	if err != nil {
		return
	}

	// Build SendRRData response: 6-byte header + CPF
	rrData := make([]byte, 6+len(cpfRespBytes))
	copy(rrData[6:], cpfRespBytes)

	// Build EIP header
	resp := make([]byte, eip.HeaderSize+len(rrData))
	binary.LittleEndian.PutUint16(resp[0:], uint16(eip.CommandSendRRData))
	binary.LittleEndian.PutUint16(resp[2:], uint16(len(rrData)))
	binary.LittleEndian.PutUint32(resp[4:], sessionHandle)
	binary.LittleEndian.PutUint32(resp[8:], 0) // status OK
	copy(resp[12:20], senderCtx[:])
	copy(resp[eip.HeaderSize:], rrData)
	conn.Write(resp)
}

// extractSymbolicName pulls the tag name from a symbolic ANSI extended path segment.
func extractSymbolicName(path cip.Path) string {
	b := path.Bytes()
	if len(b) < 2 || b[0] != 0x91 {
		return ""
	}
	nameLen := int(b[1])
	if len(b) < 2+nameLen {
		return ""
	}
	return string(b[2 : 2+nameLen])
}

func (s *mockEIPServer) handleReadTag(req *cip.MessageRouterRequest) *cip.MessageRouterResponse {
	tagName := extractSymbolicName(req.RequestPath)
	if tagName == "" {
		return &cip.MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: cip.StatusPathSegmentError,
		}
	}

	data, ok := s.getTag(tagName)
	if !ok {
		return &cip.MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: cip.StatusPathSegmentError,
		}
	}

	return &cip.MessageRouterResponse{
		Service:       req.Service | 0x80,
		GeneralStatus: cip.StatusSuccess,
		ResponseData:  data,
	}
}

func (s *mockEIPServer) handleWriteTag(req *cip.MessageRouterRequest) *cip.MessageRouterResponse {
	tagName := extractSymbolicName(req.RequestPath)
	if tagName == "" {
		return &cip.MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: cip.StatusPathSegmentError,
		}
	}

	// Write tag request data: [TypeCode:2][Elements:2][Data...]
	if len(req.RequestData) < 4 {
		return &cip.MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: cip.StatusNotEnoughData,
		}
	}

	// Store the full response format: type code + value data
	typeCode := binary.LittleEndian.Uint16(req.RequestData[0:2])
	valueData := req.RequestData[4:] // skip type + elements

	stored := make([]byte, 2+len(valueData))
	binary.LittleEndian.PutUint16(stored[0:2], typeCode)
	copy(stored[2:], valueData)

	s.setTag(tagName, stored)

	return &cip.MessageRouterResponse{
		Service:       req.Service | 0x80,
		GeneralStatus: cip.StatusSuccess,
	}
}

// makeDINTTagData creates raw CIP tag data for a DINT (int32) value.
func makeDINTTagData(val int32) []byte {
	data := make([]byte, 6)
	binary.LittleEndian.PutUint16(data[0:2], uint16(cip.TypeDINT))
	binary.LittleEndian.PutUint32(data[2:6], uint32(val))
	return data
}


// ===========================================================================
// Modbus Tests
// ===========================================================================

func TestModbusReadHoldingRegisters(t *testing.T) {
	host, port, store := startModbusServer(t)

	// Pre-populate registers at addresses 0-2
	ctx := context.Background()
	store.WriteSingleRegister(ctx, 0, 100)
	store.WriteSingleRegister(ctx, 1, 200)
	store.WriteSingleRegister(ctx, 2, 300)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		local regs = client:read_holding_registers(0, 3)
		client:close()
		return regs[1], regs[2], regs[3]
	`, host, port)

	results := runLua(t, script)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].AsInt() != 100 {
		t.Errorf("reg[0] = %d, want 100", results[0].AsInt())
	}
	if results[1].AsInt() != 200 {
		t.Errorf("reg[1] = %d, want 200", results[1].AsInt())
	}
	if results[2].AsInt() != 300 {
		t.Errorf("reg[2] = %d, want 300", results[2].AsInt())
	}
}

func TestModbusWriteRegister(t *testing.T) {
	host, port, store := startModbusServer(t)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		client:write_register(10, 12345)
		client:close()
	`, host, port)

	runLua(t, script)

	// Verify the server received the write
	ctx := context.Background()
	vals, err := store.ReadHoldingRegisters(ctx, 10, 1)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if vals[0] != 12345 {
		t.Errorf("register 10 = %d, want 12345", vals[0])
	}
}

func TestModbusWriteMultipleRegisters(t *testing.T) {
	host, port, store := startModbusServer(t)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		client:write_registers(20, {1111, 2222, 3333})
		client:close()
	`, host, port)

	runLua(t, script)

	ctx := context.Background()
	vals, err := store.ReadHoldingRegisters(ctx, 20, 3)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if vals[0] != 1111 || vals[1] != 2222 || vals[2] != 3333 {
		t.Errorf("registers = %v, want [1111, 2222, 3333]", vals)
	}
}

func TestModbusReadWriteCoils(t *testing.T) {
	host, port, store := startModbusServer(t)

	// Write coils from Lua
	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		client:write_coil(0, true)
		client:write_coil(1, false)
		client:write_coil(2, true)

		local coils = client:read_coils(0, 3)
		client:close()
		return coils[1], coils[2], coils[3]
	`, host, port)

	results := runLua(t, script)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].AsBool() != true {
		t.Errorf("coil[0] = %v, want true", results[0].AsBool())
	}
	if results[1].AsBool() != false {
		t.Errorf("coil[1] = %v, want false", results[1].AsBool())
	}
	if results[2].AsBool() != true {
		t.Errorf("coil[2] = %v, want true", results[2].AsBool())
	}

	// Verify server state
	ctx := context.Background()
	coilVals, err := store.ReadCoils(ctx, 0, 3)
	if err != nil {
		t.Fatalf("read coils from store: %v", err)
	}
	if coilVals[0] != true || coilVals[1] != false || coilVals[2] != true {
		t.Errorf("server coils = %v, want [true, false, true]", coilVals)
	}
}

func TestModbusWriteMultipleCoils(t *testing.T) {
	host, port, store := startModbusServer(t)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		client:write_coils(10, {true, false, true, true})
		client:close()
	`, host, port)

	runLua(t, script)

	ctx := context.Background()
	coilVals, err := store.ReadCoils(ctx, 10, 4)
	if err != nil {
		t.Fatalf("read coils from store: %v", err)
	}
	if coilVals[0] != true || coilVals[1] != false || coilVals[2] != true || coilVals[3] != true {
		t.Errorf("coils = %v, want [true, false, true, true]", coilVals)
	}
}

func TestModbusClose(t *testing.T) {
	host, port, _ := startModbusServer(t)

	// After close(), further operations should panic
	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		client:close()

		-- Try to read after close: should error
		local ok, err = pcall(function()
			client:read_holding_registers(0, 1)
		end)
		return ok, tostring(err)
	`, host, port)

	results := runLua(t, script)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].AsBool() != false {
		t.Error("expected pcall to return false after close")
	}
}

func TestModbusConnectionRefused(t *testing.T) {
	// Try to connect to a port that is not listening
	errMsg := runLuaExpectPanic(t, `
		local client = modbus.connect("127.0.0.1", {port = 1, timeout = 2})
	`)
	if !strings.Contains(errMsg, "modbus.connect") {
		t.Errorf("expected error to contain 'modbus.connect', got: %s", errMsg)
	}
}

func TestModbusReadWriteRegistersAtomicOp(t *testing.T) {
	host, port, store := startModbusServer(t)

	// Pre-populate some registers to be read
	ctx := context.Background()
	store.WriteSingleRegister(ctx, 0, 10)
	store.WriteSingleRegister(ctx, 1, 20)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		local regs = client:read_write_registers(0, 2, 50, {999, 888})
		client:close()
		return regs[1], regs[2]
	`, host, port)

	results := runLua(t, script)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].AsInt() != 10 {
		t.Errorf("read reg[0] = %d, want 10", results[0].AsInt())
	}
	if results[1].AsInt() != 20 {
		t.Errorf("read reg[1] = %d, want 20", results[1].AsInt())
	}

	// Verify the writes happened
	vals, err := store.ReadHoldingRegisters(ctx, 50, 2)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if vals[0] != 999 || vals[1] != 888 {
		t.Errorf("written registers = %v, want [999, 888]", vals)
	}
}

func TestModbusToInt32(t *testing.T) {
	host, port, store := startModbusServer(t)

	// Store a 32-bit integer across two registers (big-endian):
	// Value = 100000 = 0x000186A0
	// High register = 0x0001, Low register = 0x86A0
	ctx := context.Background()
	store.WriteSingleRegister(ctx, 0, 0x0001)
	store.WriteSingleRegister(ctx, 1, 0x86A0)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		local regs = client:read_holding_registers(0, 2)
		local val = client:to_int32(regs[1], regs[2])
		client:close()
		return val
	`, host, port)

	results := runLua(t, script)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AsInt() != 100000 {
		t.Errorf("to_int32 = %d, want 100000", results[0].AsInt())
	}
}

func TestModbusToFloat32(t *testing.T) {
	host, port, store := startModbusServer(t)

	// Store float32(3.14) across two registers (big-endian):
	// 3.14 as float32 = 0x4048F5C3
	// High register = 0x4048, Low register = 0xF5C3
	ctx := context.Background()
	store.WriteSingleRegister(ctx, 0, 0x4048)
	store.WriteSingleRegister(ctx, 1, 0xF5C3)

	script := fmt.Sprintf(`
		local client = modbus.connect("%s", {port = %d, unit = 1, timeout = 5})
		local regs = client:read_holding_registers(0, 2)
		local val = client:to_float32(regs[1], regs[2])
		client:close()
		return val
	`, host, port)

	results := runLua(t, script)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0].AsFloat()
	// float32(3.14) = 3.1400001049041748 in float64
	if got < 3.13 || got > 3.15 {
		t.Errorf("to_float32 = %f, want ~3.14", got)
	}
}

// ===========================================================================
// EtherNet/IP Tests
// ===========================================================================

func TestEIPReadTag(t *testing.T) {
	srv := newMockEIPServer(t)
	addr := srv.addr()

	// Set a DINT tag with value 42
	srv.setTag("TestTag", makeDINTTagData(42))

	script := fmt.Sprintf(`
		local client = eip.connect("%s:%d", {timeout = 5})
		local val = client:read_tag("TestTag")
		client:close()
		return val
	`, addr.IP.String(), addr.Port)

	results := runLua(t, script)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AsInt() != 42 {
		t.Errorf("read_tag = %d, want 42", results[0].AsInt())
	}
}

func TestEIPWriteTagAndReadBack(t *testing.T) {
	srv := newMockEIPServer(t)
	addr := srv.addr()

	// Initialize tag with value 0
	srv.setTag("Counter", makeDINTTagData(0))

	script := fmt.Sprintf(`
		local client = eip.connect("%s:%d", {timeout = 5})

		-- Write a new value
		client:write_tag("Counter", 100, eip.types.DINT)

		-- Read it back
		local val = client:read_tag("Counter")
		client:close()
		return val
	`, addr.IP.String(), addr.Port)

	results := runLua(t, script)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AsInt() != 100 {
		t.Errorf("read_tag after write = %d, want 100", results[0].AsInt())
	}
}

func TestEIPWriteTagAutoType(t *testing.T) {
	srv := newMockEIPServer(t)
	addr := srv.addr()

	script := fmt.Sprintf(`
		local client = eip.connect("%s:%d", {timeout = 5})

		-- Write without explicit type (defaults to DINT for integers)
		client:write_tag("AutoTag", 55)

		local val = client:read_tag("AutoTag")
		client:close()
		return val
	`, addr.IP.String(), addr.Port)

	results := runLua(t, script)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].AsInt() != 55 {
		t.Errorf("auto-typed write = %d, want 55", results[0].AsInt())
	}
}

func TestEIPClose(t *testing.T) {
	srv := newMockEIPServer(t)
	addr := srv.addr()

	srv.setTag("Dummy", makeDINTTagData(1))

	script := fmt.Sprintf(`
		local client = eip.connect("%s:%d", {timeout = 5})
		client:close()

		-- Operations after close should fail
		local ok, err = pcall(function()
			client:read_tag("Dummy")
		end)
		return ok, tostring(err)
	`, addr.IP.String(), addr.Port)

	results := runLua(t, script)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].AsBool() != false {
		t.Error("expected pcall to return false after close")
	}
}

func TestEIPConnectionRefused(t *testing.T) {
	errMsg := runLuaExpectPanic(t, `
		local client = eip.connect("127.0.0.1:1", {timeout = 2})
	`)
	if !strings.Contains(errMsg, "eip.connect") {
		t.Errorf("expected error to contain 'eip.connect', got: %s", errMsg)
	}
}

func TestEIPReadNonexistentTag(t *testing.T) {
	srv := newMockEIPServer(t)
	addr := srv.addr()

	script := fmt.Sprintf(`
		local client = eip.connect("%s:%d", {timeout = 5})
		local ok, err = pcall(function()
			client:read_tag("NoSuchTag")
		end)
		client:close()
		return ok, tostring(err)
	`, addr.IP.String(), addr.Port)

	results := runLua(t, script)
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	if results[0].AsBool() != false {
		t.Error("expected pcall to return false for nonexistent tag")
	}
}

// ===========================================================================
// Open() / Module Registration Tests
// ===========================================================================

func TestOpenRegistersGlobals(t *testing.T) {
	// Verify that Open() registers both "modbus" and "eip" globals
	script := `
		return type(modbus), type(eip), type(modbus.connect), type(eip.connect)
	`
	results := runLua(t, script)
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].AsString() != "table" {
		t.Errorf("modbus type = %s, want table", results[0].AsString())
	}
	if results[1].AsString() != "table" {
		t.Errorf("eip type = %s, want table", results[1].AsString())
	}
	if results[2].AsString() != "function" {
		t.Errorf("modbus.connect type = %s, want function", results[2].AsString())
	}
	if results[3].AsString() != "function" {
		t.Errorf("eip.connect type = %s, want function", results[3].AsString())
	}
}

func TestEIPTypeConstants(t *testing.T) {
	script := `
		return eip.types.BOOL, eip.types.INT, eip.types.DINT, eip.types.REAL, eip.types.STRING
	`
	results := runLua(t, script)
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	expected := []string{"BOOL", "INT", "DINT", "REAL", "STRING"}
	for i, exp := range expected {
		if results[i].AsString() != exp {
			t.Errorf("eip.types result[%d] = %s, want %s", i, results[i].AsString(), exp)
		}
	}
}
