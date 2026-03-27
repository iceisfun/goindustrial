package monitor

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iceisfun/goindustrial/plc"
)

type testPoint struct {
	name string
}

func (t testPoint) String() string { return t.name }

type stubReader struct {
	callCount atomic.Int32
	values    []byte
	err       error
}

func (s *stubReader) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	s.callCount.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	results := make([]plc.Value, len(points))
	for i, p := range points {
		raw := make([]byte, len(s.values))
		copy(raw, s.values)
		results[i] = plc.Value{DataPoint: p, Raw: raw}
	}
	return results, nil
}

// blockingReader blocks until the context is canceled, simulating a
// slow or stuck reader. It records the context error in ctxErr.
type blockingReader struct {
	started chan struct{} // closed when Read begins blocking
	ctxErr  atomic.Value // stores the context.Err() observed on wakeup
}

func newBlockingReader() *blockingReader {
	return &blockingReader{started: make(chan struct{})}
}

func (b *blockingReader) Read(ctx context.Context, points ...plc.DataPoint) ([]plc.Value, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	b.ctxErr.Store(ctx.Err())
	return nil, ctx.Err()
}

func TestMonitorSubscribeAndReceiveEvent(t *testing.T) {
	reader := &stubReader{values: []byte{0x01, 0x02}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	point := testPoint{name: "test_tag"}
	_, err = m.Subscribe(point, WithFrequency(50*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case evt := <-m.Events():
		if evt.Err != nil {
			t.Fatalf("unexpected event error: %v", evt.Err)
		}
		if evt.Snapshot.Point.String() != "test_tag" {
			t.Errorf("expected point name test_tag, got %s", evt.Snapshot.Point.String())
		}
		if len(evt.Snapshot.Value.Raw) != 2 {
			t.Errorf("expected 2 raw bytes, got %d", len(evt.Snapshot.Value.Raw))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMonitorMultipleEvents(t *testing.T) {
	reader := &stubReader{values: []byte{0xAA}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count := 0
	timeout := time.After(2 * time.Second)
	for count < 3 {
		select {
		case evt := <-m.Events():
			if evt.Err != nil {
				t.Fatalf("unexpected error: %v", evt.Err)
			}
			count++
		case <-timeout:
			t.Fatalf("timeout, only received %d events", count)
		}
	}
}

func TestMonitorSubscriptionStop(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.Subscribe(testPoint{name: "tag1"},
		WithFrequency(20*time.Millisecond),
		WithInitialRead(false),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Let it poll a couple times.
	time.Sleep(80 * time.Millisecond)
	sub.Stop()
	countBefore := reader.callCount.Load()

	// Wait and verify no more polls.
	time.Sleep(80 * time.Millisecond)
	countAfter := reader.callCount.Load()

	if countAfter != countBefore {
		t.Errorf("expected no more polls after stop, got %d more", countAfter-countBefore)
	}
}

func TestMonitorChangeDetector(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	_, err = m.Subscribe(testPoint{name: "tag1"},
		WithFrequency(20*time.Millisecond),
		WithChangeDetector(ByteChangeDetector{}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First event should always be "changed" (no previous).
	evt := <-m.Events()
	if !evt.Changed {
		t.Error("first event should report changed")
	}

	// Subsequent events with same data should report not changed.
	evt = <-m.Events()
	if evt.Changed {
		t.Error("second event with same data should report not changed")
	}
}

func TestMonitorHandler(t *testing.T) {
	reader := &stubReader{values: []byte{0x42}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	handlerCalled := make(chan struct{}, 1)
	_, err = m.Subscribe(testPoint{name: "tag1"},
		WithFrequency(50*time.Millisecond),
		WithHandler(func(s Snapshot) {
			select {
			case handlerCalled <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-handlerCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for handler callback")
	}
}

func TestMonitorCloseStopsAll(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for range 5 {
		m.Subscribe(testPoint{name: "tag"}, WithFrequency(10*time.Millisecond))
	}

	// Should not block or panic.
	m.Close()

	// Drain any buffered events, then verify the channel is closed.
	for range m.Events() {
	}
}

func TestMonitorNilReader(t *testing.T) {
	_, err := NewMonitor(nil)
	if err == nil {
		t.Error("expected error for nil reader")
	}
}

func TestMonitorNilPoint(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	_, err = m.Subscribe(nil)
	if err == nil {
		t.Error("expected error for nil data point")
	}
}

func TestMonitorSubscribeAfterClose(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m.Close()

	_, err = m.Subscribe(testPoint{name: "tag"})
	if err != ErrMonitorClosed {
		t.Errorf("expected ErrMonitorClosed, got %v", err)
	}
}

func TestByteChangeDetector(t *testing.T) {
	d := ByteChangeDetector{}

	s1 := Snapshot{Value: plc.Value{Raw: []byte{0x01, 0x02}}}
	s2 := Snapshot{Value: plc.Value{Raw: []byte{0x01, 0x02}}}
	s3 := Snapshot{Value: plc.Value{Raw: []byte{0x01, 0x03}}}

	if d.Detect(s1, s2) {
		t.Error("same bytes should not be detected as changed")
	}
	if !d.Detect(s1, s3) {
		t.Error("different bytes should be detected as changed")
	}
}

func TestSubscriptionStopCancelsBlockedRead(t *testing.T) {
	br := newBlockingReader()
	m, err := NewMonitor(br)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.Subscribe(testPoint{name: "blocking"},
		WithFrequency(time.Hour), // long interval; only the immediate read matters
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the reader to start blocking.
	select {
	case <-br.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for blocking read to start")
	}

	// Stopping the subscription should cancel the context and unblock Read.
	done := make(chan struct{})
	go func() {
		sub.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscription stop; blocked read was not canceled")
	}

	stored := br.ctxErr.Load()
	if stored == nil {
		t.Fatal("expected context error to be recorded")
	}
	if stored != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", stored)
	}
}

func TestMonitorCloseCancelsBlockedRead(t *testing.T) {
	br := newBlockingReader()
	m, err := NewMonitor(br)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.Subscribe(testPoint{name: "blocking"},
		WithFrequency(time.Hour),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the reader to start blocking.
	select {
	case <-br.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for blocking read to start")
	}

	// Closing the monitor should cancel the context and unblock Read.
	done := make(chan struct{})
	go func() {
		m.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for monitor close; blocked read was not canceled")
	}

	stored := br.ctxErr.Load()
	if stored == nil {
		t.Fatal("expected context error to be recorded")
	}
	if stored != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", stored)
	}
}
