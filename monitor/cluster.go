package monitor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/plc"
)

// Clusterable is implemented by [plc.DataPoint] types that support read
// clustering. The [ClusteringReader] uses this interface to merge nearby
// addresses into block reads, reducing network round trips for protocols
// like Modbus TCP.
//
// Implementors must provide an area key for grouping (e.g., "holding" or
// "coil"), a start address and quantity, and methods to merge address ranges
// and extract sub-ranges from block read results.
type Clusterable interface {
	plc.DataPoint

	// ClusterKey returns an area grouping key (e.g., "holding-registers").
	// Points with different keys are never merged.
	ClusterKey() string

	// ClusterAddr returns the starting address of this data point.
	ClusterAddr() uint16

	// ClusterQty returns the number of units (registers or coils) this
	// point spans.
	ClusterQty() uint16

	// ClusterBitsPerUnit returns the bit width of each unit: 16 for
	// registers, 1 for coils and discrete inputs.
	ClusterBitsPerUnit() uint16

	// ClusterMerge creates a new [plc.DataPoint] representing the merged
	// read window starting at start with the given count of units.
	ClusterMerge(start, count uint16) plc.DataPoint

	// ClusterExtract extracts this point's value from a block read result
	// that started at clusterStart.
	ClusterExtract(val plc.Value, clusterStart uint16) plc.Value
}

// Registrar is implemented by readers that accept point registration hints so
// they can pre-compute optimized read plans. The [Monitor] calls Register when
// a subscription is created and Unregister when it is stopped.
// [ClusteringReader] implements this interface.
type Registrar interface {
	// Register informs the reader about data points that will be read.
	Register(points ...plc.DataPoint)
	// Unregister tells the reader that the given data points are no longer needed.
	Unregister(points ...plc.DataPoint)
}

// ClusterConfig holds parameters that control how nearby addresses are merged
// into block reads.
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

// ClusterOption configures a [ClusteringReader] created by
// [NewClusteringReader].
type ClusterOption func(*ClusteringReader)

// WithGapThreshold sets the maximum gap (in register or coil units) between
// two addresses that will still be merged into a single block read.
func WithGapThreshold(gap uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.GapThreshold = gap }
}

// WithMaxRegistersPerRead sets the upper limit on the number of registers that
// may be read in a single block request. The Modbus specification allows up to
// 125; the default is 120 to leave headroom.
func WithMaxRegistersPerRead(max uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.MaxRegistersPerRead = max }
}

// WithMaxCoilsPerRead sets the upper limit on the number of coils or discrete
// inputs that may be read in a single block request.
func WithMaxCoilsPerRead(max uint16) ClusterOption {
	return func(r *ClusteringReader) { r.config.MaxCoilsPerRead = max }
}

// WithCacheTTL sets how long clustered read results are cached. While the
// cache is fresh, concurrent subscription goroutines reading from the same
// cluster window share a single network request. A zero TTL (the default)
// disables caching.
func WithCacheTTL(ttl time.Duration) ClusterOption {
	return func(r *ClusteringReader) { r.config.CacheTTL = ttl }
}

// WithClusteringEnabled enables or disables clustering entirely.
func WithClusteringEnabled(enabled bool) ClusterOption {
	return func(r *ClusteringReader) { r.config.Enabled = enabled }
}

// ClusteringReader wraps a [plc.Reader] and coalesces nearby Modbus register
// or coil addresses into block reads, reducing the number of protocol round
// trips. It implements both [plc.Reader] and [Registrar].
//
// As subscriptions are registered via [ClusteringReader.Register], a read plan
// of contiguous windows is built. When [ClusteringReader.Read] is called, each
// requested point is served from the smallest window that covers it. A
// singleflight mechanism ensures that concurrent reads targeting the same
// window produce only one network request.
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

	// ReadCount tracks the total number of reads dispatched to the inner
	// reader, useful for testing and performance monitoring.
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

// NewClusteringReader creates a [ClusteringReader] that wraps the given
// [plc.Reader]. Use [ClusterOption] values to tune gap thresholds, maximum
// read sizes, and caching behavior.
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

// Register adds data points to the clustering plan and rebuilds the read
// windows. Non-[Clusterable] points are silently ignored. This method is
// safe for concurrent use.
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

// Unregister removes data points from the clustering plan and rebuilds the
// read windows. Non-[Clusterable] points are silently ignored. This method is
// safe for concurrent use.
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

// Read implements [plc.Reader]. For [Clusterable] points that fall within a
// pre-computed cluster window, it performs a single block read of the window
// and extracts the requested sub-range. Non-clusterable points and points not
// covered by any window are read individually through the inner reader.
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
