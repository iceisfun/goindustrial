package logging

import "context"

// NopLogger is a logger that silently discards all messages.
type NopLogger struct{}

// NewNopLogger creates a new NopLogger.
func NewNopLogger() *NopLogger {
	return &NopLogger{}
}

func (l *NopLogger) Trace(ctx context.Context, format string, args ...any) {}
func (l *NopLogger) Debug(ctx context.Context, format string, args ...any) {}
func (l *NopLogger) Info(ctx context.Context, format string, args ...any)  {}
func (l *NopLogger) Warn(ctx context.Context, format string, args ...any)  {}
func (l *NopLogger) Error(ctx context.Context, format string, args ...any) {}

func (l *NopLogger) WithFields(fields map[string]any) Logger { return l }
func (l *NopLogger) GetLevel() Level                         { return LevelNone }
func (l *NopLogger) SetLevel(level Level)                    {}
