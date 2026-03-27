package monitor

import "bytes"

// ByteChangeDetector is a [ChangeDetector] that reports a change whenever the
// raw byte representations of two snapshots differ. It is the simplest and
// most commonly used detector.
type ByteChangeDetector struct{}

// Detect returns true when the raw bytes of prev and curr are not equal.
func (ByteChangeDetector) Detect(prev, curr Snapshot) bool {
	return !bytes.Equal(prev.Value.Raw, curr.Value.Raw)
}
