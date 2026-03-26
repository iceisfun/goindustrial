package monitor

import (
	"fmt"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// MonitorOption configures a Monitor instance.
type MonitorOption func(*monitorConfig)

type monitorConfig struct {
	logger      logging.Logger
	eventBuffer int
}

// WithLogger overrides the logger used by the monitor.
func WithLogger(logger logging.Logger) MonitorOption {
	return func(cfg *monitorConfig) {
		cfg.logger = logger
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

// SubscriptionOption configures a subscription.
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

// WithFrequency configures the poll interval for a subscription.
func WithFrequency(freq time.Duration) SubscriptionOption {
	return func(cfg *subConfig) error {
		if freq <= 0 {
			return fmt.Errorf("frequency must be positive")
		}
		cfg.frequency = freq
		return nil
	}
}

// WithReadVariance adds random timing variance to each poll cycle.
// The actual delay will be frequency +/- a random value up to variance.
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

// WithHandler registers a callback that executes after a successful poll.
func WithHandler(handler Handler) SubscriptionOption {
	return func(cfg *subConfig) error {
		if handler == nil {
			return fmt.Errorf("handler cannot be nil")
		}
		cfg.handler = handler
		return nil
	}
}

// WithInitialRead toggles whether a subscription performs an immediate read when created.
func WithInitialRead(enabled bool) SubscriptionOption {
	return func(cfg *subConfig) error {
		cfg.immediate = enabled
		return nil
	}
}
