package monitor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/plc"
)

// Clusterable is implemented by data point types that support read clustering.
// The monitor uses this interface to merge nearby addresses into single reads.
type Clusterable interface {
	plc.DataPoint
	ClusterKey() string                                          // area grouping key
	ClusterAddr() uint16                                         // start address
	ClusterQty() uint16                                          // unit count
	ClusterBitsPerUnit() uint16                                  // 16 for registers, 1 for coils
	ClusterMerge(start, count uint16) plc.DataPoint              // create merged point
	ClusterExtract(val plc.Value, clusterStart uint16) plc.Value // extract sub-range
}

// Registrar is implemented by readers that accept point registration hints.
// The monitor calls Register/Unregister when subscriptions are created/removed.
type Registrar interface {
	Register(points ...plc.DataPoint)
	Unregister(points ...plc.DataPoint)
}

// ClusterConfig holds clustering parameters.
type ClusterConfig struct {
	// GapThreshold is the max gap (in register/coil units) between two points
	// that will be merged into a single read window. Default: 32.
	GapThreshold uint16

	// MaxRegistersPerRead caps the size of a single register read.
	// Default: 120 (safely under Modbus 125 limit).
	MaxRegistersPerRead uint16

	// MaxCoilsPerRead caps the size of a single coil/discrete read.
	// Default: 2000.
	MaxCoilsPerRead uint16

	// CacheTTL controls how long cached cluster data is considered fresh.
	// Default: 0 (no caching — each Read() goes to wire).
	// Set > 0 to allow concurrent subscription goroutines to share reads.
	CacheTTL time.Duration

	// Enabled controls clustering. When false, reads pass through directly.
	// Default: true.
	Enabled bool
}

// ClusterOption configures a ClusteringReader.
type ClusterOption func(*ClusteringReader)

// WithGapThreshold sets the maximum gap between addresses for merging.
func WithGapThreshold(gap uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.GapThreshold = gap }
}

// WithMaxRegistersPerRead sets the max registers in a single read.
func WithMaxRegistersPerRead(max uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.MaxRegistersPerRead = max }
}

// WithMaxCoilsPerRead sets the max coils in a single read.
func WithMaxCoilsPerRead(max uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.MaxCoilsPerRead = max }
}

// WithCacheTTL sets how long clustered read data is cached.
func WithCacheTTL(ttl time.Duration) ClusterOption {
	return func(r *ClusteringReader) { r.config.CacheTTL = ttl }
}

// WithClusteringEnabled enables or disables clustering entirely.
func WithClusteringEnabled(enabled bool) ClusterOption {
	return func(r *ClusteringReader) { r.config.Enabled = enabled }
}

// ClusteringReader wraps a plc.Reader and coalesces nearby Modbus addresses
// into block reads, reducing the number of protocol requests.
type ClusteringReader struct {
	inner  plc.Reader
	config ClusterConfig

	// Registered points and pre-computed windows.
	mu      sync.RWMutex
	points  []Clusterable
	windows []readWindow

	// Cache of recently-read cluster data.
	cacheMu sync.RWMutex
	cache   map[int]*cachedRead

	// Singleflight: prevents duplicate concurrent reads for the same window.
	// Keyed by "gen:windowIndex".
	flightMu sync.Mutex
	flights  map[string]*flightEntry

	// Generation counter — incremented on each replan to invalidate
	// in-flight reads that used a stale plan.
	gen atomic.Int64

	// ReadCount tracks total inner reads for testing/monitoring.
	ReadCount atomic.Int64
}

type cachedRead struct {
	val       plc.Value
	timestamp time.Time
}

type flightEntry struct {
	done chan struct{}
	val  plc.Value
	err  error
}

// NewClusteringReader creates a clustering reader wrapping the given reader.
func NewClusteringReader(inner plc.Reader, opts ...ClusterOption) *ClusteringReader {
	r := &ClusteringReader{
		inner: inner,
		config: ClusterConfig{
			GapThreshold:        32,
			MaxRegistersPerRead: 120,
			MaxCoilsPerRead:     2000,
			Enabled:             true,
		},
		cache:   make(map[int]*cachedRead),
		flights: make(map[string]*flightEntry),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds points to the clustering plan. Thread-safe.
func (r *ClusteringReader) Register(points ...plc.DataPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range points {
		if c, ok := p.(Clusterable); ok {
			r.points = append(r.points, c)
		}
	}
	r.replan()
}

// Unregister removes points from the clustering plan. Thread-safe.
func (r *ClusteringReader) Unregister(points ...plc.DataPoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range points {
		c, ok := p.(Clusterable)
		if !ok {
			continue
		}
		for i, existing := range r.points {
			if existing.ClusterKey() == c.ClusterKey() &&
				existing.ClusterAddr() == c.ClusterAddr() &&
				existing.ClusterQty() == c.ClusterQty() {
				r.points = append(r.points[:i], r.points[i+1:]...)
				break
			}
		}
	}
	r.replan()
}

// replan rebuilds cluster windows from registered points. Must hold mu write lock.
func (r *ClusteringReader) replan() {
	r.windows = buildWindows(r.points, r.config)
	r.gen.Add(1)

	// Invalidate cache on replan.
	r.cacheMu.Lock()
	r.cache = make(map[int]*cachedRead)
	r.cacheMu.Unlock()
}

// Read implements plc.Reader. It clusters Clusterable points into block reads
// and extracts the requested sub-ranges from the results.
func (r *ClusteringReader) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	if !r.config.Enabled {
		r.ReadCount.Add(int64(len(points)))
		return r.inner.Read(ctx, points...)
	}

	results := make([]plc.Value, len(points))

	// Separate clusterable from non-clusterable points.
	type pendingRead struct {
		idx     int // index in results
		winIdx  int // window index (-1 for direct read)
		point   plc.DataPoint
		cluster Clusterable
	}

	var directPoints []pendingRead
	var clusteredPoints []pendingRead

	r.mu.RLock()
	windows := r.windows
	gen := r.gen.Load()
	r.mu.RUnlock()

	for i, p := range points {
		c, ok := p.(Clusterable)
		if !ok {
			directPoints = append(directPoints, pendingRead{idx: i, winIdx: -1, point: p})
			continue
		}

		winIdx := findWindow(windows, c)
		if winIdx < 0 {
			// Not covered by any window — read directly.
			directPoints = append(directPoints, pendingRead{idx: i, winIdx: -1, point: p})
			continue
		}

		clusteredPoints = append(clusteredPoints, pendingRead{idx: i, winIdx: winIdx, point: p, cluster: c})
	}

	// Direct reads: pass through to inner reader one at a time.
	for _, dp := range directPoints {
		vals, err := r.inner.Read(ctx, dp.point)
		r.ReadCount.Add(1)
		if err != nil {
			return nil, err
		}
		if len(vals) > 0 {
			results[dp.idx] = vals[0]
		}
	}

	// Clustered reads: deduplicate by window index.
	windowsNeeded := make(map[int]struct{})
	for _, cp := range clusteredPoints {
		windowsNeeded[cp.winIdx] = struct{}{}
	}

	// Read each needed window (with cache + singleflight).
	windowValues := make(map[int]plc.Value)
	for winIdx := range windowsNeeded {
		val, err := r.readWindow(ctx, windows, winIdx, gen)
		if err != nil {
			return nil, err
		}
		windowValues[winIdx] = val
	}

	// Extract each point's value from its window.
	for _, cp := range clusteredPoints {
		wv := windowValues[cp.winIdx]
		results[cp.idx] = cp.cluster.ClusterExtract(wv, windows[cp.winIdx].start)
		results[cp.idx].DataPoint = cp.point
	}

	return results, nil
}

// readWindow reads a single cluster window, using cache and singleflight.
func (r *ClusteringReader) readWindow(ctx context.Context, windows []readWindow, winIdx int, gen int64) (plc.Value, error) {
	// Check cache.
	if r.config.CacheTTL > 0 {
		r.cacheMu.RLock()
		cached, ok := r.cache[winIdx]
		r.cacheMu.RUnlock()
		if ok && time.Since(cached.timestamp) < r.config.CacheTTL {
			return cached.val, nil
		}
	}

	// Singleflight: only one goroutine reads per window per generation.
	key := fmt.Sprintf("%d:%d", gen, winIdx)

	r.flightMu.Lock()
	if f, ok := r.flights[key]; ok {
		r.flightMu.Unlock()
		// Wait for the in-flight read.
		select {
		case <-f.done:
			return f.val, f.err
		case <-ctx.Done():
			return plc.Value{}, ctx.Err()
		}
	}

	f := &flightEntry{done: make(chan struct{})}
	r.flights[key] = f
	r.flightMu.Unlock()

	// Actually read from the inner reader.
	w := windows[winIdx]
	vals, err := r.inner.Read(ctx, w.merged)
	r.ReadCount.Add(1)

	if err == nil && len(vals) > 0 {
		f.val = vals[0]
		// Cache the result.
		if r.config.CacheTTL > 0 {
			r.cacheMu.Lock()
			r.cache[winIdx] = &cachedRead{
				val:       vals[0],
				timestamp: time.Now(),
			}
			r.cacheMu.Unlock()
		}
	}
	f.err = err
	close(f.done)

	// Clean up flight entry.
	r.flightMu.Lock()
	delete(r.flights, key)
	r.flightMu.Unlock()

	if err != nil {
		return plc.Value{}, err
	}
	return f.val, nil
}
