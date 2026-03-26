package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Moonlight-Companies/gomodbus/client"
	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// CustomLogger implements the common.LoggerInterface
type CustomLogger struct {
	logger *log.Logger
	level  common.LogLevel
}

// NewCustomLogger creates a new custom logger
func NewCustomLogger(level common.LogLevel) *CustomLogger {
	return &CustomLogger{
		logger: log.New(os.Stdout, "[MODBUS] ", log.LstdFlags|log.Lmicroseconds),
		level:  level,
	}
}

// Trace logs a trace message
func (l *CustomLogger) Trace(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelTrace {
		l.logger.Printf("[TRACE] "+format, args...)
	}
}

// Debug logs a debug message
func (l *CustomLogger) Debug(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelDebug {
		l.logger.Printf("[DEBUG] "+format, args...)
	}
}

// Info logs an info message
func (l *CustomLogger) Info(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelInfo {
		l.logger.Printf("[INFO] "+format, args...)
	}
}

// Warn logs a warning message
func (l *CustomLogger) Warn(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelWarn {
		l.logger.Printf("[WARN] "+format, args...)
	}
}

// Error logs an error message
func (l *CustomLogger) Error(ctx context.Context, format string, args ...interface{}) {
	if l.level <= common.LevelError {
		l.logger.Printf("[ERROR] "+format, args...)
	}
}

// GetLevel returns the current log level
func (l *CustomLogger) GetLevel() common.LogLevel {
	return l.level
}

// SetLevel sets the logger level
func (l *CustomLogger) SetLevel(level common.LogLevel) {
	l.level = level
}

// WithFields returns a new logger with the given fields
func (l *CustomLogger) WithFields(fields map[string]interface{}) common.LoggerInterface {
	// This is a simple implementation that ignores fields
	// In a real implementation, you might append these fields to each log message
	return l
}

func main() {
	// Parse command-line arguments
	ip := "127.0.0.1"
	port := 502
	timeout := 5 * time.Second

	// Create a custom logger
	logger := NewCustomLogger(common.LevelDebug)
	fmt.Println("Using custom logger implementation...")

	// Create a Modbus client with the custom logger
	modbusClient := client.NewTCPClient(
		ip,
		transport.WithPort(port),
		transport.WithTimeoutOption(timeout),
		transport.WithTransportLogger(logger),
	)

	// Apply TCP client options
	modbusClient = modbusClient.WithOptions(
		client.WithTCPLogger(logger),
	)

	// Set the unit ID (using a TCP-specific option)
	modbusClient = modbusClient.WithOptions(
		client.WithTCPUnitID(common.UnitID(1)),
	)

	// Connect to the server
	ctx := context.Background()
	err := modbusClient.Connect(ctx)
	if err != nil {
		fmt.Println("Failed to connect to Modbus server:", err)
		return
	}
	defer modbusClient.Disconnect(ctx)

	// Read some holding registers
	registers, err := modbusClient.ReadHoldingRegisters(ctx, common.Address(0), common.Quantity(10))
	if err != nil {
		fmt.Println("Failed to read holding registers:", err)
		return
	}

	// Display the results
	fmt.Println("Read holding registers:")
	for i, value := range registers {
		fmt.Printf("Register %d: %d (0x%04X)\n", i, value, value)
	}
}