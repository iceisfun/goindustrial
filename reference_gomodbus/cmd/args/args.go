package args

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Moonlight-Companies/gomodbus/client"
	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// ModbusArgs holds common command-line arguments for Modbus clients
type ModbusArgs struct {
	IP         string
	Port       int
	UnitID     int
	Timeout    time.Duration
	LogLevel   string
	LogLevelID common.LogLevel
}

// ParseArgs parses common command-line arguments for Modbus clients
func ParseArgs() *ModbusArgs {
	args := &ModbusArgs{}

	// Define command-line flags
	flag.StringVar(&args.IP, "ip", "127.0.0.1", "Modbus server IP address")
	flag.IntVar(&args.Port, "port", 502, "Modbus server port")
	flag.IntVar(&args.UnitID, "unit", 1, "Modbus unit ID (slave ID)")
	flag.DurationVar(&args.Timeout, "timeout", 5*time.Second, "Timeout for Modbus operations")
	flag.StringVar(&args.LogLevel, "log", "info", "Log level (debug, info, warn, error)")

	// Custom usage function
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}

	// Parse the flags
	flag.Parse()

	// Map log level string to LogLevel
	switch args.LogLevel {
	case "debug":
		args.LogLevelID = common.LevelDebug
	case "info":
		args.LogLevelID = common.LevelInfo
	case "warn":
		args.LogLevelID = common.LevelWarn
	case "error":
		args.LogLevelID = common.LevelError
	default:
		fmt.Printf("Invalid log level: %s, using 'info'\n", args.LogLevel)
		args.LogLevelID = common.LevelInfo
	}

	return args
}

// CreateClient creates a Modbus TCP client using the command-line arguments
func (args *ModbusArgs) CreateClient() *client.TCPClient {
	// Create a logger
	logger := logging.NewLogger(
		logging.WithLevel(args.LogLevelID),
	)

	// Create a TCP client
	modbusClient := client.NewTCPClient(
		args.IP,
		transport.WithPort(args.Port),
		transport.WithTimeoutOption(args.Timeout),
		transport.WithTransportLogger(logger),
	)

	// Set the logger and unit ID
	configuredClient := modbusClient.WithOptions(
		client.WithTCPLogger(logger),
		client.WithTCPUnitID(common.UnitID(args.UnitID)),
	)

	return configuredClient
}