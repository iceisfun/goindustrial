package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/iceisfun/goindustrial/logging"
)

// MockLogger implements logging.Logger and records every log message so tests
// can assert on log output. It is safe for concurrent use.
type MockLogger struct {
	mu       sync.Mutex
	Messages []LogMessage
	level    logging.Level
}

// LogMessage is a single recorded log entry with its severity level and
// formatted message text.
type LogMessage struct {
	Level   logging.Level
	Message string
}

// NewMockLogger creates a MockLogger that captures all messages at and above
// LevelTrace (i.e., everything).
func NewMockLogger() *MockLogger {
	return &MockLogger{level: logging.LevelTrace}
}

// Trace records a trace-level message.
func (l *MockLogger) Trace(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelTrace, format, args...)
}

// Debug records a debug-level message.
func (l *MockLogger) Debug(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelDebug, format, args...)
}

// Info records an info-level message.
func (l *MockLogger) Info(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelInfo, format, args...)
}

// Warn records a warn-level message.
func (l *MockLogger) Warn(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelWarn, format, args...)
}

// Error records an error-level message.
func (l *MockLogger) Error(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelError, format, args...)
}

// WithFields returns the same MockLogger (fields are ignored in tests).
func (l *MockLogger) WithFields(fields map[string]any) logging.Logger { return l }

// GetLevel returns the current minimum log level.
func (l *MockLogger) GetLevel() logging.Level { return l.level }

// SetLevel sets the minimum log level. Messages below this level are discarded.
func (l *MockLogger) SetLevel(level logging.Level) { l.level = level }

func (l *MockLogger) record(level logging.Level, format string, args ...any) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.Messages = append(l.Messages, LogMessage{Level: level, Message: msg})
}
