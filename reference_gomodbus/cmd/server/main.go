package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/server"
)

func main() {
	// Parse command line flags
	address := flag.String("address", "0.0.0.0", "Server address to bind to")
	port := flag.Int("port", common.DefaultTCPPort, "TCP port to listen on")
	debug := flag.Bool("debug", false, "Enable debug logging")
	preloadData := flag.Bool("preload", true, "Preload some example data in the memory store")
	flag.Parse()

	// Create a logger
	logLevel := common.LevelInfo
	if *debug {
		logLevel = common.LevelDebug
	}
	logger := logging.NewLogger(logging.WithLevel(logLevel))

	// Create context for clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create memory data store
	store := server.NewMemoryStore()
	
	// Preload some sample data
	if *preloadData {
		preloadSampleData(store, logger)
	}

	// Create TCP server
	modbusServer := server.NewTCPServer(
		*address,
		server.WithServerPort(*port),
		server.WithServerLogger(logger),
		server.WithServerDataStore(store),
	)

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info(ctx, "Received shutdown signal, stopping server...")
		if err := modbusServer.Stop(ctx); err != nil {
			logger.Error(ctx, "Error stopping server: %v", err)
		}
		cancel()
	}()

	// Start the server
	logger.Info(ctx, "Starting Modbus TCP server on %s:%d...", *address, *port)
	if err := modbusServer.Start(ctx); err != nil {
		logger.Error(ctx, "Failed to start server: %v", err)
		os.Exit(1)
	}

	// Start a goroutine to periodically dump the data store's content
	if *debug {
		go func() {
			tick := time.NewTicker(10 * time.Second)
			defer tick.Stop()
			
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					logger.Debug(ctx, "Current data store contents:\n%s", store.DumpRegisters())
				}
			}
		}()
	}

	// Start a goroutine to periodically update some registers to demonstrate changing values
	go func() {
		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()

		counter := common.RegisterValue(0)
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				// Update some registers
				counter++
				store.SetInputRegister(common.Address(1000), common.InputRegisterValue(counter))
				store.SetInputRegister(common.Address(1001), common.InputRegisterValue(time.Now().Unix()&0xFFFF))
				store.SetHoldingRegister(common.Address(2000), common.RegisterValue(counter))
				store.SetCoil(common.Address(3000), common.CoilValue(counter%2 == 0)) // Toggle every second
			}
		}
	}()

	// Block until context is canceled
	<-ctx.Done()
	logger.Info(ctx, "Server shutdown complete")
}

// preloadSampleData initializes the data store with sample values
func preloadSampleData(store *server.MemoryStore, logger common.LoggerInterface) {
	ctx := context.Background()
	logger.Info(ctx, "Preloading sample data...")

	// Add some coils (digital outputs)
	coilValues := []common.CoilValue{true, false, true, true, false}
	for i, value := range coilValues {
		store.SetCoil(common.Address(i), value)
	}

	// Add some discrete inputs (digital inputs)
	diValues := []common.DiscreteInputValue{false, true, false, true, true}
	for i, value := range diValues {
		store.SetDiscreteInput(common.Address(i), value)
	}

	// Add some holding registers (analog outputs)
	hrValues := []common.RegisterValue{1000, 2000, 3000, 4000, 5000}
	for i, value := range hrValues {
		store.SetHoldingRegister(common.Address(i), value)
	}

	// Add some input registers (analog inputs)
	irValues := []common.InputRegisterValue{100, 200, 300, 400, 500}
	for i, value := range irValues {
		store.SetInputRegister(common.Address(i), value)
	}

	// Add some special registers
	store.SetInputRegister(common.Address(1000), common.InputRegisterValue(0))           // Counter register (will be updated)
	store.SetInputRegister(common.Address(1001), common.InputRegisterValue(0))           // Timestamp register (will be updated)
	store.SetHoldingRegister(common.Address(2000), common.RegisterValue(0))         // Counter register (will be updated)
	store.SetHoldingRegister(common.Address(5000), common.RegisterValue(12345))     // Fixed value
	store.SetCoil(common.Address(3000), common.CoilValue(false))                // Boolean toggle (will be updated)
	
	logger.Info(ctx, "Sample data preloaded")

	// Log the initial state
	logger.Debug(ctx, "Initial data store contents:\n%s", store.DumpRegisters())
}

// handleCustomCommand implements a custom Modbus function
func handleCustomCommand(ctx context.Context, req common.Request, store *server.MemoryStore) (common.Response, error) {
	// This is an example of how you could implement a custom Modbus function
	// For example, this could be a vendor-specific function for configuration or diagnostics
	
	// In a real implementation, you would parse the request data and perform
	// whatever action is needed, then format the response data accordingly
	
	// For this example, we'll just return a fixed response
	fmt.Println("Received custom command:", req.GetPDU().Data)
	
	// Return an error to indicate "not implemented"
	return nil, common.NewModbusError(
		req.GetPDU().FunctionCode, 
		common.ExceptionFunctionCodeNotSupported,
	)
}