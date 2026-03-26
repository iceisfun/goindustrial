package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// Logger implements the common.LoggerInterface and common.LoggerInterfaceHexdump
type Logger struct {
	mu     sync.Mutex
	level  common.LogLevel
	writer io.Writer
	fields map[string]interface{}
}

// Option is a function that configures a Logger
type Option func(*Logger)

// WithLevel sets the log level
func WithLevel(level common.LogLevel) Option {
	return func(l *Logger) {
		l.level = level
	}
}

// WithWriter sets the writer for the logger
func WithWriter(writer io.Writer) Option {
	return func(l *Logger) {
		l.writer = writer
	}
}

// WithFields adds fields to the logger
func WithFields(fields map[string]interface{}) Option {
	return func(l *Logger) {
		// Create a new map if it doesn't exist
		if l.fields == nil {
			l.fields = make(map[string]interface{})
		}
		// Copy the fields
		for k, v := range fields {
			l.fields[k] = v
		}
	}
}

// Hexdump outputs a hexdump of the given data at TRACE level
// Format: offset   00 01 02 03 04 05 06 07 | 08 09 0a 0b 0c 0d 0e 0f
func (l *Logger) Hexdump(ctx context.Context, data []byte) {
	// Only hexdump if the logger is at TRACE level
	if l.level > common.LevelTrace {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Get current time
	timestamp := time.Now().Format(time.RFC3339)

	// Create the header
	header := fmt.Sprintf("[%s] TRACE: HEXDUMP\n", timestamp)

	// First line is the header for the columns
	hexdump := "offset   00 01 02 03 04 05 06 07 | 08 09 0a 0b 0c 0d 0e 0f\n"

	// Process the data 16 bytes at a time
	for i := 0; i < len(data); i += 16 {
		// Print the offset
		hexdump += fmt.Sprintf("%08x", i)

		// Print the hex values
		for j := 0; j < 16; j++ {
			// Add separator between left and right halves
			if j == 8 {
				hexdump += " |"
			}

			// Add space before each byte
			hexdump += " "

			// If we still have data, print it
			if i+j < len(data) {
				hexdump += fmt.Sprintf("%02x", data[i+j])
			} else {
				// Otherwise, print spaces to maintain alignment
				hexdump += "  "
			}
		}

		// Add newline at the end of each row
		hexdump += "\n"
	}

	// Add fields if any exist
	fieldsStr := ""
	if len(l.fields) > 0 {
		fieldStrings := make([]string, 0, len(l.fields))
		for k, v := range l.fields {
			fieldStrings = append(fieldStrings, fmt.Sprintf("%s=%q", k, fmt.Sprintf("%v", v)))
		}
		fieldsStr = " " + strings.Join(fieldStrings, " ")
	}

	// Write to the output
	output := header + hexdump
	if fieldsStr != "" {
		output += fieldsStr + "\n"
	}

	_, err := fmt.Fprint(l.writer, output)
	if err != nil {
		// Since we can't log the error (that would cause a recursive loop),
		// we'll write directly to stderr as a last resort
		if l.writer != os.Stderr {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to write hexdump: %v\n", err)
		}
	}
}

// NewLogger creates a new logger with the given options
func NewLogger(options ...Option) *Logger {
	// Default logger writes to stdout with info level
	logger := &Logger{
		level:  common.LevelInfo,
		writer: os.Stdout,
		fields: make(map[string]interface{}),
	}

	// Apply options
	for _, option := range options {
		option(logger)
	}

	return logger
}

// Trace logs a trace message
func (l *Logger) Trace(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelTrace {
		l.log(ctx, "TRACE", format, args...)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelDebug {
		l.log(ctx, "DEBUG", format, args...)
	}
}

// Info logs an info message
func (l *Logger) Info(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelInfo {
		l.log(ctx, "INFO", format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelWarn {
		l.log(ctx, "WARN", format, args...)
	}
}

// Error logs an error message
func (l *Logger) Error(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelError {
		l.log(ctx, "ERROR", format, args...)
	}
}

// WithFields returns a new logger with the given fields
func (l *Logger) WithFields(fields map[string]interface{}) common.LoggerInterface {
	// Return the concrete Logger type which implements both interfaces
	return NewLogger(
		WithLevel(l.level),
		WithWriter(l.writer),
		WithFields(l.fields),    // Copy existing fields
		WithFields(fields),      // Add new fields
	)
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() common.LogLevel {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level common.LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log is an internal method that handles the actual logging
func (l *Logger) log(ctx context.Context, level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Get current time
	timestamp := time.Now().Format(time.RFC3339)

	// Format the message
	message := fmt.Sprintf(format, args...)

	// Build the log entry
	entry := fmt.Sprintf("[%s] %s: %s", timestamp, level, message)

	// Add context-specific fields if any
	if len(l.fields) > 0 {
		// Format fields in a more machine-parseable way: key="value" key2="value2"
		fieldStrings := make([]string, 0, len(l.fields))
		for k, v := range l.fields {
			// Format key="value" with proper string escaping
			fieldStrings = append(fieldStrings, fmt.Sprintf("%s=%q", k, fmt.Sprintf("%v", v)))
		}
		entry += " " + strings.Join(fieldStrings, " ")
	}

	// Add a newline if not already present
	if entry[len(entry)-1] != '\n' {
		entry += "\n"
	}

	// Write to the output and handle potential errors
	_, err := fmt.Fprint(l.writer, entry)
	if err != nil {
		// Since we can't log the error (that would cause a recursive loop),
		// we'll write directly to stderr as a last resort
		if l.writer != os.Stderr {
			fmt.Fprintf(os.Stderr, "ERROR: Failed to write log entry: %v\n", err)
		}
		// In a production environment, you might want to have a mechanism to
		// report these errors or switch to an alternative logger
	}
}