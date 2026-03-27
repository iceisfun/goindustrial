package monitor

import (
	"sync"
	"testing"
	"time"
)

func TestSubscriberReceivesEvents(t *testing.T) {
	reader := &stubReader{values: []byte{0x01, 0x02}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sub.Done()

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case evt := <-sub.Events():
		if evt.Err != nil {
			t.Fatalf("unexpected error: %v", evt.Err)
		}
		if evt.Snapshot.Point.String() != "tag1" {
			t.Errorf("expected tag1, got %s", evt.Snapshot.Point.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for subscriber event")
	}
}

func TestSubscriberBroadcastToMultiple(t *testing.T) {
	reader := &stubReader{values: []byte{0xAA}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub1, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sub1.Done()

	sub2, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sub2.Done()

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both subscribers should receive the same event.
	for _, sub := range []*Subscriber{sub1, sub2} {
		select {
		case evt := <-sub.Events():
			if evt.Err != nil {
				t.Fatalf("unexpected error: %v", evt.Err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for subscriber event")
		}
	}
}

func TestSubscriberNeverBlocksMonitor(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	// Create subscriber with tiny buffer of 1.
	sub, err := m.NewSubscriber(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sub.Done()

	// Create a fast-polling subscription. If the subscriber blocked the
	// monitor, this would deadlock or hang.
	_, err = m.Subscribe(testPoint{name: "fast"}, WithFrequency(5*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Don't consume from the subscriber at all. Wait to confirm the
	// monitor keeps running (it drops events for the slow subscriber).
	time.Sleep(100 * time.Millisecond)

	// The reader should have been called many times, proving the monitor
	// was not blocked by the full subscriber buffer.
	calls := reader.callCount.Load()
	if calls < 5 {
		t.Errorf("expected at least 5 polls, got %d (monitor may have been blocked)", calls)
	}
}

func TestSubscriberDone(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Receive at least one event.
	select {
	case <-sub.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	// Done should close the channel.
	sub.Done()

	// Channel should be closed — range should terminate.
	_, ok := <-sub.Events()
	if ok {
		// Might get a buffered event, drain.
		for range sub.Events() {
		}
	}

	// Double-Done should not panic.
	sub.Done()
}

func TestSubscriberAll(t *testing.T) {
	reader := &stubReader{values: []byte{0x42}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use All() in a for-range, collecting a few events then breaking.
	count := 0
	for evt := range sub.All() {
		if evt.Err != nil {
			t.Fatalf("unexpected error: %v", evt.Err)
		}
		count++
		if count >= 3 {
			break
		}
	}

	sub.Done()

	if count < 3 {
		t.Errorf("expected 3 events from All(), got %d", count)
	}
}

func TestSubscriberAllTerminatesOnDone(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer m.Close()

	sub, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for range sub.All() {
		}
		close(done)
	}()

	// Let some events flow.
	time.Sleep(80 * time.Millisecond)

	// Done should cause All() to terminate.
	sub.Done()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: All() did not terminate after Done()")
	}
}

func TestSubscriberAllTerminatesOnClose(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sub, err := m.NewSubscriber(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer sub.Done()

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(20*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for range sub.All() {
		}
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)

	// Monitor.Close should close all subscriber channels.
	m.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: All() did not terminate after Monitor.Close()")
	}
}

func TestNewSubscriberAfterClose(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m.Close()

	_, err = m.NewSubscriber(64)
	if err != ErrMonitorClosed {
		t.Errorf("expected ErrMonitorClosed, got %v", err)
	}
}

func TestSubscriberConcurrentDoneAndClose(t *testing.T) {
	reader := &stubReader{values: []byte{0x01}}
	m, err := NewMonitor(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	subs := make([]*Subscriber, 10)
	for i := range subs {
		subs[i], err = m.NewSubscriber(16)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	_, err = m.Subscribe(testPoint{name: "tag1"}, WithFrequency(10*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Race Done() and Close() concurrently. Should not panic or deadlock.
	var wg sync.WaitGroup
	wg.Add(len(subs) + 1)
	for _, sub := range subs {
		go func() {
			defer wg.Done()
			sub.Done()
		}()
	}
	go func() {
		defer wg.Done()
		m.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: concurrent Done/Close appears deadlocked")
	}
}
