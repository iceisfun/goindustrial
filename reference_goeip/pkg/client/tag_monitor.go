package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/cip"
)

// ErrMonitorClosed is returned when an operation targets a stopped TagMonitor.
var ErrMonitorClosed = errors.New("tag monitor is closed")

// TagReader is the minimal interface needed by TagMonitor to fetch tag values.
type TagReader interface {
	ReadTag(tagName string) ([]byte, error)
}

// MonitorOption configures a TagMonitor instance.
type MonitorOption func(*monitorConfig)

type monitorConfig struct {
	reader      TagReader
	logger      internal.Logger
	eventBuffer int
}

// WithMonitorLogger overrides the logger used by the monitor.
func WithMonitorLogger(logger internal.Logger) MonitorOption {
	return func(cfg *monitorConfig) {
		cfg.logger = logger
	}
}

// WithMonitorReader injects a custom reader implementation, primarily for tests.
func WithMonitorReader(reader TagReader) MonitorOption {
	return func(cfg *monitorConfig) {
		cfg.reader = reader
	}
}

// WithEventBuffer configures the size of the event channel buffer.
func WithEventBuffer(size int) MonitorOption {
	return func(cfg *monitorConfig) {
		if size <= 0 {
			size = 1
		}
		cfg.eventBuffer = size
	}
}

// TagMonitor polls one or more CIP tags on a schedule and emits events.
type TagMonitor struct {
	client *Client
	reader TagReader
	logger internal.Logger

	mu      sync.RWMutex
	subs    map[int64]*tagSubscription
	closed  bool
	nextID  int64
	stopCh  chan struct{}
	events  chan TagEvent
	closeMx sync.Once
	wg      sync.WaitGroup
}

// NewTagMonitor creates a monitor bound to the provided client.
// A custom reader may be supplied via WithMonitorReader for tests.
func NewTagMonitor(client *Client, opts ...MonitorOption) (*TagMonitor, error) {
	cfg := monitorConfig{eventBuffer: 64}
	if client != nil {
		cfg.reader = client
		cfg.logger = client.logger
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.reader == nil {
		return nil, errors.New("tag monitor requires a client or custom reader")
	}

	if cfg.logger == nil {
		cfg.logger = internal.NopLogger()
	}

	m := &TagMonitor{
		client: client,
		reader: cfg.reader,
		logger: cfg.logger,
		subs:   make(map[int64]*tagSubscription),
		stopCh: make(chan struct{}),
		events: make(chan TagEvent, cfg.eventBuffer),
	}
	return m, nil
}

// Client returns the underlying client when available. It can be nil when using a custom reader.
func (m *TagMonitor) Client() *Client {
	return m.client
}

// Wait exposes the receive-only event stream emitted by the monitor.
// Close closes the channel; consumers should stop upon channel closure.
func (m *TagMonitor) Wait() <-chan TagEvent {
	return m.events
}

// Close stops the monitor and all active subscriptions.
func (m *TagMonitor) Close() {
	m.closeMx.Do(func() {
		close(m.stopCh)

		m.mu.Lock()
		subs := make([]*tagSubscription, 0, len(m.subs))
		for _, sub := range m.subs {
			subs = append(subs, sub)
		}
		m.closed = true
		m.subs = make(map[int64]*tagSubscription)
		m.mu.Unlock()

		for _, sub := range subs {
			sub.stop()
		}

		m.wg.Wait()
		close(m.events)
	})
}

// AddTag registers a tag to poll. Frequency can be tuned with WithFrequency and
// handlers added through additional tag options.
func (m *TagMonitor) AddTag(name string, opts ...TagOption) (*TagSubscription, error) {
	if name == "" {
		return nil, errors.New("tag name is required")
	}

	cfg := defaultTagConfig()
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
	sub := newTagSubscription(id, name, *cfg, m)
	m.subs[id] = sub
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		sub.run()
	}()

	return &TagSubscription{monitor: m, id: id}, nil
}

func (m *TagMonitor) removeSubscription(id int64) {
	m.mu.Lock()
	sub, ok := m.subs[id]
	if ok {
		delete(m.subs, id)
	}
	m.mu.Unlock()

	if ok {
		sub.stop()
	}
}

func (m *TagMonitor) emit(event TagEvent) {
	select {
	case <-m.stopCh:
		return
	default:
	}

	select {
	case <-m.stopCh:
	case m.events <- event:
	default:
	}
}

// TagSubscription represents a running polling routine. Stop should be called
// when a subscription is no longer needed to free resources.
type TagSubscription struct {
	monitor *TagMonitor
	id      int64
	once    sync.Once
}

// ID returns the subscription identifier used in events.
func (s *TagSubscription) ID() int64 {
	return s.id
}

// Stop cancels the subscription.
func (s *TagSubscription) Stop() {
	if s.monitor == nil {
		return
	}

	s.once.Do(func() {
		s.monitor.removeSubscription(s.id)
	})
}

// TagEvent represents the result of a polling cycle.
type TagEvent struct {
	SubscriptionID int64
	Snapshot       TagSnapshot
	Err            error
	Changed        bool
}

// TagSnapshot encapsulates the latest value of a tag.
type TagSnapshot struct {
	Name      string
	Timestamp time.Time
	Type      cip.UINT
	Payload   []byte
}

// Into unmarshals the payload into the provided destination.
func (s TagSnapshot) Into(dst any) error {
	return cip.Unmarshal(s.Payload, dst)
}

// Refreshable models user-defined state that can be updated by tag snapshots.
type Refreshable interface {
	Refresh(snapshot TagSnapshot) (changed bool, err error)
}

// TagHandler is invoked after a successful refresh and before an event is dispatched.
type TagHandler func(snapshot TagSnapshot)

type tagSubscription struct {
	id           int64
	name         string
	frequency    time.Duration
	readVariance time.Duration
	handler      TagHandler
	refreshable  Refreshable
	immediate    bool

	monitor *TagMonitor

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func newTagSubscription(id int64, name string, cfg tagConfig, monitor *TagMonitor) *tagSubscription {
	return &tagSubscription{
		id:           id,
		name:         name,
		frequency:    cfg.frequency,
		readVariance: cfg.readVariance,
		handler:      cfg.handler,
		refreshable:  cfg.refreshable,
		immediate:    cfg.immediate,
		monitor:      monitor,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

func (s *tagSubscription) run() {
	defer close(s.doneCh)

	if s.immediate {
		s.poll()
	}

	for {
		delay := s.frequency
		if s.readVariance > 0 {
			// Add random variance in range [-readVariance, +readVariance]
			variance := time.Duration(rand.Int63n(int64(s.readVariance)*2+1)) - s.readVariance
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

func (s *tagSubscription) stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
	})
}

func (s *tagSubscription) poll() {
	ts := time.Now()
	data, err := s.monitor.reader.ReadTag(s.name)
	if err != nil {
		s.monitor.logger.Warnf("tag monitor read failed for %s: %v", s.name, err)
		s.monitor.emit(TagEvent{SubscriptionID: s.id, Snapshot: TagSnapshot{Name: s.name, Timestamp: ts}, Err: err})
		return
	}

	if len(data) < 2 {
		err := fmt.Errorf("tag monitor: response for %s too short", s.name)
		s.monitor.logger.Warnf(err.Error())
		s.monitor.emit(TagEvent{SubscriptionID: s.id, Snapshot: TagSnapshot{Name: s.name, Timestamp: ts}, Err: err})
		return
	}

	payload := append([]byte(nil), data[2:]...)
	typeCode := cip.UINT(binary.LittleEndian.Uint16(data[:2]))

	snapshot := TagSnapshot{
		Name:      s.name,
		Timestamp: ts,
		Type:      typeCode,
		Payload:   payload,
	}

	event := TagEvent{
		SubscriptionID: s.id,
		Snapshot:       snapshot,
		Changed:        true,
	}

	if s.refreshable != nil {
		changed, err := s.refreshable.Refresh(snapshot)
		if err != nil {
			s.monitor.logger.Warnf("tag monitor refresh failed for %s: %v", s.name, err)
			event.Err = err
		}
		event.Changed = changed
	}

	if event.Err == nil && s.handler != nil {
		s.handler(snapshot)
	}

	s.monitor.emit(event)
}

type TagOption func(*tagConfig) error

type tagConfig struct {
	frequency    time.Duration
	readVariance time.Duration
	handler      TagHandler
	refreshable  Refreshable
	immediate    bool
}

func defaultTagConfig() *tagConfig {
	return &tagConfig{
		frequency: 500 * time.Millisecond,
		immediate: true,
	}
}

// WithFrequency configures the poll interval for a tag subscription.
func WithFrequency(freq time.Duration) TagOption {
	return func(cfg *tagConfig) error {
		if freq <= 0 {
			return fmt.Errorf("frequency must be positive")
		}
		cfg.frequency = freq
		return nil
	}
}

// WithReadVariance adds random timing variance to each poll cycle.
// The actual delay will be frequency +/- a random value up to variance.
// This helps spread out reads when multiple subscriptions would otherwise
// poll at the same time, reducing load on the PLC.
func WithReadVariance(variance time.Duration) TagOption {
	return func(cfg *tagConfig) error {
		if variance < 0 {
			return fmt.Errorf("read variance cannot be negative")
		}
		cfg.readVariance = variance
		return nil
	}
}

// WithRefreshable attaches state that is updated each time the tag is polled.
func WithRefreshable(r Refreshable) TagOption {
	return func(cfg *tagConfig) error {
		if r == nil {
			return fmt.Errorf("refreshable cannot be nil")
		}
		cfg.refreshable = r
		return nil
	}
}

// WithHandler registers a callback that executes after a successful poll.
func WithHandler(handler TagHandler) TagOption {
	return func(cfg *tagConfig) error {
		if handler == nil {
			return fmt.Errorf("handler cannot be nil")
		}
		cfg.handler = handler
		return nil
	}
}

// WithInitialRead toggles whether a subscription performs an immediate read when created.
func WithInitialRead(enabled bool) TagOption {
	return func(cfg *tagConfig) error {
		cfg.immediate = enabled
		return nil
	}
}
