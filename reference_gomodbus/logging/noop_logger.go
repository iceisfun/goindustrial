package logging

import (
	"context"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// NoopLogger is a logger that does nothing
type NoopLogger struct{}

// NewNoopLogger creates a new NoopLogger
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// Trace implements the LoggerInterface Trace method
func (l *NoopLogger) Trace(ctx context.Context, format string, args ...interface{}) {
	// Do nothing
}

// Debug implements the LoggerInterface Debug method
func (l *NoopLogger) Debug(ctx context.Context, format string, args ...interface{}) {
	// Do nothing
}

// Info implements the LoggerInterface Info method
func (l *NoopLogger) Info(ctx context.Context, format string, args ...interface{}) {
	// Do nothing
}

// Warn implements the LoggerInterface Warn method
func (l *NoopLogger) Warn(ctx context.Context, format string, args ...interface{}) {
	// Do nothing
}

// Error implements the LoggerInterface Error method
func (l *NoopLogger) Error(ctx context.Context, format string, args ...interface{}) {
	// Do nothing
}

// WithFields implements the LoggerInterface WithFields method
func (l *NoopLogger) WithFields(fields map[string]interface{}) common.LoggerInterface {
	return l
}

// GetLevel implements the LoggerInterface GetLevel method
func (l *NoopLogger) GetLevel() common.LogLevel {
	return common.LevelNone
}

// SetLevel implements the LoggerInterface SetLevel method
func (l *NoopLogger) SetLevel(level common.LogLevel) {
	// Do nothing
}

