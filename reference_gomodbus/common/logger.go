package common

import "context"

// LogLevel represents a logging level.
type LogLevel int

const (
	// LevelTrace is the most verbose logging level.
	LevelTrace LogLevel = iota
	// LevelDebug is the most verbose logging level.
	LevelDebug
	// LevelInfo is for general information.
	LevelInfo
	// LevelWarn is for warnings.
	LevelWarn
	// LevelError is for errors.
	LevelError
	// LevelNone disables all logging.
	LevelNone
)

// LoggerInterface defines the interface for a logger.
type LoggerInterface interface {
	Trace(ctx context.Context, format string, args ...interface{})
	// Debug logs a debug message.
	Debug(ctx context.Context, format string, args ...interface{})
	// Info logs an info message.
	Info(ctx context.Context, format string, args ...interface{})
	// Warn logs a warning message.
	Warn(ctx context.Context, format string, args ...interface{})
	// Error logs an error message.
	Error(ctx context.Context, format string, args ...interface{})
	// WithFields returns a new logger with the given fields.
	WithFields(fields map[string]interface{}) LoggerInterface
	// GetLevel returns the current log level.
	GetLevel() LogLevel
	// SetLevel sets the log level.
	SetLevel(level LogLevel)
}

type LoggerInterfaceHexdump interface {
	// Hexdump logs a hexdump of the given data.
	// optional interface for extra verbose protocol debug
	Hexdump(context.Context, []byte)
}
