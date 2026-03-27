package monitor

import (
	"sort"

	"github.com/iceisfun/goindustrial/plc"
)

// readWindow is a pre-computed contiguous read range covering one or more
// registered Clusterable points within the same area.
type readWindow struct {
	key      string        // ClusterKey() value — area identifier
	start    uint16        // starting address
	count    uint16        // number of units (registers or coils)
	merged   plc.DataPoint // the merged DataPoint to read the full window
	points   []Clusterable // original registered points covered by this window
}

// buildWindows groups registered Clusterable points by area, sorts them,
// and merges nearby points into read windows respecting gap and max-size
// constraints. This is a pure function with no side effects.
func buildWindows(points []Clusterable, cfg ClusterConfig) []readWindow {
	if len(points) == 0 {
		return nil
	}

	// Group by area key.
	groups := make(map[string][]Clusterable)
	for _, p := range points {
		k := p.ClusterKey()
		groups[k] = append(groups[k], p)
	}

	var windows []readWindow

	for key, pts := range groups {
		// Sort ascending by address.
		sort.Slice(pts, func(i, j int) bool {
			return pts[i].ClusterAddr() < pts[j].ClusterAddr()
		})

		maxPerRead := cfg.MaxRegistersPerRead
		if pts[0].ClusterBitsPerUnit() == 1 {
			maxPerRead = cfg.MaxCoilsPerRead
		}

		// Merge pass.
		winStart := pts[0].ClusterAddr()
		winEnd := winStart + pts[0].ClusterQty() // exclusive end
		winPoints := []Clusterable{pts[0]}

		flush := func() {
			count := winEnd - winStart
			windows = append(windows, readWindow{
				key:    key,
				start:  winStart,
				count:  count,
				merged: winPoints[0].ClusterMerge(winStart, count),
				points: winPoints,
			})
		}

		for i := 1; i < len(pts); i++ {
			p := pts[i]
			pEnd := p.ClusterAddr() + p.ClusterQty()

			gap := uint16(0)
			if p.ClusterAddr() > winEnd {
				gap = p.ClusterAddr() - winEnd
			}

			mergedCount := pEnd - winStart
			if pEnd < winStart {
				// Overlap: point is fully contained.
				mergedCount = winEnd - winStart
			}

			canMerge := gap <= cfg.GapThreshold && mergedCount <= maxPerRead

			if canMerge {
				if pEnd > winEnd {
					winEnd = pEnd
				}
				winPoints = append(winPoints, p)
			} else {
				flush()
				winStart = p.ClusterAddr()
				winEnd = pEnd
				winPoints = []Clusterable{p}
			}
		}
		flush()
	}

	return windows
}

// findWindow returns the index of the window containing the given point,
// or -1 if no window covers it.
func findWindow(windows []readWindow, p Clusterable) int {
	key := p.ClusterKey()
	addr := p.ClusterAddr()
	end := addr + p.ClusterQty()

	for i := range windows {
		w := &windows[i]
		if w.key != key {
			continue
		}
		wEnd := w.start + w.count
		if addr >= w.start && end <= wEnd {
			return i
		}
	}
	return -1
}
