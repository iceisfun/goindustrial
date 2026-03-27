package monitor

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

// ---------------------------------------------------------------------------
// countingReader — mock plc.Reader that tracks read calls
// ---------------------------------------------------------------------------

type countingReader struct {
	mu        sync.Mutex
	calls     []readCall
	callCount atomic.Int64
	dataFn    func(dp plc.DataPoint) plc.Value // generates return data
}

type readCall struct {
	points []plc.DataPoint
}

func newCountingReader(dataFn func(dp plc.DataPoint) plc.Value) *countingReader {
	return &countingReader{dataFn: dataFn}
}

func (r *countingReader) Read(_ context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	r.mu.Lock()
	r.calls = append(r.calls, readCall{points: points})
	r.mu.Unlock()
	r.callCount.Add(1)

	results := make([]plc.Value, len(points))
	for i, p := range points {
		results[i] = r.dataFn(p)
		results[i].DataPoint = p
	}
	return results, nil
}

func (r *countingReader) count() int64 {
	return r.callCount.Load()
}

// registerDataFn returns data where each register's value equals its address.
func registerDataFn(dp plc.DataPoint) plc.Value {
	switch p := dp.(type) {
	case modbus.HoldingRegister:
		raw := make([]byte, int(p.Qty)*2)
		for i := 0; i < int(p.Qty); i++ {
			binary.BigEndian.PutUint16(raw[i*2:], uint16(p.Addr)+uint16(i))
		}
		dt := plc.TypeUint16
		if p.Qty > 1 {
			dt = plc.TypeBytes
		}
		return plc.Value{Raw: raw, Type: dt, ByteOrder: plc.ByteOrderBigEndian}

	case modbus.InputRegister:
		raw := make([]byte, int(p.Qty)*2)
		for i := 0; i < int(p.Qty); i++ {
			binary.BigEndian.PutUint16(raw[i*2:], uint16(p.Addr)+uint16(i))
		}
		dt := plc.TypeUint16
		if p.Qty > 1 {
			dt = plc.TypeBytes
		}
		return plc.Value{Raw: raw, Type: dt, ByteOrder: plc.ByteOrderBigEndian}

	case modbus.Coil:
		raw := make([]byte, (int(p.Qty)+7)/8)
		for i := 0; i < int(p.Qty); i++ {
			addr := uint16(p.Addr) + uint16(i)
			if addr%2 == 0 { // even addresses are ON
				raw[i/8] |= 1 << uint(i%8)
			}
		}
		return plc.Value{Raw: raw, Type: plc.TypeBool, ByteOrder: plc.ByteOrderBigEndian}

	case modbus.DiscreteInput:
		raw := make([]byte, (int(p.Qty)+7)/8)
		for i := 0; i < int(p.Qty); i++ {
			addr := uint16(p.Addr) + uint16(i)
			if addr%2 == 0 {
				raw[i/8] |= 1 << uint(i%8)
			}
		}
		return plc.Value{Raw: raw, Type: plc.TypeBool, ByteOrder: plc.ByteOrderBigEndian}
	}
	return plc.Value{}
}

// ===========================================================================
// Cluster Plan Tests
// ===========================================================================

func TestBuildWindowsClustered(t *testing.T) {
	// Addresses: 1, 25, 26, 27, 30, 33, 34
	// With gap threshold 32, all merge into one window: [1, 34]
	points := []Clusterable{
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 27, Qty: 1},
		modbus.HoldingRegister{Addr: 30, Qty: 1},
		modbus.HoldingRegister{Addr: 33, Qty: 1},
		modbus.HoldingRegister{Addr: 34, Qty: 1},
	}

	cfg := ClusterConfig{GapThreshold: 32, MaxRegistersPerRead: 120, MaxCoilsPerRead: 2000}
	windows := buildWindows(points, cfg)

	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.start != 1 || w.count != 34 {
		t.Errorf("window = [%d, %d), want [1, 35)", w.start, w.start+w.count)
	}
	if len(w.points) != 7 {
		t.Errorf("window has %d points, want 7", len(w.points))
	}
}

func TestBuildWindowsSparse(t *testing.T) {
	// Same as above but with outlier at 488.
	// Gap 488-35 = 453 > 32, so two windows.
	points := []Clusterable{
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 27, Qty: 1},
		modbus.HoldingRegister{Addr: 30, Qty: 1},
		modbus.HoldingRegister{Addr: 33, Qty: 1},
		modbus.HoldingRegister{Addr: 34, Qty: 1},
		modbus.HoldingRegister{Addr: 488, Qty: 1},
	}

	cfg := ClusterConfig{GapThreshold: 32, MaxRegistersPerRead: 120, MaxCoilsPerRead: 2000}
	windows := buildWindows(points, cfg)

	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}
}

func TestBuildWindowsGapThreshold(t *testing.T) {
	points := []Clusterable{
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 50, Qty: 1},
	}

	// Gap = 50 - 11 = 39
	// With threshold 32: 39 > 32 → 2 windows
	cfg := ClusterConfig{GapThreshold: 32, MaxRegistersPerRead: 120, MaxCoilsPerRead: 2000}
	w := buildWindows(points, cfg)
	if len(w) != 2 {
		t.Errorf("gap=32: expected 2 windows, got %d", len(w))
	}

	// With threshold 40: 39 <= 40 → 1 window
	cfg.GapThreshold = 40
	w = buildWindows(points, cfg)
	if len(w) != 1 {
		t.Errorf("gap=40: expected 1 window, got %d", len(w))
	}

	// Exactly at threshold: 39 == 39 → 1 window (<=)
	cfg.GapThreshold = 39
	w = buildWindows(points, cfg)
	if len(w) != 1 {
		t.Errorf("gap=39: expected 1 window, got %d", len(w))
	}
}

func TestBuildWindowsMaxBlockSize(t *testing.T) {
	// 200 contiguous registers, max 120 → 2 windows.
	points := make([]Clusterable, 200)
	for i := range points {
		points[i] = modbus.HoldingRegister{Addr: modbus.Address(i), Qty: 1}
	}

	cfg := ClusterConfig{GapThreshold: 32, MaxRegistersPerRead: 120, MaxCoilsPerRead: 2000}
	windows := buildWindows(points, cfg)

	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}

	total := int(windows[0].count) + int(windows[1].count)
	if total != 200 {
		t.Errorf("total coverage = %d, want 200", total)
	}
	if windows[0].count > 120 || windows[1].count > 120 {
		t.Errorf("window exceeds max: %d, %d", windows[0].count, windows[1].count)
	}
}

func TestBuildWindowsMixedTypes(t *testing.T) {
	points := []Clusterable{
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 11, Qty: 1},
		modbus.InputRegister{Addr: 10, Qty: 1},
		modbus.InputRegister{Addr: 11, Qty: 1},
		modbus.Coil{Addr: 100, Qty: 1},
	}

	cfg := ClusterConfig{GapThreshold: 32, MaxRegistersPerRead: 120, MaxCoilsPerRead: 2000}
	windows := buildWindows(points, cfg)

	// 3 types → 3 windows (HR, IR, Coil grouped separately).
	if len(windows) != 3 {
		t.Fatalf("expected 3 windows, got %d", len(windows))
	}
}

// ===========================================================================
// ClusteringReader Tests
// ===========================================================================

func TestClusteredReadReducesRequests(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	// Register 7 adjacent holding registers.
	cr.Register(
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 27, Qty: 1},
		modbus.HoldingRegister{Addr: 30, Qty: 1},
		modbus.HoldingRegister{Addr: 33, Qty: 1},
		modbus.HoldingRegister{Addr: 34, Qty: 1},
	)

	ctx := context.Background()

	// Read each point individually — all should hit the same cluster.
	points := []plc.DataPoint{
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 27, Qty: 1},
		modbus.HoldingRegister{Addr: 30, Qty: 1},
		modbus.HoldingRegister{Addr: 33, Qty: 1},
		modbus.HoldingRegister{Addr: 34, Qty: 1},
	}

	// Read all 7 in one call.
	vals, err := cr.Read(ctx, points...)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Should be 1 inner read (one window covers all 7).
	if got := inner.count(); got != 1 {
		t.Errorf("inner reads = %d, want 1", got)
	}

	// Verify extracted values.
	if len(vals) != 7 {
		t.Fatalf("got %d values, want 7", len(vals))
	}

	expected := []uint16{1, 25, 26, 27, 30, 33, 34}
	for i, exp := range expected {
		if len(vals[i].Raw) != 2 {
			t.Errorf("val[%d] raw len = %d, want 2", i, len(vals[i].Raw))
			continue
		}
		got := binary.BigEndian.Uint16(vals[i].Raw)
		if got != exp {
			t.Errorf("val[%d] = %d, want %d", i, got, exp)
		}
	}
}

func TestSparseAddressesMultipleClusters(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	cr.Register(
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 488, Qty: 1},
	)

	ctx := context.Background()
	vals, err := cr.Read(ctx,
		modbus.HoldingRegister{Addr: 1, Qty: 1},
		modbus.HoldingRegister{Addr: 25, Qty: 1},
		modbus.HoldingRegister{Addr: 26, Qty: 1},
		modbus.HoldingRegister{Addr: 488, Qty: 1},
	)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// 2 clusters: [1-26] and [488].
	if got := inner.count(); got != 2 {
		t.Errorf("inner reads = %d, want 2", got)
	}

	// Verify outlier value.
	if binary.BigEndian.Uint16(vals[3].Raw) != 488 {
		t.Errorf("outlier = %d, want 488", binary.BigEndian.Uint16(vals[3].Raw))
	}
}

func TestDisabledPassthrough(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner, WithClusteringEnabled(false))

	cr.Register(
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 11, Qty: 1},
		modbus.HoldingRegister{Addr: 12, Qty: 1},
	)

	ctx := context.Background()
	vals, err := cr.Read(ctx,
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 11, Qty: 1},
		modbus.HoldingRegister{Addr: 12, Qty: 1},
	)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Disabled: passes through directly, 1 call with all 3 points.
	if got := cr.ReadCount.Load(); got != 3 {
		t.Errorf("read count = %d, want 3", got)
	}

	if len(vals) != 3 {
		t.Fatalf("got %d values, want 3", len(vals))
	}
}

func TestCacheTTL(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner, WithCacheTTL(200*time.Millisecond))

	cr.Register(modbus.HoldingRegister{Addr: 10, Qty: 1})

	ctx := context.Background()

	// First read: cache miss → inner read.
	_, err := cr.Read(ctx, modbus.HoldingRegister{Addr: 10, Qty: 1})
	if err != nil {
		t.Fatal(err)
	}
	if inner.count() != 1 {
		t.Fatalf("after first read: inner = %d, want 1", inner.count())
	}

	// Second read within TTL: cache hit.
	_, err = cr.Read(ctx, modbus.HoldingRegister{Addr: 10, Qty: 1})
	if err != nil {
		t.Fatal(err)
	}
	if inner.count() != 1 {
		t.Errorf("within TTL: inner = %d, want 1", inner.count())
	}

	// Wait for cache to expire.
	time.Sleep(250 * time.Millisecond)

	// Third read: cache miss → inner read.
	_, err = cr.Read(ctx, modbus.HoldingRegister{Addr: 10, Qty: 1})
	if err != nil {
		t.Fatal(err)
	}
	if inner.count() != 2 {
		t.Errorf("after TTL: inner = %d, want 2", inner.count())
	}
}

func TestUnregisteredFallback(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	// Register only addr 10.
	cr.Register(modbus.HoldingRegister{Addr: 10, Qty: 1})

	ctx := context.Background()

	// Read addr 10 (clustered) and addr 999 (unregistered).
	vals, err := cr.Read(ctx,
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 999, Qty: 1},
	)
	if err != nil {
		t.Fatal(err)
	}

	// 2 inner reads: 1 for window, 1 for unregistered fallback.
	if inner.count() != 2 {
		t.Errorf("inner = %d, want 2", inner.count())
	}

	if binary.BigEndian.Uint16(vals[0].Raw) != 10 {
		t.Errorf("val[0] = %d, want 10", binary.BigEndian.Uint16(vals[0].Raw))
	}
	if binary.BigEndian.Uint16(vals[1].Raw) != 999 {
		t.Errorf("val[1] = %d, want 999", binary.BigEndian.Uint16(vals[1].Raw))
	}
}

func TestCoilBitExtraction(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	cr.Register(
		modbus.Coil{Addr: 100, Qty: 1},
		modbus.Coil{Addr: 107, Qty: 1},
	)

	ctx := context.Background()
	vals, err := cr.Read(ctx,
		modbus.Coil{Addr: 100, Qty: 1},
		modbus.Coil{Addr: 107, Qty: 1},
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1 inner read (both coils in one window).
	if inner.count() != 1 {
		t.Errorf("inner = %d, want 1", inner.count())
	}

	// Addr 100 is even → ON. Addr 107 is odd → OFF.
	if len(vals[0].Raw) != 1 {
		t.Fatalf("coil 100 raw len = %d, want 1", len(vals[0].Raw))
	}
	if vals[0].Raw[0]&1 != 1 {
		t.Error("coil 100 should be ON (even address)")
	}
	if vals[1].Raw[0]&1 != 0 {
		t.Error("coil 107 should be OFF (odd address)")
	}
}

func TestCoilMultipleBitsExtraction(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	// Register two ranges of coils.
	cr.Register(
		modbus.Coil{Addr: 100, Qty: 4},
		modbus.Coil{Addr: 110, Qty: 4},
	)

	ctx := context.Background()
	vals, err := cr.Read(ctx,
		modbus.Coil{Addr: 100, Qty: 4},
		modbus.Coil{Addr: 110, Qty: 4},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Coils at even addresses are ON:
	// 100=ON, 101=OFF, 102=ON, 103=OFF → bits 0,2 set → 0x05
	if vals[0].Raw[0] != 0x05 {
		t.Errorf("coils 100-103 = 0x%02X, want 0x05", vals[0].Raw[0])
	}
	// 110=ON, 111=OFF, 112=ON, 113=OFF → 0x05
	if vals[1].Raw[0] != 0x05 {
		t.Errorf("coils 110-113 = 0x%02X, want 0x05", vals[1].Raw[0])
	}
}

func TestSingleflightDedup(t *testing.T) {
	var readDelay atomic.Bool
	readDelay.Store(true)

	inner := newCountingReader(func(dp plc.DataPoint) plc.Value {
		if readDelay.Load() {
			time.Sleep(50 * time.Millisecond)
		}
		return registerDataFn(dp)
	})

	cr := NewClusteringReader(inner)
	cr.Register(modbus.HoldingRegister{Addr: 10, Qty: 1})

	ctx := context.Background()

	// Launch 10 goroutines all reading the same point concurrently.
	var wg sync.WaitGroup
	errors := make([]error, 10)
	values := make([]plc.Value, 10)

	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vals, err := cr.Read(ctx, modbus.HoldingRegister{Addr: 10, Qty: 1})
			errors[i] = err
			if len(vals) > 0 {
				values[i] = vals[0]
			}
		}()
	}
	wg.Wait()

	// Singleflight: only 1 inner read despite 10 concurrent callers.
	if got := inner.count(); got != 1 {
		t.Errorf("inner reads = %d, want 1", got)
	}

	// All should have succeeded with correct value.
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
		if len(values[i].Raw) >= 2 {
			if binary.BigEndian.Uint16(values[i].Raw) != 10 {
				t.Errorf("goroutine %d value = %d, want 10", i, binary.BigEndian.Uint16(values[i].Raw))
			}
		}
	}
}

func TestMaxBlockSizeEnforced(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner, WithMaxRegistersPerRead(120))

	// Register 200 contiguous addresses.
	points := make([]plc.DataPoint, 200)
	for i := range 200 {
		p := modbus.HoldingRegister{Addr: modbus.Address(i), Qty: 1}
		cr.Register(p)
		points[i] = p
	}

	ctx := context.Background()
	vals, err := cr.Read(ctx, points...)
	if err != nil {
		t.Fatal(err)
	}

	// Should be 2 inner reads (120 + 80).
	if got := inner.count(); got != 2 {
		t.Errorf("inner reads = %d, want 2", got)
	}

	// Verify all values correct.
	for i, v := range vals {
		got := binary.BigEndian.Uint16(v.Raw)
		if got != uint16(i) {
			t.Errorf("val[%d] = %d, want %d", i, got, i)
		}
	}
}

func TestRegisterUnregister(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	p1 := modbus.HoldingRegister{Addr: 10, Qty: 1}
	p2 := modbus.HoldingRegister{Addr: 11, Qty: 1}

	cr.Register(p1, p2)

	// Should have 1 window covering both.
	cr.mu.RLock()
	if len(cr.windows) != 1 {
		t.Errorf("after register: %d windows, want 1", len(cr.windows))
	}
	cr.mu.RUnlock()

	// Unregister one point.
	cr.Unregister(p2)

	cr.mu.RLock()
	if len(cr.windows) != 1 {
		t.Errorf("after unregister: %d windows, want 1", len(cr.windows))
	}
	if len(cr.points) != 1 {
		t.Errorf("after unregister: %d points, want 1", len(cr.points))
	}
	cr.mu.RUnlock()
}

// ===========================================================================
// Baseline vs Optimized Comparison
// ===========================================================================

func TestBaselineVsOptimized(t *testing.T) {
	addresses := []modbus.Address{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}

	// Baseline: disabled clustering.
	baseInner := newCountingReader(registerDataFn)
	baseline := NewClusteringReader(baseInner, WithClusteringEnabled(false))
	for _, addr := range addresses {
		baseline.Register(modbus.HoldingRegister{Addr: addr, Qty: 1})
	}

	ctx := context.Background()
	points := make([]plc.DataPoint, len(addresses))
	for i, addr := range addresses {
		points[i] = modbus.HoldingRegister{Addr: addr, Qty: 1}
	}

	baseVals, err := baseline.Read(ctx, points...)
	if err != nil {
		t.Fatal(err)
	}

	// Optimized: clustering enabled.
	optInner := newCountingReader(registerDataFn)
	optimized := NewClusteringReader(optInner)
	for _, addr := range addresses {
		optimized.Register(modbus.HoldingRegister{Addr: addr, Qty: 1})
	}

	optVals, err := optimized.Read(ctx, points...)
	if err != nil {
		t.Fatal(err)
	}

	// Baseline: 10 reads (passthrough). Optimized: 1 read.
	baseCount := baseline.ReadCount.Load()
	optCount := optInner.count()

	if baseCount != 10 {
		t.Errorf("baseline reads = %d, want 10", baseCount)
	}
	if optCount != 1 {
		t.Errorf("optimized reads = %d, want 1", optCount)
	}

	// Values must be identical.
	for i := range baseVals {
		if binary.BigEndian.Uint16(baseVals[i].Raw) != binary.BigEndian.Uint16(optVals[i].Raw) {
			t.Errorf("value mismatch at %d: base=%d, opt=%d",
				i,
				binary.BigEndian.Uint16(baseVals[i].Raw),
				binary.BigEndian.Uint16(optVals[i].Raw))
		}
	}
}

// ===========================================================================
// Integration Test: Real Modbus Server on localhost
// ===========================================================================

func startTestModbusServer(t *testing.T, numRegisters int) (*modbus.Server, *net.TCPAddr, *modbus.MemoryStore) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	store := modbus.NewMemoryStore()
	for i := 0; i < numRegisters; i++ {
		store.SetHoldingRegister(modbus.Address(i), modbus.RegisterValue(i))
	}

	srv := modbus.NewServer(ln.Addr().String(),
		modbus.WithServerDataStore(store),
		modbus.WithServerListener(ln),
	)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("server start: %v", err)
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })

	return srv, ln.Addr().(*net.TCPAddr), store
}

func TestClusteringWithModbusServer(t *testing.T) {
	_, addr, _ := startTestModbusServer(t, 100)

	ctx := context.Background()
	client, err := modbus.Connect(ctx, addr.IP.String(),
		modbus.WithPort(addr.Port),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	cr := NewClusteringReader(client)

	// Register 10 adjacent addresses.
	for i := 10; i < 20; i++ {
		cr.Register(modbus.HoldingRegister{Addr: modbus.Address(i), Qty: 1})
	}

	points := make([]plc.DataPoint, 10)
	for i := range 10 {
		points[i] = modbus.HoldingRegister{Addr: modbus.Address(10 + i), Qty: 1}
	}

	vals, err := cr.Read(ctx, points...)
	if err != nil {
		t.Fatalf("clustered read: %v", err)
	}

	// Should be 1 inner read (one cluster).
	if got := cr.ReadCount.Load(); got != 1 {
		t.Errorf("inner reads = %d, want 1", got)
	}

	// Verify all values.
	for i, v := range vals {
		expected := uint16(10 + i)
		got := binary.BigEndian.Uint16(v.Raw)
		if got != expected {
			t.Errorf("val[%d] = %d, want %d", i, got, expected)
		}
	}
}

func TestClusteringWithModbusServerSparseAddresses(t *testing.T) {
	_, addr, _ := startTestModbusServer(t, 500)

	ctx := context.Background()
	client, err := modbus.Connect(ctx, addr.IP.String(),
		modbus.WithPort(addr.Port),
	)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	cr := NewClusteringReader(client)

	// Sparse: cluster at 10-12, outlier at 400.
	cr.Register(
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 11, Qty: 1},
		modbus.HoldingRegister{Addr: 12, Qty: 1},
		modbus.HoldingRegister{Addr: 400, Qty: 1},
	)

	vals, err := cr.Read(ctx,
		modbus.HoldingRegister{Addr: 10, Qty: 1},
		modbus.HoldingRegister{Addr: 11, Qty: 1},
		modbus.HoldingRegister{Addr: 12, Qty: 1},
		modbus.HoldingRegister{Addr: 400, Qty: 1},
	)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// 2 clusters → 2 inner reads.
	if got := cr.ReadCount.Load(); got != 2 {
		t.Errorf("inner reads = %d, want 2", got)
	}

	expected := []uint16{10, 11, 12, 400}
	for i, exp := range expected {
		got := binary.BigEndian.Uint16(vals[i].Raw)
		if got != exp {
			t.Errorf("val[%d] = %d, want %d", i, got, exp)
		}
	}
}

// ===========================================================================
// Monitor Integration: Subscribe auto-registers
// ===========================================================================

func TestMonitorSubscribeRegistersWithClusteringReader(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	mon, err := NewMonitor(cr, WithEventBuffer(64))
	if err != nil {
		t.Fatal(err)
	}
	defer mon.Close()

	// Subscribe 3 adjacent holding registers.
	for i := 10; i < 13; i++ {
		_, err := mon.Subscribe(
			modbus.HoldingRegister{Addr: modbus.Address(i), Qty: 1},
			WithFrequency(50*time.Millisecond),
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Verify points were registered.
	cr.mu.RLock()
	numPoints := len(cr.points)
	numWindows := len(cr.windows)
	cr.mu.RUnlock()

	if numPoints != 3 {
		t.Errorf("registered points = %d, want 3", numPoints)
	}
	if numWindows != 1 {
		t.Errorf("windows = %d, want 1", numWindows)
	}
}

// ===========================================================================
// Non-Clusterable passthrough
// ===========================================================================

type nonClusterablePoint struct {
	name string
}

func (n nonClusterablePoint) String() string { return n.name }

func TestNonClusterablePassesThrough(t *testing.T) {
	inner := newCountingReader(func(dp plc.DataPoint) plc.Value {
		return plc.Value{Raw: []byte{0x42}}
	})
	cr := NewClusteringReader(inner)

	ctx := context.Background()
	vals, err := cr.Read(ctx, nonClusterablePoint{name: "custom"})
	if err != nil {
		t.Fatal(err)
	}

	if inner.count() != 1 {
		t.Errorf("inner = %d, want 1", inner.count())
	}
	if len(vals) != 1 || vals[0].Raw[0] != 0x42 {
		t.Error("unexpected value for non-clusterable point")
	}
}

// ===========================================================================
// Determinism: repeated runs produce identical results
// ===========================================================================

func TestDeterminism(t *testing.T) {
	for run := range 5 {
		inner := newCountingReader(registerDataFn)
		cr := NewClusteringReader(inner)

		cr.Register(
			modbus.HoldingRegister{Addr: 1, Qty: 1},
			modbus.HoldingRegister{Addr: 25, Qty: 1},
			modbus.HoldingRegister{Addr: 26, Qty: 1},
			modbus.HoldingRegister{Addr: 27, Qty: 1},
			modbus.HoldingRegister{Addr: 488, Qty: 1},
		)

		ctx := context.Background()
		vals, err := cr.Read(ctx,
			modbus.HoldingRegister{Addr: 1, Qty: 1},
			modbus.HoldingRegister{Addr: 25, Qty: 1},
			modbus.HoldingRegister{Addr: 26, Qty: 1},
			modbus.HoldingRegister{Addr: 27, Qty: 1},
			modbus.HoldingRegister{Addr: 488, Qty: 1},
		)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}

		expected := []uint16{1, 25, 26, 27, 488}
		for i, exp := range expected {
			got := binary.BigEndian.Uint16(vals[i].Raw)
			if got != exp {
				t.Errorf("run %d: val[%d] = %d, want %d", run, i, got, exp)
			}
		}

		if inner.count() != 2 {
			t.Errorf("run %d: inner = %d, want 2", run, inner.count())
		}
	}
}

// ===========================================================================
// Request counting display helper
// ===========================================================================

func TestClusteringRequestCountDisplay(t *testing.T) {
	// This test demonstrates the request reduction clearly.
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	// Scenario from SOW: 7 addresses in one cluster.
	addrs := []modbus.Address{1, 25, 26, 27, 30, 33, 34}
	points := make([]plc.DataPoint, len(addrs))
	for i, a := range addrs {
		p := modbus.HoldingRegister{Addr: a, Qty: 1}
		cr.Register(p)
		points[i] = p
	}

	ctx := context.Background()
	_, err := cr.Read(ctx, points...)
	if err != nil {
		t.Fatal(err)
	}

	baseline := len(addrs)
	optimized := int(inner.count())

	t.Logf("Baseline requests:  %d", baseline)
	t.Logf("Optimized requests: %d", optimized)
	t.Logf("Reduction:          %dx", baseline/optimized)

	if optimized >= baseline {
		t.Error("optimized should be fewer requests than baseline")
	}
}

func TestClusteringRequestCountDisplaySparse(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	addrs := []modbus.Address{1, 25, 26, 27, 30, 33, 34, 488}
	points := make([]plc.DataPoint, len(addrs))
	for i, a := range addrs {
		p := modbus.HoldingRegister{Addr: a, Qty: 1}
		cr.Register(p)
		points[i] = p
	}

	ctx := context.Background()
	_, err := cr.Read(ctx, points...)
	if err != nil {
		t.Fatal(err)
	}

	baseline := len(addrs)
	optimized := int(inner.count())

	t.Logf("Baseline requests:  %d", baseline)
	t.Logf("Optimized requests: %d", optimized)
	t.Logf("Reduction:          %.1fx", float64(baseline)/float64(optimized))

	if optimized != 2 {
		t.Errorf("expected 2 optimized requests, got %d", optimized)
	}
}

// ===========================================================================
// Edge case: empty inputs
// ===========================================================================

func TestReadEmptyPoints(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	ctx := context.Background()
	vals, err := cr.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 0 {
		t.Errorf("expected 0 values, got %d", len(vals))
	}
	if inner.count() != 0 {
		t.Errorf("expected 0 inner reads, got %d", inner.count())
	}
}

func TestReadNoRegisteredPoints(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)
	// No Register() call — point falls back to direct read.

	ctx := context.Background()
	vals, err := cr.Read(ctx, modbus.HoldingRegister{Addr: 42, Qty: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Falls back to direct read.
	if inner.count() != 1 {
		t.Errorf("inner = %d, want 1", inner.count())
	}
	if binary.BigEndian.Uint16(vals[0].Raw) != 42 {
		t.Errorf("val = %d, want 42", binary.BigEndian.Uint16(vals[0].Raw))
	}
}

// ===========================================================================
// Overlapping point ranges
// ===========================================================================

func TestOverlappingRanges(t *testing.T) {
	inner := newCountingReader(registerDataFn)
	cr := NewClusteringReader(inner)

	// Register overlapping ranges.
	cr.Register(
		modbus.HoldingRegister{Addr: 100, Qty: 5}, // 100-104
		modbus.HoldingRegister{Addr: 103, Qty: 5}, // 103-107
	)

	ctx := context.Background()
	vals, err := cr.Read(ctx,
		modbus.HoldingRegister{Addr: 100, Qty: 5},
		modbus.HoldingRegister{Addr: 103, Qty: 5},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Should merge into one window [100, 108) = qty 8.
	if inner.count() != 1 {
		t.Errorf("inner = %d, want 1", inner.count())
	}

	// Verify first range: 100,101,102,103,104
	if len(vals[0].Raw) != 10 {
		t.Fatalf("val[0] raw len = %d, want 10", len(vals[0].Raw))
	}
	for i := 0; i < 5; i++ {
		got := binary.BigEndian.Uint16(vals[0].Raw[i*2:])
		if got != uint16(100+i) {
			t.Errorf("val[0][%d] = %d, want %d", i, got, 100+i)
		}
	}

	// Verify second range: 103,104,105,106,107
	for i := 0; i < 5; i++ {
		got := binary.BigEndian.Uint16(vals[1].Raw[i*2:])
		if got != uint16(103+i) {
			t.Errorf("val[1][%d] = %d, want %d", i, got, 103+i)
		}
	}
}

// ===========================================================================
// Printing helpers for test output
// ===========================================================================

func init() {
	_ = fmt.Sprintf // avoid unused import
}
