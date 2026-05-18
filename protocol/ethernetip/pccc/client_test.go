package pccc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/monitor"
	"github.com/iceisfun/goindustrial/plc"
)

// fakeExecutor records every PCCC command sent through it and produces
// canned replies. The reply function receives the decoded request (CMD,
// FNC, file, element, sub-element, data) and may return either raw reply
// bytes or an error. If replyFunc is nil, default success replies are
// generated for typed-logical reads and writes.
type fakeExecutor struct {
	sent         [][]byte
	replyFunc    func(req []byte) ([]byte, error)
	connectCalls int
	disconnect   int
	isConnected  bool
}

func (f *fakeExecutor) ExecutePCCC(_ context.Context, cmd []byte) ([]byte, error) {
	clone := append([]byte(nil), cmd...)
	f.sent = append(f.sent, clone)
	if f.replyFunc != nil {
		return f.replyFunc(clone)
	}
	return defaultSuccess(clone), nil
}

// Default reply: echo CMD|0x40, STS=0, TNS echoed; for a typed read with
// FNC=0xA2, return byteSize zero bytes. For a typed write (FNC=0xAA),
// return no payload.
func defaultSuccess(req []byte) []byte {
	if len(req) < 5 {
		return nil
	}
	tns := req[2:4]
	fnc := req[4]
	hdr := []byte{req[0] | ReplyBit, 0x00, tns[0], tns[1]}
	if fnc == FuncProtectedTypedLogicalRead && len(req) >= 6 {
		size := int(req[5])
		out := make([]byte, 4+size)
		copy(out, hdr)
		return out
	}
	return hdr
}

// ---------------------------------------------------------------------------
// ReadAddress / Read
// ---------------------------------------------------------------------------

func TestReadAddressInteger(t *testing.T) {
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			return []byte{req[0] | ReplyBit, 0x00, req[2], req[3], 0x2A, 0x00}, nil
		},
	}
	c := NewClient(exec)

	v, err := c.ReadAddress(context.Background(), "N7:0")
	if err != nil {
		t.Fatalf("ReadAddress: %v", err)
	}
	got, err := v.Int()
	if err != nil {
		t.Fatalf("v.Int: %v", err)
	}
	if got != 42 {
		t.Errorf("value: got %d want 42", got)
	}
	if v.Type != plc.TypeInt16 {
		t.Errorf("Type: got %s want int16", v.Type)
	}
	if v.ByteOrder != plc.ByteOrderLittleEndian {
		t.Errorf("ByteOrder: got %s want little-endian", v.ByteOrder)
	}

	// Verify the request: CMD=0x0F, FNC=0xA2, size=2, file=7, type=N(0x89), elem=0, sub=0
	if len(exec.sent) != 1 {
		t.Fatalf("sent: got %d frames, want 1", len(exec.sent))
	}
	req := exec.sent[0]
	if req[0] != CmdProtectedTypedLogical || req[4] != FuncProtectedTypedLogicalRead {
		t.Errorf("CMD/FNC: got 0x%02X/0x%02X", req[0], req[4])
	}
	if req[5] != 2 || req[6] != 7 || req[7] != byte(FileTypeInteger) || req[8] != 0 || req[9] != 0 {
		t.Errorf("addressing bytes: % X", req[5:10])
	}
}

func TestReadAddressFloat(t *testing.T) {
	bits := math.Float32bits(3.14)
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			payload := []byte{
				byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24),
			}
			return append([]byte{req[0] | ReplyBit, 0x00, req[2], req[3]}, payload...), nil
		},
	}
	c := NewClient(exec)

	v, err := c.ReadAddress(context.Background(), "F8:5")
	if err != nil {
		t.Fatalf("ReadAddress: %v", err)
	}
	got, err := v.Float32()
	if err != nil {
		t.Fatalf("v.Float32: %v", err)
	}
	if got != 3.14 {
		t.Errorf("value: got %v want 3.14", got)
	}
	if v.Type != plc.TypeFloat32 {
		t.Errorf("Type: got %s want float32", v.Type)
	}
	// Verify the request asked for 4 bytes at F8:5.
	req := exec.sent[0]
	if req[5] != 4 || req[6] != 8 || req[7] != byte(FileTypeFloat) || req[8] != 5 {
		t.Errorf("addressing bytes: % X", req[5:10])
	}
}

func TestReadAddressBit(t *testing.T) {
	// Word value = 0b0000_0000_0001_0100 (bits 2 and 4 set)
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			return []byte{req[0] | ReplyBit, 0x00, req[2], req[3], 0x14, 0x00}, nil
		},
	}
	c := NewClient(exec)

	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"B3:0/0", false},
		{"B3:0/2", true},
		{"B3:0/3", false},
		{"B3:0/4", true},
	} {
		t.Run(tc.addr, func(t *testing.T) {
			v, err := c.ReadAddress(context.Background(), tc.addr)
			if err != nil {
				t.Fatalf("ReadAddress: %v", err)
			}
			if v.Type != plc.TypeBool {
				t.Errorf("Type: got %s want bool", v.Type)
			}
			if v.Bool() != tc.want {
				t.Errorf("Bool: got %v want %v", v.Bool(), tc.want)
			}
		})
	}
}

func TestReadAddressTimerWholeElement(t *testing.T) {
	// 6 bytes: control=0xE000, PRE=1000 (0x03E8), ACC=250 (0x00FA)
	payload := []byte{0x00, 0xE0, 0xE8, 0x03, 0xFA, 0x00}
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			return append([]byte{req[0] | ReplyBit, 0x00, req[2], req[3]}, payload...), nil
		},
	}
	c := NewClient(exec)

	v, err := c.ReadAddress(context.Background(), "T4:0")
	if err != nil {
		t.Fatalf("ReadAddress: %v", err)
	}
	if v.Type != plc.TypeBytes {
		t.Errorf("Type: got %s want bytes", v.Type)
	}
	if !bytes.Equal(v.Raw, payload) {
		t.Errorf("Raw: got % X want % X", v.Raw, payload)
	}

	tm, err := DecodeTimer(v.Raw)
	if err != nil {
		t.Fatalf("DecodeTimer: %v", err)
	}
	if tm.PRE != 1000 || tm.ACC != 250 || !tm.EN() {
		t.Errorf("decoded timer: %s", tm)
	}

	// Request should have asked for 6 bytes.
	if exec.sent[0][5] != 6 {
		t.Errorf("read size: got %d want 6", exec.sent[0][5])
	}
}

func TestReadAddressTimerSubField(t *testing.T) {
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			return []byte{req[0] | ReplyBit, 0x00, req[2], req[3], 0xFA, 0x00}, nil
		},
	}
	c := NewClient(exec)

	v, err := c.ReadAddress(context.Background(), "T4:5.ACC")
	if err != nil {
		t.Fatalf("ReadAddress: %v", err)
	}
	if v.Type != plc.TypeInt16 {
		t.Errorf("Type: got %s want int16", v.Type)
	}
	got, _ := v.Int()
	if got != 250 {
		t.Errorf("value: got %d want 250", got)
	}
	// Request should have asked for 2 bytes at sub-element 2.
	req := exec.sent[0]
	if req[5] != 2 || req[6] != 4 || req[7] != byte(FileTypeTimer) || req[8] != 5 || req[9] != 2 {
		t.Errorf("addressing: % X", req[5:10])
	}
}

func TestReadWords(t *testing.T) {
	// Reply with 5 INT words: 1, 2, 3, -4, 5
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			out := []byte{req[0] | ReplyBit, 0x00, req[2], req[3]}
			for _, v := range []int16{1, 2, 3, -4, 5} {
				out = append(out, byte(uint16(v)), byte(uint16(v)>>8))
			}
			return out, nil
		},
	}
	c := NewClient(exec)

	got, err := c.ReadWords(context.Background(), "N7:0", 5)
	if err != nil {
		t.Fatalf("ReadWords: %v", err)
	}
	want := []int16{1, 2, 3, -4, 5}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d]: got %d want %d", i, got[i], want[i])
		}
	}
	// Request should have asked for 10 bytes.
	if exec.sent[0][5] != 10 {
		t.Errorf("read size: got %d want 10", exec.sent[0][5])
	}
}

func TestReadWordsRejectsBitSuffix(t *testing.T) {
	c := NewClient(&fakeExecutor{})
	_, err := c.ReadWords(context.Background(), "N7:0/2", 3)
	if err == nil {
		t.Fatal("expected error for bit suffix")
	}
}

// ---------------------------------------------------------------------------
// Write / WriteAddress
// ---------------------------------------------------------------------------

func TestWriteAddressInteger(t *testing.T) {
	exec := &fakeExecutor{}
	c := NewClient(exec)
	if err := c.WriteAddress(context.Background(), "N7:0", int16(42)); err != nil {
		t.Fatalf("WriteAddress: %v", err)
	}
	req := exec.sent[0]
	// FNC=0xAA, file=7, type=N, elem=0, sub=0, then 2 data bytes
	if req[4] != FuncProtectedTypedLogicalWrite {
		t.Errorf("FNC: got 0x%02X", req[4])
	}
	if req[5] != 7 || req[6] != byte(FileTypeInteger) || req[7] != 0 || req[8] != 0 {
		t.Errorf("addressing: % X", req[5:9])
	}
	if !bytes.Equal(req[9:11], []byte{0x2A, 0x00}) {
		t.Errorf("data: got % X want 2A 00", req[9:11])
	}
}

func TestWriteAddressFloat(t *testing.T) {
	exec := &fakeExecutor{}
	c := NewClient(exec)
	if err := c.WriteAddress(context.Background(), "F8:5", float32(2.5)); err != nil {
		t.Fatalf("WriteAddress: %v", err)
	}
	req := exec.sent[0]
	want := math.Float32bits(2.5)
	got := binary.LittleEndian.Uint32(req[9:13])
	if got != want {
		t.Errorf("data: got 0x%08X want 0x%08X", got, want)
	}
}

func TestWriteAddressBitReadModifyWrite(t *testing.T) {
	// Initial word = 0x0001 (only bit 0 set). Writing /2 = true should
	// produce a follow-up write of 0x0005.
	step := 0
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			defer func() { step++ }()
			hdr := []byte{req[0] | ReplyBit, 0x00, req[2], req[3]}
			if step == 0 {
				// expect a read
				if req[4] != FuncProtectedTypedLogicalRead {
					return nil, errors.New("expected read first")
				}
				return append(hdr, 0x01, 0x00), nil
			}
			// expect a write
			if req[4] != FuncProtectedTypedLogicalWrite {
				return nil, errors.New("expected write second")
			}
			return hdr, nil
		},
	}
	c := NewClient(exec)
	if err := c.WriteAddress(context.Background(), "B3:0/2", true); err != nil {
		t.Fatalf("WriteAddress: %v", err)
	}
	if len(exec.sent) != 2 {
		t.Fatalf("expected 2 frames (R+W), got %d", len(exec.sent))
	}
	writeReq := exec.sent[1]
	if !bytes.Equal(writeReq[9:11], []byte{0x05, 0x00}) {
		t.Errorf("rmw payload: got % X want 05 00", writeReq[9:11])
	}
}

func TestWriteAddressBitClear(t *testing.T) {
	// Initial word = 0xFFFF. Writing /3 = false should produce 0xFFF7.
	step := 0
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			defer func() { step++ }()
			hdr := []byte{req[0] | ReplyBit, 0x00, req[2], req[3]}
			if step == 0 {
				return append(hdr, 0xFF, 0xFF), nil
			}
			return hdr, nil
		},
	}
	c := NewClient(exec)
	if err := c.WriteAddress(context.Background(), "N7:5/3", false); err != nil {
		t.Fatalf("WriteAddress: %v", err)
	}
	writeReq := exec.sent[1]
	if !bytes.Equal(writeReq[9:11], []byte{0xF7, 0xFF}) {
		t.Errorf("rmw payload: got % X want F7 FF", writeReq[9:11])
	}
}

// ---------------------------------------------------------------------------
// plc.Reader / plc.Writer via File DataPoint
// ---------------------------------------------------------------------------

func TestPLCReadWithFile(t *testing.T) {
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			return []byte{req[0] | ReplyBit, 0x00, req[2], req[3], 0x07, 0x00}, nil
		},
	}
	c := NewClient(exec)
	vals, err := c.Read(context.Background(), File{Address: "N7:1"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("len(vals): got %d want 1", len(vals))
	}
	got, _ := vals[0].Int()
	if got != 7 {
		t.Errorf("value: got %d want 7", got)
	}
	if vals[0].DataPoint == nil {
		t.Error("DataPoint should be populated")
	}
}

func TestPLCWriteWithFile(t *testing.T) {
	exec := &fakeExecutor{}
	c := NewClient(exec)
	if err := c.Write(context.Background(), File{Address: "N7:0"}, []byte{0x09, 0x00}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	req := exec.sent[0]
	if !bytes.Equal(req[9:11], []byte{0x09, 0x00}) {
		t.Errorf("data: got % X want 09 00", req[9:11])
	}
}

func TestPLCReadRejectsNonFileDataPoint(t *testing.T) {
	type otherDP struct{ s string }
	o := otherDP{s: "x"}
	// Implement DataPoint interface inline via a wrapper type that has
	// a String() method.
	_ = o
	c := NewClient(&fakeExecutor{})
	_, err := c.Read(context.Background(), notFile{})
	if err == nil {
		t.Fatal("expected error for non-File DataPoint")
	}
}

type notFile struct{}

func (notFile) String() string { return "notFile" }

// ---------------------------------------------------------------------------
// Lifecycle delegation
// ---------------------------------------------------------------------------

type lifecycleExec struct {
	fakeExecutor
	connected   bool
	connectErr  error
}

func (l *lifecycleExec) Connect(_ context.Context) error {
	l.connectCalls++
	if l.connectErr == nil {
		l.connected = true
	}
	return l.connectErr
}
func (l *lifecycleExec) Disconnect(_ context.Context) error {
	l.disconnect++
	l.connected = false
	return nil
}
func (l *lifecycleExec) IsConnected() bool { return l.connected }

func TestLifecycleDelegation(t *testing.T) {
	exec := &lifecycleExec{}
	c := NewClient(exec)
	if c.IsConnected() {
		t.Error("should not be connected initially")
	}
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if exec.connectCalls != 1 {
		t.Errorf("connectCalls: got %d want 1", exec.connectCalls)
	}
	if !c.IsConnected() {
		t.Error("should be connected after Connect")
	}
	if err := c.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if c.IsConnected() {
		t.Error("should not be connected after Disconnect")
	}
}

func TestNoLifecycleNoOps(t *testing.T) {
	// fakeExecutor doesn't implement Connector — lifecycle methods should
	// succeed and IsConnected should be true.
	c := NewClient(&fakeExecutor{})
	if err := c.Connect(context.Background()); err != nil {
		t.Errorf("Connect: %v", err)
	}
	if !c.IsConnected() {
		t.Error("IsConnected should be true when no lifecycle is provided")
	}
	if err := c.Disconnect(context.Background()); err != nil {
		t.Errorf("Disconnect: %v", err)
	}
}

// ---------------------------------------------------------------------------
// monitor integration
// ---------------------------------------------------------------------------

func TestMonitorIntegration(t *testing.T) {
	// Stub returns 11, 12, 13, ... incrementing for every successive read.
	var ctr int16
	exec := &fakeExecutor{
		replyFunc: func(req []byte) ([]byte, error) {
			ctr++
			return []byte{req[0] | ReplyBit, 0x00, req[2], req[3], byte(uint16(ctr)), byte(uint16(ctr) >> 8)}, nil
		},
	}
	c := NewClient(exec)

	mon, err := monitor.NewMonitor(c)
	if err != nil {
		t.Fatalf("NewMonitor: %v", err)
	}
	defer mon.Close()

	sub, err := mon.NewSubscriber(8)
	if err != nil {
		t.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Done()

	_, err = mon.Subscribe(File{Address: "N7:0"}, monitor.WithFrequency(10*time.Millisecond))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Collect 3 events.
	got := 0
	deadline := time.After(2 * time.Second)
	for got < 3 {
		select {
		case ev := <-sub.Events():
			if ev.Err != nil {
				t.Fatalf("event err: %v", ev.Err)
			}
			n, err := ev.Snapshot.Value.Int()
			if err != nil {
				t.Fatalf("value.Int: %v", err)
			}
			if n <= 0 {
				t.Errorf("expected positive value, got %d", n)
			}
			got++
		case <-deadline:
			t.Fatalf("only got %d events", got)
		}
	}
}
