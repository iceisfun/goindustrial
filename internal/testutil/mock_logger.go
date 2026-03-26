package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/iceisfun/goindustrial/logging"
)

// MockLogger records all log messages for test assertions.
type MockLogger struct {
	mu       sync.Mutex
	Messages []LogMessage
	level    logging.Level
}

// LogMessage is a single recorded log entry.
type LogMessage struct {
	Level   logging.Level
	Message string
}

// NewMockLogger creates a MockLogger that captures all messages at LevelTrace.
func NewMockLogger() *MockLogger {
	return &MockLogger{level: logging.LevelTrace}
}

func (l *MockLogger) Trace(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelTrace, format, args...)
}

func (l *MockLogger) Debug(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelDebug, format, args...)
}

func (l *MockLogger) Info(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelInfo, format, args...)
}

func (l *MockLogger) Warn(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelWarn, format, args...)
}

func (l *MockLogger) Error(ctx context.Context, format string, args ...any) {
	l.record(logging.LevelError, format, args...)
}

func (l *MockLogger) WithFields(fields map[string]any) logging.Logger { return l }
func (l *MockLogger) GetLevel() logging.Level                         { return l.level }
func (l *MockLogger) SetLevel(level logging.Level)                    { l.level = level }

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
