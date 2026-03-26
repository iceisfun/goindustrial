package goindustrial_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/monitor"
	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/modbus"
	"github.com/iceisfun/goindustrial/transport"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// modbusConnectorFunc adapts a net.Conn into a transport.Connector[*modbus.TCPConn].
type modbusConnectorFunc struct {
	conn net.Conn
	used bool
	mu   sync.Mutex
}

func (f *modbusConnectorFunc) Connect(ctx context.Context) (*modbus.TCPConn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := modbus.NewTCPConn("", modbus.WithConn(f.conn))
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

type modbusCloserFunc struct{}

func (modbusCloserFunc) Close(conn *modbus.TCPConn) error {
	return conn.Disconnect(context.Background())
}

// mockTagObject implements cip.Object and responds to ReadTag requests.
type mockTagObject struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func (o *mockTagObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	if service != cip.ServiceReadTag {
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
	// For class-routed paths, the router strips the class segment.
	// Remaining path has instance segment. We return the first entry in data.
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, resp := range o.data {
		return resp, nil
	}
	return nil, cip.Error{Status: cip.StatusObjectDoesNotExist}
}

func marshalTagResponse(dt cip.DataType, value any) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint16(dt))
	binary.Write(buf, binary.LittleEndian, value)
	return buf.Bytes()
}

// multiReader implements plc.Reader by dispatching to the appropriate protocol.
type multiReader struct {
	modbus *modbus.Client
	eip    *ethernetip.Session
}

func (r *multiReader) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	results := make([]plc.Value, len(points))
	for i, p := range points {
		switch dp := p.(type) {
		case modbus.HoldingRegister:
			vals, err := r.modbus.ReadHoldingRegisters(ctx, dp.Addr, dp.Qty)
			if err != nil {
				return nil, err
			}
			raw := make([]byte, len(vals)*2)
			for j, v := range vals {
				binary.BigEndian.PutUint16(raw[j*2:], v)
			}
			results[i] = plc.Value{DataPoint: p, Raw: raw}

		case modbus.Coil:
			vals, err := r.modbus.ReadCoils(ctx, dp.Addr, dp.Qty)
			if err != nil {
				return nil, err
			}
			raw := make([]byte, len(vals))
			for j, v := range vals {
				if v {
					raw[j] = 1
				}
			}
			results[i] = plc.Value{DataPoint: p, Raw: raw}

		case ethernetip.Tag:
			// Use class-based path for CIP routing compatibility.
			// Class 0x04 (Assembly), Instance 1, with ReadTag service.
			req := &cip.MessageRouterRequest{
				Service:     cip.ServiceReadTag,
				RequestPath: cip.Path([]byte{0x20, 0x04, 0x24, 0x01}),
				RequestData: []byte{byte(dp.Elements), byte(dp.Elements >> 8)},
			}
			resp, err := r.eip.SendCIPRequest(ctx, req)
			if err != nil {
				return nil, err
			}
			if err := resp.Error(); err != nil {
				return nil, err
			}
			results[i] = plc.Value{DataPoint: p, Raw: resp.ResponseData}
		}
	}
	return results, nil
}

// pipeListener implements net.Listener, yielding one conn then blocking.
type pipeListener struct {
	conn   net.Conn
	once   sync.Once
	closed chan struct{}
}

func newPipeListener(conn net.Conn) *pipeListener {
	return &pipeListener{conn: conn, closed: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.conn })
	if c != nil {
		return c, nil
	}
	<-l.closed
	return nil, io.ErrClosedPipe
}

func (l *pipeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCrossProtocolMonitor verifies that a single Monitor instance can poll
// both Modbus and EtherNet/IP data points, receiving events on a shared channel.
func TestCrossProtocolMonitor(t *testing.T) {
	ctx := context.Background()

	// --- Modbus side ---
	modbusServerConn, modbusClientConn := net.Pipe()

	modbusStore := modbus.NewMemoryStore()
	modbusStore.SetHoldingRegister(100, 0x1234)
	modbusStore.SetHoldingRegister(101, 0x5678)

	modbusSrv := modbus.NewServer("",
		modbus.WithServerConn(modbusServerConn),
		modbus.WithServerDataStore(modbusStore),
	)
	modbusSrv.Start(ctx)
	defer modbusSrv.Stop(ctx)

	connector := &modbusConnectorFunc{conn: modbusClientConn}
	tp := transport.NewReconnectingTransport[*modbus.TCPConn](connector, modbusCloserFunc{})
	modbusClient := modbus.NewClient(tp)
	defer modbusClient.Close()

	// --- EtherNet/IP side ---
	eipServerConn, eipClientConn := net.Pipe()

	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassAssembly, &mockTagObject{
		data: map[string][]byte{
			"TestTag": marshalTagResponse(cip.TypeDINT, int32(42)),
		},
	})

	eipSrv := ethernetip.NewServer(router)
	go eipSrv.HandleConn(eipServerConn)

	eipConn, err := ethernetip.NewTCPConn("", ethernetip.WithConn(eipClientConn))
	if err != nil {
		t.Fatalf("eip conn: %v", err)
	}
	eipSession := ethernetip.NewSession(eipConn, nil)
	if err := eipSession.Register(ctx); err != nil {
		t.Fatalf("eip register: %v", err)
	}
	defer eipSession.Close()

	// --- Monitor ---
	multi := &multiReader{modbus: modbusClient, eip: eipSession}
	m, err := monitor.NewMonitor(multi, monitor.WithEventBuffer(32))
	if err != nil {
		t.Fatalf("monitor: %v", err)
	}
	defer m.Close()

	modbusSub, err := m.Subscribe(
		modbus.HoldingRegister{Addr: 100, Qty: 2},
		monitor.WithFrequency(50*time.Millisecond),
		monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
	)
	if err != nil {
		t.Fatalf("subscribe modbus: %v", err)
	}

	eipSub, err := m.Subscribe(
		ethernetip.Tag{Name: "TestTag", Elements: 1},
		monitor.WithFrequency(50*time.Millisecond),
		monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
	)
	if err != nil {
		t.Fatalf("subscribe eip: %v", err)
	}

	sawModbus := false
	sawEIP := false
	timeout := time.After(5 * time.Second)

	for !sawModbus || !sawEIP {
		select {
		case evt := <-m.Events():
			if evt.Err != nil {
				t.Logf("event error (sub %d): %v", evt.SubscriptionID, evt.Err)
				continue
			}
			if evt.SubscriptionID == modbusSub.ID() {
				sawModbus = true
				if len(evt.Snapshot.Value.Raw) != 4 {
					t.Errorf("modbus: expected 4 bytes, got %d", len(evt.Snapshot.Value.Raw))
				} else {
					r0 := binary.BigEndian.Uint16(evt.Snapshot.Value.Raw[0:2])
					r1 := binary.BigEndian.Uint16(evt.Snapshot.Value.Raw[2:4])
					if r0 != 0x1234 || r1 != 0x5678 {
						t.Errorf("modbus: expected [0x1234, 0x5678], got [0x%04X, 0x%04X]", r0, r1)
					}
				}
			}
			if evt.SubscriptionID == eipSub.ID() {
				sawEIP = true
				if len(evt.Snapshot.Value.Raw) < 6 {
					t.Errorf("eip: expected at least 6 bytes, got %d", len(evt.Snapshot.Value.Raw))
				} else {
					val := int32(binary.LittleEndian.Uint32(evt.Snapshot.Value.Raw[2:6]))
					if val != 42 {
						t.Errorf("eip: expected 42, got %d", val)
					}
				}
			}
		case <-timeout:
			t.Fatalf("timeout: sawModbus=%v sawEIP=%v", sawModbus, sawEIP)
		}
	}
}

// TestModbusClientPLCInterface verifies the Modbus Client satisfies plc.PLC
// via its Read method dispatching on DataPoint types.
func TestModbusClientPLCInterface(t *testing.T) {
	ctx := context.Background()

	serverConn, clientConn := net.Pipe()
	store := modbus.NewMemoryStore()
	store.SetHoldingRegister(0, 100)
	store.SetCoil(0, true)

	srv := modbus.NewServer("",
		modbus.WithServerConn(serverConn),
		modbus.WithServerDataStore(store),
	)
	srv.Start(ctx)
	defer srv.Stop(ctx)

	connector := &modbusConnectorFunc{conn: clientConn}
	tp := transport.NewReconnectingTransport[*modbus.TCPConn](connector, modbusCloserFunc{})
	client := modbus.NewClient(tp)
	defer client.Close()

	// Test via plc.Reader interface
	var reader plc.Reader = client
	vals, err := reader.Read(ctx,
		modbus.HoldingRegister{Addr: 0, Qty: 1},
		modbus.Coil{Addr: 0, Qty: 1},
	)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
	if len(vals[0].Raw) != 2 {
		t.Errorf("expected 2 bytes for register, got %d", len(vals[0].Raw))
	}
	if len(vals[1].Raw) != 1 || vals[1].Raw[0] != 1 {
		t.Errorf("expected coil true [0x01], got %v", vals[1].Raw)
	}
}

// TestEIPClientPLCInterface verifies the EtherNet/IP path satisfies plc.Reader.
func TestEIPClientPLCInterface(t *testing.T) {
	ctx := context.Background()

	serverConn, clientConn := net.Pipe()

	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassAssembly, &mockTagObject{
		data: map[string][]byte{
			"MyDINT": marshalTagResponse(cip.TypeDINT, int32(99)),
		},
	})

	eipSrv := ethernetip.NewServer(router)
	go eipSrv.HandleConn(serverConn)

	tcpConn, err := ethernetip.NewTCPConn("", ethernetip.WithConn(clientConn))
	if err != nil {
		t.Fatal(err)
	}
	sess := ethernetip.NewSession(tcpConn, nil)
	if err := sess.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer sess.Close()

	reader := &multiReader{eip: sess}
	vals, err := reader.Read(ctx, ethernetip.Tag{Name: "MyDINT", Elements: 1})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if len(vals[0].Raw) >= 6 {
		val := int32(binary.LittleEndian.Uint32(vals[0].Raw[2:6]))
		if val != 99 {
			t.Errorf("expected 99, got %d", val)
		}
	} else {
		t.Errorf("expected at least 6 bytes, got %d", len(vals[0].Raw))
	}
}
