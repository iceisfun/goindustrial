package logging

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultLogger is a structured logger that writes formatted messages to an io.Writer.
type DefaultLogger struct {
	mu     sync.Mutex
	level  Level
	writer io.Writer
	fields map[string]any
}

// Option configures a DefaultLogger.
type Option func(*DefaultLogger)

// WithLevel sets the log level.
func WithLevel(level Level) Option {
	return func(l *DefaultLogger) {
		l.level = level
	}
}

// WithWriter sets the output writer.
func WithWriter(writer io.Writer) Option {
	return func(l *DefaultLogger) {
		l.writer = writer
	}
}

// WithLogFields adds fields to the logger.
func WithLogFields(fields map[string]any) Option {
	return func(l *DefaultLogger) {
		if l.fields == nil {
			l.fields = make(map[string]any)
		}
		maps.Copy(l.fields, fields)
	}
}

// NewDefaultLogger creates a new DefaultLogger with the given options.
// Defaults to LevelInfo writing to os.Stdout.
func NewDefaultLogger(options ...Option) *DefaultLogger {
	logger := &DefaultLogger{
		level:  LevelInfo,
		writer: os.Stdout,
		fields: make(map[string]any),
	}
	for _, option := range options {
		option(logger)
	}
	return logger
}

func (l *DefaultLogger) Trace(ctx context.Context, format string, args ...any) {
	if l.level <= LevelTrace {
		l.log("TRACE", format, args...)
	}
}

func (l *DefaultLogger) Debug(ctx context.Context, format string, args ...any) {
	if l.level <= LevelDebug {
		l.log("DEBUG", format, args...)
	}
}

func (l *DefaultLogger) Info(ctx context.Context, format string, args ...any) {
	if l.level <= LevelInfo {
		l.log("INFO", format, args...)
	}
}

func (l *DefaultLogger) Warn(ctx context.Context, format string, args ...any) {
	if l.level <= LevelWarn {
		l.log("WARN", format, args...)
	}
}

func (l *DefaultLogger) Error(ctx context.Context, format string, args ...any) {
	if l.level <= LevelError {
		l.log("ERROR", format, args...)
	}
}

func (l *DefaultLogger) WithFields(fields map[string]any) Logger {
	return NewDefaultLogger(
		WithLevel(l.level),
		WithWriter(l.writer),
		WithLogFields(l.fields),
		WithLogFields(fields),
	)
}

func (l *DefaultLogger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

func (l *DefaultLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Hexdump outputs a hex dump of the given data at TRACE level.
func (l *DefaultLogger) Hexdump(ctx context.Context, data []byte) {
	if l.level > LevelTrace {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format(time.RFC3339)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] TRACE: HEXDUMP\n", timestamp)
	b.WriteString("offset   00 01 02 03 04 05 06 07 | 08 09 0a 0b 0c 0d 0e 0f\n")

	for i := 0; i < len(data); i += 16 {
		fmt.Fprintf(&b, "%08x", i)
		for j := 0; j < 16; j++ {
			if j == 8 {
				b.WriteString(" |")
			}
			b.WriteByte(' ')
			if i+j < len(data) {
				fmt.Fprintf(&b, "%02x", data[i+j])
			} else {
				b.WriteString("  ")
			}
		}
		b.WriteByte('\n')
	}

	l.writeFields(&b)
	fmt.Fprint(l.writer, b.String())
}

func (l *DefaultLogger) log(level, format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().Format(time.RFC3339)
	message := fmt.Sprintf(format, args...)

	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s: %s", timestamp, level, message)
	l.writeFields(&b)

	if b.Len() == 0 || b.String()[b.Len()-1] != '\n' {
		b.WriteByte('\n')
	}

	_, err := fmt.Fprint(l.writer, b.String())
	if err != nil && l.writer != os.Stderr {
		fmt.Fprintf(os.Stderr, "ERROR: Failed to write log entry: %v\n", err)
	}
}

func (l *DefaultLogger) writeFields(b *strings.Builder) {
	if len(l.fields) == 0 {
		return
	}
	fieldStrings := make([]string, 0, len(l.fields))
	for k, v := range l.fields {
		fieldStrings = append(fieldStrings, fmt.Sprintf("%s=%q", k, fmt.Sprintf("%v", v)))
	}
	b.WriteByte(' ')
	b.WriteString(strings.Join(fieldStrings, " "))
}
