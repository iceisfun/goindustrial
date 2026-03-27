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

// SubscriptionOption configures a subscription created by [Monitor.Subscribe].
type SubscriptionOption func(*subConfig) error

type subConfig struct {
	frequency    time.Duration
	readVariance time.Duration
	handler      Handler
	detector     ChangeDetector
	immediate    bool
}

func defaultSubConfig() *subConfig {
	return &subConfig{
		frequency: 500 * time.Millisecond,
		immediate: true,
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

// WithInitialRead controls whether a subscription performs an immediate read
// when created, before waiting for the first poll interval. The default is
// true.
func WithInitialRead(enabled bool) SubscriptionOption {
	return func(cfg *subConfig) error {
		cfg.immediate = enabled
		return nil
	}
}
