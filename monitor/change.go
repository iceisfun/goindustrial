package monitor

import "bytes"

// ByteChangeDetector compares raw bytes to detect changes.
// This is the simplest and most common change detector.
type ByteChangeDetector struct{}

func (ByteChangeDetector) Detect(prev, curr Snapshot) bool {
	return !bytes.Equal(prev.Value.Raw, curr.Value.Raw)
}
