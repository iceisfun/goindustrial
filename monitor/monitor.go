package monitor

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/plc"
)

// ErrMonitorClosed is returned when an operation targets a stopped Monitor.
var ErrMonitorClosed = errors.New("monitor is closed")

// Snapshot holds the latest value of a monitored data point.
type Snapshot struct {
	Point     plc.DataPoint
	Value     plc.Value
	Timestamp time.Time
}

// Event is emitted by the monitor on each poll cycle.
type Event struct {
	SubscriptionID int64
	Snapshot       Snapshot
	Err            error
	Changed        bool
}

// ChangeDetector determines if a value has changed between two snapshots.
type ChangeDetector interface {
	Detect(prev, curr Snapshot) bool
}

// Handler is called after a successful poll.
type Handler func(Snapshot)

// Monitor polls data points on a schedule and emits events.
type Monitor struct {
	reader plc.Reader
	logger logging.Logger

	mu      sync.RWMutex
	subs    map[int64]*subscription
	closed  bool
	nextID  int64
	stopCh  chan struct{}
	events  chan Event
	closeMx sync.Once
	wg      sync.WaitGroup

	// ctx is canceled when the monitor is closed. Subscription contexts are
	// derived from it so that blocked reads are interrupted on shutdown.
	ctx    context.Context
	cancel context.CancelFunc

	subscriberMu sync.RWMutex
	subscribers  map[*Subscriber]struct{}
}

// NewMonitor creates a monitor bound to the provided reader.
func NewMonitor(reader plc.Reader, opts ...MonitorOption) (*Monitor, error) {
	if reader == nil {
		return nil, errors.New("monitor requires a reader")
	}

	cfg := monitorConfig{eventBuffer: 64}
	for _, opt := range opts {
		opt(&cfg)
	}

	logger := cfg.logger
	if logger == nil {
		logger = logging.NewNopLogger()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Monitor{
		reader:      reader,
		logger:      logger,
		subs:        make(map[int64]*subscription),
		stopCh:      make(chan struct{}),
		events:      make(chan Event, cfg.eventBuffer),
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[*Subscriber]struct{}),
	}, nil
}

// Events exposes the receive-only event stream.
func (m *Monitor) Events() <-chan Event {
	return m.events
}

// NewSubscriber creates a Subscriber that receives all events broadcast by
// the Monitor. Each Subscriber has its own buffered channel of the given
// size, so a slow consumer never blocks the monitor or other subscribers.
// Events are silently dropped when the buffer is full.
//
// Call Done when finished to unregister and close the channel:
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
func (m *Monitor) NewSubscriber(bufferSize int) (*Subscriber, error) {
	if bufferSize <= 0 {
		bufferSize = 64
	}

	m.subscriberMu.Lock()
	defer m.subscriberMu.Unlock()

	select {
	case <-m.stopCh:
		return nil, ErrMonitorClosed
	default:
	}

	s := &Subscriber{
		ch:      make(chan Event, bufferSize),
		monitor: m,
	}
	m.subscribers[s] = struct{}{}
	return s, nil
}

func (m *Monitor) unregisterSubscriber(s *Subscriber) {
	m.subscriberMu.Lock()
	delete(m.subscribers, s)
	m.subscriberMu.Unlock()
}

// Close stops the monitor and all active subscriptions.
func (m *Monitor) Close() {
	m.closeMx.Do(func() {
		m.cancel()
		close(m.stopCh)

		m.mu.Lock()
		subs := make([]*subscription, 0, len(m.subs))
		for _, sub := range m.subs {
			subs = append(subs, sub)
		}
		m.closed = true
		m.subs = make(map[int64]*subscription)
		m.mu.Unlock()

		for _, sub := range subs {
			sub.stop()
		}

		m.wg.Wait()
		close(m.events)

		m.subscriberMu.Lock()
		for sub := range m.subscribers {
			sub.closeCh()
		}
		m.subscribers = make(map[*Subscriber]struct{})
		m.subscriberMu.Unlock()
	})
}

// Subscribe registers a data point to poll. Returns a Subscription handle
// that can be used to stop polling for this specific point.
func (m *Monitor) Subscribe(point plc.DataPoint, opts ...SubscriptionOption) (*Subscription, error) {
	if point == nil {
		return nil, errors.New("data point is required")
	}

	cfg := defaultSubConfig()
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrMonitorClosed
	}
	m.nextID++
	id := m.nextID
	sub := newSubscription(id, point, *cfg, m)
	m.subs[id] = sub
	m.wg.Add(1)
	m.mu.Unlock()

	// If the reader supports registration, tell it about the new point
	// so it can optimize read plans (e.g., clustering).
	if reg, ok := m.reader.(Registrar); ok {
		reg.Register(point)
	}

	go func() {
		defer m.wg.Done()
		sub.run()
	}()

	return &Subscription{monitor: m, id: id}, nil
}

func (m *Monitor) removeSubscription(id int64) {
	m.mu.Lock()
	sub, ok := m.subs[id]
	if ok {
		delete(m.subs, id)
	}
	m.mu.Unlock()

	if ok {
		if reg, ok := m.reader.(Registrar); ok {
			reg.Unregister(sub.point)
		}
		sub.stop()
	}
}

func (m *Monitor) emit(event Event) {
	select {
	case <-m.stopCh:
		return
	default:
	}

	m.subscriberMu.RLock()
	for sub := range m.subscribers {
		select {
		case sub.ch <- event:
		default:
		}
	}
	m.subscriberMu.RUnlock()

	select {
	case <-m.stopCh:
	case m.events <- event:
	default:
	}
}

// Subscription represents a running polling routine.
type Subscription struct {
	monitor *Monitor
	id      int64
	once    sync.Once
}

// ID returns the subscription identifier used in events.
func (s *Subscription) ID() int64 { return s.id }

// Stop cancels the subscription.
func (s *Subscription) Stop() {
	if s.monitor == nil {
		return
	}
	s.once.Do(func() {
		s.monitor.removeSubscription(s.id)
	})
}

type subscription struct {
	id           int64
	point        plc.DataPoint
	frequency    time.Duration
	readVariance time.Duration
	handler      Handler
	detector     ChangeDetector
	immediate    bool

	monitor *Monitor
	prev    *Snapshot

	// ctx is derived from the monitor's context and canceled when the
	// subscription stops. It is passed to reader.Read so that blocked
	// reads are interrupted on stop or monitor close.
	ctx    context.Context
	cancel context.CancelFunc

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func newSubscription(id int64, point plc.DataPoint, cfg subConfig, monitor *Monitor) *subscription {
	ctx, cancel := context.WithCancel(monitor.ctx)
	return &subscription{
		id:           id,
		point:        point,
		frequency:    cfg.frequency,
		readVariance: cfg.readVariance,
		handler:      cfg.handler,
		detector:     cfg.detector,
		immediate:    cfg.immediate,
		monitor:      monitor,
		ctx:          ctx,
		cancel:       cancel,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

func (s *subscription) run() {
	defer close(s.doneCh)

	if s.immediate {
		s.poll()
	}

	for {
		delay := s.frequency
		if s.readVariance > 0 {
			variance := time.Duration(rand.Int64N(int64(s.readVariance)*2+1)) - s.readVariance
			delay += variance
			if delay < 0 {
				delay = 0
			}
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
			s.poll()
		case <-s.stopCh:
			timer.Stop()
			return
		case <-s.monitor.stopCh:
			timer.Stop()
			return
		}
	}
}

func (s *subscription) stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		close(s.stopCh)
		<-s.doneCh
	})
}

func (s *subscription) poll() {
	ts := time.Now()

	values, err := s.monitor.reader.Read(s.ctx, s.point)
	if err != nil {
		s.monitor.logger.Warn(s.ctx, "monitor read failed for %s: %v", s.point.String(), err)
		s.monitor.emit(Event{
			SubscriptionID: s.id,
			Snapshot:       Snapshot{Point: s.point, Timestamp: ts},
			Err:            err,
		})
		return
	}

	if len(values) == 0 {
		err := fmt.Errorf("monitor: no values returned for %s", s.point.String())
		s.monitor.logger.Warn(s.ctx, err.Error())
		s.monitor.emit(Event{SubscriptionID: s.id, Snapshot: Snapshot{Point: s.point, Timestamp: ts}, Err: err})
		return
	}

	snapshot := Snapshot{
		Point:     s.point,
		Value:     values[0],
		Timestamp: ts,
	}

	event := Event{
		SubscriptionID: s.id,
		Snapshot:       snapshot,
		Changed:        true,
	}

	if s.detector != nil && s.prev != nil {
		event.Changed = s.detector.Detect(*s.prev, snapshot)
	}
	s.prev = &snapshot

	if event.Err == nil && s.handler != nil {
		s.handler(snapshot)
	}

	s.monitor.emit(event)
}
