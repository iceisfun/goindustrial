package ethernetip

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/pccc"
)

// ---------------------------------------------------------------------------
// Test plumbing
// ---------------------------------------------------------------------------

// staticTransport is a transport.Transport[*Session] that always returns a
// pre-built session. It is the minimum scaffolding needed to drive a
// (*Client) over a net.Pipe-backed Session in tests.
type staticTransport struct {
	sess *Session
}

func (s *staticTransport) Conn(_ context.Context) (*Session, error) { return s.sess, nil }
func (s *staticTransport) Reset(_ *Session) error                   { return nil }
func (s *staticTransport) Close() error                             { return s.sess.Close() }
func (s *staticTransport) Peek() bool                               { return true }

// pcccStubObject is a minimal CIP object that handles Execute_PCCC (0x4B).
// It verifies the requestor-ID header layout, records the inner PCCC bytes,
// and returns a reply assembled as `requestor ID echo || replyPCCC`.
type pcccStubObject struct {
	t              *testing.T
	wantVendor     uint16
	wantSerial     uint32
	lastPCCC       []byte
	replyPCCC      []byte
	returnCIPError bool
}

func (o *pcccStubObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	if service != cip.ServiceExecutePCCC {
		o.t.Errorf("unexpected service 0x%02X (want Execute_PCCC 0x4B)", service)
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
	if len(data) < 7 {
		o.t.Errorf("Execute_PCCC payload too short: %d bytes", len(data))
		return nil, cip.Error{Status: cip.StatusNotEnoughData}
	}
	if data[0] != 0x07 {
		o.t.Errorf("requestor-ID length: got 0x%02X want 0x07", data[0])
	}
	vendor := uint16(data[1]) | uint16(data[2])<<8
	serial := uint32(data[3]) | uint32(data[4])<<8 | uint32(data[5])<<16 | uint32(data[6])<<24
	if vendor != o.wantVendor {
		o.t.Errorf("vendor: got 0x%04X want 0x%04X", vendor, o.wantVendor)
	}
	if serial != o.wantSerial {
		o.t.Errorf("serial: got 0x%08X want 0x%08X", serial, o.wantSerial)
	}
	o.lastPCCC = append([]byte(nil), data[7:]...)

	if o.returnCIPError {
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
	out := make([]byte, 7+len(o.replyPCCC))
	copy(out, data[:7])
	copy(out[7:], o.replyPCCC)
	return out, nil
}

// newPCCCStubClient wires up a Client whose transport is backed by an
// in-process EtherNet/IP server with the given PCCC object registered.
func newPCCCStubClient(t *testing.T, obj cip.Object, opts ...ClientOption) *Client {
	t.Helper()
	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassPCCC, obj)
	_, clientConn := setupPipePair(t, router)

	tc, err := NewTCPConn("", WithConn(clientConn))
	if err != nil {
		t.Fatalf("NewTCPConn: %v", err)
	}
	sess := NewSession(tc, nil)
	if err := sess.Register(context.Background()); err != nil {
		t.Fatalf("Session.Register: %v", err)
	}

	c := &Client{
		transport:       &staticTransport{sess: sess},
		logger:          logging.NewNopLogger(),
		cipVendorID:     DefaultCIPVendorID,
		cipSerialNumber: DefaultCIPSerialNumber,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Encode a typed-logical read with the pccc package, send via ExecutePCCC,
// confirm the PCCC bytes are propagated back unchanged (with the 7-byte
// requestor ID stripped) and the inner request bytes match what was sent.
func TestExecutePCCCReadRoundTrip(t *testing.T) {
	pcccReq, err := pccc.EncodeTypedRead(0x1234, 2, 7, pccc.FileTypeInteger, 0, 0)
	if err != nil {
		t.Fatalf("EncodeTypedRead: %v", err)
	}
	stub := &pcccStubObject{
		t:          t,
		wantVendor: 0x1234,
		wantSerial: 0xCAFEF00D,
		replyPCCC:  []byte{pcccReq[0] | pccc.ReplyBit, 0x00, 0x34, 0x12, 0x2A, 0x00},
	}

	c := newPCCCStubClient(t, stub,
		WithCIPVendorID(0x1234),
		WithCIPSerialNumber(0xCAFEF00D),
	)
	defer c.Close()

	got, err := c.ExecutePCCC(context.Background(), pcccReq)
	if err != nil {
		t.Fatalf("ExecutePCCC: %v", err)
	}
	if !bytes.Equal(got, stub.replyPCCC) {
		t.Fatalf("returned PCCC bytes mismatch\n got: % X\nwant: % X", got, stub.replyPCCC)
	}
	if !bytes.Equal(stub.lastPCCC, pcccReq) {
		t.Fatalf("PCCC bytes received by stub mismatch\n got: % X\nwant: % X", stub.lastPCCC, pcccReq)
	}
}

// pccc.DecodeReply on a STS-error reply must surface as *pccc.Error, and
// ExecutePCCC itself must NOT report a transport-layer failure (the CIP
// layer succeeded; only the PCCC payload contains an error).
func TestExecutePCCCSTSErrorSurfaces(t *testing.T) {
	pcccReq, _ := pccc.EncodeTypedRead(0x0001, 2, 7, pccc.FileTypeInteger, 0, 0)
	stub := &pcccStubObject{
		t:          t,
		wantVendor: DefaultCIPVendorID,
		wantSerial: DefaultCIPSerialNumber,
		replyPCCC:  []byte{pcccReq[0] | pccc.ReplyBit, 0x10, 0x01, 0x00},
	}

	c := newPCCCStubClient(t, stub)
	defer c.Close()

	raw, err := c.ExecutePCCC(context.Background(), pcccReq)
	if err != nil {
		t.Fatalf("ExecutePCCC at CIP layer: %v", err)
	}
	if _, err = pccc.DecodeReply(raw); err == nil {
		t.Fatal("expected PCCC STS error, got nil")
	}
	var pe *pccc.Error
	if !errors.As(err, &pe) {
		t.Fatalf("expected *pccc.Error, got %T: %v", err, err)
	}
	if pe.STS != 0x10 {
		t.Errorf("STS: got 0x%02X want 0x10", pe.STS)
	}
}

// A non-zero CIP general status must propagate as a cip.Error from
// ExecutePCCC. The Client wraps it in an internal cipError type to skip
// retries; errors.As must still unwrap to the underlying cip.Error.
func TestExecutePCCCCIPErrorSurfaces(t *testing.T) {
	pcccReq, _ := pccc.EncodeTypedRead(0x0001, 2, 7, pccc.FileTypeInteger, 0, 0)
	stub := &pcccStubObject{
		t:              t,
		wantVendor:     DefaultCIPVendorID,
		wantSerial:     DefaultCIPSerialNumber,
		returnCIPError: true,
	}

	c := newPCCCStubClient(t, stub)
	defer c.Close()

	_, err := c.ExecutePCCC(context.Background(), pcccReq)
	if err == nil {
		t.Fatal("expected CIP error, got nil")
	}
	var cipErr cip.Error
	if !errors.As(err, &cipErr) {
		t.Fatalf("expected cip.Error, got %T: %v", err, err)
	}
	if cipErr.Status != cip.StatusServiceNotSupported {
		t.Errorf("CIP status: got 0x%02X want 0x%02X",
			cipErr.Status, cip.StatusServiceNotSupported)
	}
}

// Without WithCIPVendorID/WithCIPSerialNumber, the client must populate the
// requestor-ID header with the documented defaults.
func TestExecutePCCCDefaultRequestorID(t *testing.T) {
	pcccReq, _ := pccc.EncodeTypedRead(0x0001, 2, 7, pccc.FileTypeInteger, 0, 0)
	stub := &pcccStubObject{
		t:          t,
		wantVendor: DefaultCIPVendorID,
		wantSerial: DefaultCIPSerialNumber,
		replyPCCC:  []byte{pcccReq[0] | pccc.ReplyBit, 0x00, 0x01, 0x00, 0x00, 0x00},
	}

	c := newPCCCStubClient(t, stub)
	defer c.Close()

	if _, err := c.ExecutePCCC(context.Background(), pcccReq); err != nil {
		t.Fatalf("ExecutePCCC: %v", err)
	}
}

// A reply too short to contain the 7-byte requestor-ID echo must error
// cleanly rather than panicking.
func TestExecutePCCCRejectsTruncatedRequestorID(t *testing.T) {
	pcccReq, _ := pccc.EncodeTypedRead(0x0001, 2, 7, pccc.FileTypeInteger, 0, 0)
	c := newPCCCStubClient(t, shortReplyObject{})
	defer c.Close()

	_, err := c.ExecutePCCC(context.Background(), pcccReq)
	if err == nil {
		t.Fatal("expected error for truncated reply, got nil")
	}
}

type shortReplyObject struct{}

func (shortReplyObject) HandleRequest(_ cip.USINT, _ cip.Path, _ []byte) ([]byte, error) {
	return []byte{0x01, 0x02, 0x03}, nil
}
