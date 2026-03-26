package internal

import (
	"log"
	"os"
)

type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

func NopLogger() Logger {
	return nopLogger{}
}

type ConsoleLogger struct {
	logger *log.Logger
}

func NewConsoleLogger() Logger {
	return &ConsoleLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *ConsoleLogger) Debugf(format string, args ...any) {
	l.logger.Printf("[DEBUG] "+format, args...)
}

func (l *ConsoleLogger) Infof(format string, args ...any) {
	l.logger.Printf("[INFO]  "+format, args...)
}

func (l *ConsoleLogger) Warnf(format string, args ...any) {
	l.logger.Printf("[WARN]  "+format, args...)
}

func (l *ConsoleLogger) Errorf(format string, args ...any) {
	l.logger.Printf("[ERROR] "+format, args...)
}
