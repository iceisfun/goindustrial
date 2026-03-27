package logging

import "context"

// NopLogger is a [Logger] implementation that silently discards all messages.
// It is useful as a default when no logging is desired. Create one with
// [NewNopLogger].
type NopLogger struct{}

// NewNopLogger creates a new NopLogger.
func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

// Trace is a no-op.
func (l *NopLogger) Trace(ctx context.Context, format string, args ...any) {}

// Debug is a no-op.
func (l *NopLogger) Debug(ctx context.Context, format string, args ...any) {}

// Info is a no-op.
func (l *NopLogger) Info(ctx context.Context, format string, args ...any) {}

// Warn is a no-op.
func (l *NopLogger) Warn(ctx context.Context, format string, args ...any) {}

// Error is a no-op.
func (l *NopLogger) Error(ctx context.Context, format string, args ...any) {}

// WithFields returns the receiver unchanged since no fields are recorded.
func (l *NopLogger) WithFields(fields map[string]any) Logger { return l }

// GetLevel always returns LevelNone.
func (l *NopLogger) GetLevel() Level { return LevelNone }

// SetLevel is a no-op.
func (l *NopLogger) SetLevel(level Level) {}
