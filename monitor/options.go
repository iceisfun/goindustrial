package monitor

import (
	"fmt"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// MonitorOption configures a [Monitor] created by [NewMonitor].
type MonitorOption func(*monitorConfig)

type monitorConfig struct {
	logger      logging.Logger
	eventBuffer int
	connectedCh <-chan struct{}
}

// WithLogger sets the logger used by the monitor for warnings and diagnostics.
// When not set, a no-op logger is used.
func WithLogger(logger logging.Logger) MonitorOption {
	return func(cfg *monitorConfig) {
		cfg.logger = logger
	}
}

// WithEventBuffer sets the capacity of the shared event channel returned by
// [Monitor.Events]. The default buffer size is 64. Values less than or equal
// to zero are clamped to 1.
func WithEventBuffer(size int) MonitorOption {
	return func(cfg *monitorConfig) {
		if size <= 0 {
			size = 1
		}
		cfg.eventBuffer = size
	}
}

// WithConnected provides a channel that gates subscription polling. When set,
// each subscription blocks until the channel is closed before performing its
// initial read. This is typically wired to a [transport.WithOnConnect] callback
// so that monitors wait for the PLC connection before polling:
//
//	connected := make(chan struct{})
//	tp := transport.NewReconnectingTransport(connector, closer,
//	    transport.WithOnConnect(func() { close(connected) }),
//	)
//	mon, _ := monitor.NewMonitor(reader, monitor.WithConnected(connected))
func WithConnected(ch <-chan struct{}) MonitorOption {
	return func(cfg *monitorConfig) {
		cfg.connectedCh = ch
	}
}

// SubscriptionOption configures a subscription created by [Monitor.Subscribe].
type SubscriptionOption func(*subConfig) error

type subConfig struct {
	frequency    time.Duration
	readVariance time.Duration
	handler      Handler
	detector     ChangeDetector
	initialDelay time.Duration
}

func defaultSubConfig() *subConfig {
	return &subConfig{
		frequency:    500 * time.Millisecond,
		initialDelay: 50 * time.Millisecond,
	}
}

// WithFrequency sets the poll interval for a subscription. The default is
// 500ms. The value must be positive.
func WithFrequency(freq time.Duration) SubscriptionOption {
	return func(cfg *subConfig) error {
		if freq <= 0 {
			return fmt.Errorf("frequency must be positive")
		}
		cfg.frequency = freq
		return nil
	}
}

// WithReadVariance adds random jitter to each poll cycle to prevent multiple
// subscriptions from issuing reads at the exact same instant. The actual delay
// is frequency +/- a uniformly random value in [-variance, +variance].
func WithReadVariance(variance time.Duration) SubscriptionOption {
	return func(cfg *subConfig) error {
		if variance < 0 {
			return fmt.Errorf("read variance cannot be negative")
		}
		cfg.readVariance = variance
		return nil
	}
}

// WithChangeDetector attaches a change detector to the subscription.
// When set, Event.Changed reflects whether the detector reported a change.
func WithChangeDetector(detector ChangeDetector) SubscriptionOption {
	return func(cfg *subConfig) error {
		if detector == nil {
			return fmt.Errorf("change detector cannot be nil")
		}
		cfg.detector = detector
		return nil
	}
}

// WithHandler registers a [Handler] callback that is invoked after each
// successful poll. The handler runs in the subscription's goroutine.
func WithHandler(handler Handler) SubscriptionOption {
	return func(cfg *subConfig) error {
		if handler == nil {
			return fmt.Errorf("handler cannot be nil")
		}
		cfg.handler = handler
		return nil
	}
}

// WithInitialRead sets the delay before the first read after a subscription
// is created. Zero means the first read happens immediately; a positive
// duration defers it by that amount. The default is 50ms. Negative values
// are rejected.
func WithInitialRead(delay time.Duration) SubscriptionOption {
	return func(cfg *subConfig) error {
		if delay < 0 {
			return fmt.Errorf("initial read delay cannot be negative")
		}
		cfg.initialDelay = delay
		return nil
	}
}
