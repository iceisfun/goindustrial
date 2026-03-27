// Package logging defines a structured logging interface for goindustrial
// components. It provides the [Logger] interface that all protocol packages
// accept, a [DefaultLogger] that writes timestamped messages to an [io.Writer],
// and a [NopLogger] that silently discards output.
//
// Callers who already use a third-party logging library can implement the
// [Logger] interface to bridge goindustrial log output into their existing
// infrastructure.
package logging

import "context"

// Level represents a logging level.
type Level int

const (
	// LevelTrace is the most verbose logging level.
	LevelTrace Level = iota
	// LevelDebug is for debug messages.
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

// String returns the human-readable name of the log level.
func (l Level) String() string {
	switch l {
	case LevelTrace:
		return "TRACE"
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

// Logger defines the common logging interface for all goindustrial components.
// Implementations must be safe for concurrent use. All formatting methods
// accept a printf-style format string and arguments.
type Logger interface {
	// Trace logs a message at the most verbose level, useful for
	// protocol-level byte tracing and detailed diagnostics.
	Trace(ctx context.Context, format string, args ...any)

	// Debug logs a message useful during development and troubleshooting.
	Debug(ctx context.Context, format string, args ...any)

	// Info logs general operational information such as connection events.
	Info(ctx context.Context, format string, args ...any)

	// Warn logs a message indicating a potential problem that does not
	// prevent normal operation.
	Warn(ctx context.Context, format string, args ...any)

	// Error logs a message indicating a failure.
	Error(ctx context.Context, format string, args ...any)

	// WithFields returns a new Logger that includes the given structured
	// fields in every subsequent log entry.
	WithFields(fields map[string]any) Logger

	// GetLevel returns the current minimum log level.
	GetLevel() Level

	// SetLevel changes the minimum log level at runtime.
	SetLevel(level Level)
}

// HexdumpLogger is an optional extension of [Logger] for protocol-level hex
// dump debugging. Protocol implementations may check whether their Logger
// also satisfies HexdumpLogger and, if so, dump raw frame bytes for
// wire-level diagnostics.
type HexdumpLogger interface {
	// Hexdump outputs a formatted hex dump of data. Implementations should
	// gate output on the current log level (typically TRACE).
	Hexdump(ctx context.Context, data []byte)
}
