package monitor

import (
	"iter"
	"sync"
)

// Subscriber receives events broadcast by the Monitor. Each Subscriber has
// its own buffered channel, so a slow consumer never blocks the monitor or
// other subscribers — events are silently dropped when the buffer is full.
//
// Call Done when finished to unregister from the monitor and close the
// event channel:
//
//	sub, err := mon.NewSubscriber(128)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer sub.Done()
//
//	for evt := range sub.All() {
//	    fmt.Println(evt.Snapshot.Point, evt.Changed)
//	}
type Subscriber struct {
	ch        chan Event
	monitor   *Monitor
	closeOnce sync.Once
	doneOnce  sync.Once
}

// Events returns the subscriber's receive-only event channel.
func (s *Subscriber) Events() <-chan Event {
	return s.ch
}

// Done unregisters the subscriber from the monitor and closes its event
// channel. Safe to call multiple times. Should be deferred immediately
// after creating the subscriber.
func (s *Subscriber) Done() {
	s.doneOnce.Do(func() {
		s.monitor.unregisterSubscriber(s)
		s.closeCh()
	})
}

// All returns an iterator over events suitable for use in a for-range loop.
// The iterator yields events until Done is called or the monitor is closed.
//
//	for evt := range sub.All() {
//	    if evt.Err != nil {
//	        log.Println("read error:", evt.Err)
//	        continue
//	    }
//	    fmt.Println(evt.Snapshot.Point, evt.Changed)
//	}
func (s *Subscriber) All() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for evt := range s.ch {
			if !yield(evt) {
				return
			}
		}
	}
}

func (s *Subscriber) closeCh() {
	s.closeOnce.Do(func() {
		close(s.ch)
	})
}
