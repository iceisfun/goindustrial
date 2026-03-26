package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDefaultLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(WithLevel(LevelWarn), WithWriter(&buf))

	ctx := context.Background()
	logger.Debug(ctx, "should not appear")
	logger.Info(ctx, "should not appear")
	logger.Warn(ctx, "warning message")
	logger.Error(ctx, "error message")

	output := buf.String()
	if strings.Contains(output, "DEBUG") {
		t.Error("debug message should not appear at warn level")
	}
	if strings.Contains(output, "INFO") {
		t.Error("info message should not appear at warn level")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("warn message should appear at warn level")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("error message should appear at warn level")
	}
}

func TestDefaultLoggerFormatting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(WithLevel(LevelTrace), WithWriter(&buf))

	ctx := context.Background()
	logger.Info(ctx, "hello %s %d", "world", 42)

	output := buf.String()
	if !strings.Contains(output, "hello world 42") {
		t.Errorf("expected formatted message, got: %s", output)
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected INFO level label, got: %s", output)
	}
}

func TestDefaultLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(WithLevel(LevelInfo), WithWriter(&buf))

	child := logger.WithFields(map[string]any{"component": "transport"})
	child.Info(context.Background(), "connected")

	output := buf.String()
	if !strings.Contains(output, "component=") {
		t.Errorf("expected fields in output, got: %s", output)
	}
	if !strings.Contains(output, "transport") {
		t.Errorf("expected field value in output, got: %s", output)
	}
}

func TestDefaultLoggerSetLevel(t *testing.T) {
	logger := NewDefaultLogger(WithLevel(LevelInfo))

	if logger.GetLevel() != LevelInfo {
		t.Errorf("expected LevelInfo, got %v", logger.GetLevel())
	}

	logger.SetLevel(LevelError)
	if logger.GetLevel() != LevelError {
		t.Errorf("expected LevelError, got %v", logger.GetLevel())
	}
}

func TestNopLoggerImplementsInterface(t *testing.T) {
	var l Logger = NewNopLogger()
	ctx := context.Background()

	// Should not panic.
	l.Trace(ctx, "trace")
	l.Debug(ctx, "debug")
	l.Info(ctx, "info")
	l.Warn(ctx, "warn")
	l.Error(ctx, "error")
	l.WithFields(map[string]any{"k": "v"})

	if l.GetLevel() != LevelNone {
		t.Errorf("expected LevelNone, got %v", l.GetLevel())
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelTrace, "TRACE"},
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelNone, "NONE"},
		{Level(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestDefaultLoggerHexdump(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(WithLevel(LevelTrace), WithWriter(&buf))

	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	logger.Hexdump(context.Background(), data)

	output := buf.String()
	if !strings.Contains(output, "HEXDUMP") {
		t.Error("expected HEXDUMP header")
	}
	if !strings.Contains(output, "00 01 02 03") {
		t.Errorf("expected hex bytes in output, got: %s", output)
	}
}
