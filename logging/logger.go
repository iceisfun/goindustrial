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
type Logger interface {
	Trace(ctx context.Context, format string, args ...any)
	Debug(ctx context.Context, format string, args ...any)
	Info(ctx context.Context, format string, args ...any)
	Warn(ctx context.Context, format string, args ...any)
	Error(ctx context.Context, format string, args ...any)
	WithFields(fields map[string]any) Logger
	GetLevel() Level
	SetLevel(level Level)
}

// HexdumpLogger is an optional interface for protocol-level hex dump debugging.
type HexdumpLogger interface {
	Hexdump(ctx context.Context, data []byte)
}
