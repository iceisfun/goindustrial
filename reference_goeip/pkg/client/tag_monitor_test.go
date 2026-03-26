package client

import (
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
)

func TestNewTagMonitorRequiresReader(t *testing.T) {
	if _, err := NewTagMonitor(nil); err == nil {
		t.Fatalf("expected error when reader is missing")
	}
}

func TestTagMonitorRefreshableChangeDetection(t *testing.T) {
	reader := newStubTagReader()
	reader.setValue("Example", encodeDINT(1))

	monitor, err := NewTagMonitor(nil, WithMonitorReader(reader), WithMonitorLogger(internal.NopLogger()))
	if err != nil {
		t.Fatalf("NewTagMonitor() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	state := &dintState{}
	if _, err := monitor.AddTag("Example", WithFrequency(10*time.Millisecond), WithRefreshable(state)); err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}

	evt := waitForEvent(t, monitor, time.Second)
	if evt.Err != nil {
		t.Fatalf("first event error = %v", evt.Err)
	}
	if !evt.Changed {
		t.Fatalf("expected initial change to be true")
	}

	reader.setValue("Example", encodeDINT(1))
	evt = waitForEvent(t, monitor, time.Second)
	if evt.Changed {
		t.Fatalf("expected unchanged state when value repeats")
	}

	reader.setValue("Example", encodeDINT(42))
	evt = waitForEvent(t, monitor, time.Second)
	if evt.Err != nil {
		t.Fatalf("event error = %v", evt.Err)
	}
	if !evt.Changed {
		t.Fatalf("expected change when value differs")
	}
}

func TestTagMonitorStopSubscription(t *testing.T) {
	reader := newStubTagReader()
	reader.setValue("StopMe", encodeDINT(5))

	monitor, err := NewTagMonitor(nil, WithMonitorReader(reader), WithMonitorLogger(internal.NopLogger()))
	if err != nil {
		t.Fatalf("NewTagMonitor() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	sub, err := monitor.AddTag("StopMe", WithFrequency(5*time.Millisecond))
	if err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}

	_ = waitForEvent(t, monitor, time.Second)

	sub.Stop()
	drainEvents(monitor, 50*time.Millisecond)

	reader.setValue("StopMe", encodeDINT(99))

	select {
	case evt := <-monitor.Wait():
		t.Fatalf("received unexpected event after stop: %+v", evt)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestTagMonitorErrorEvent(t *testing.T) {
	reader := newStubTagReader()
	reader.setError("ErrTag", errors.New("boom"))

	monitor, err := NewTagMonitor(nil, WithMonitorReader(reader), WithMonitorLogger(internal.NopLogger()))
	if err != nil {
		t.Fatalf("NewTagMonitor() error = %v", err)
	}
	t.Cleanup(monitor.Close)

	if _, err := monitor.AddTag("ErrTag", WithFrequency(5*time.Millisecond)); err != nil {
		t.Fatalf("AddTag() error = %v", err)
	}

	evt := waitForEvent(t, monitor, time.Second)
	if evt.Err == nil {
		t.Fatalf("expected error event")
	}
}

type stubTagReader struct {
	mu     sync.Mutex
	values map[string][]byte
	errs   map[string]error
}

func newStubTagReader() *stubTagReader {
	return &stubTagReader{
		values: make(map[string][]byte),
		errs:   make(map[string]error),
	}
}

func (s *stubTagReader) ReadTag(tag string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.errs[tag]; err != nil {
		return nil, err
	}
	data, ok := s.values[tag]
	if !ok {
		return nil, errors.New("unknown tag")
	}
	return append([]byte(nil), data...), nil
}

func (s *stubTagReader) setValue(tag string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[tag] = append([]byte(nil), data...)
	delete(s.errs, tag)
}

func (s *stubTagReader) setError(tag string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.errs, tag)
	} else {
		s.errs[tag] = err
	}
}

func encodeDINT(value int32) []byte {
	buf := make([]byte, 6)
	binary.LittleEndian.PutUint16(buf[:2], uint16(cip.TypeDINT))
	binary.LittleEndian.PutUint32(buf[2:], uint32(value))
	return buf
}

func waitForEvent(t *testing.T, monitor *TagMonitor, timeout time.Duration) TagEvent {
	t.Helper()
	select {
	case evt := <-monitor.Wait():
		return evt
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event")
	}
	return TagEvent{}
}

func drainEvents(monitor *TagMonitor, window time.Duration) {
	timer := time.NewTimer(window)
	defer timer.Stop()
	for {
		select {
		case <-monitor.Wait():
		case <-timer.C:
			return
		}
	}
}

type dintState struct {
	mu    sync.Mutex
	value int32
	set   bool
}

func (d *dintState) Refresh(snapshot TagSnapshot) (bool, error) {
	var next int32
	if err := snapshot.Into(&next); err != nil {
		return false, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.set || d.value != next {
		d.value = next
		d.set = true
		return true, nil
	}
	return false, nil
}
