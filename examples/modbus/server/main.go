// Example: Modbus TCP Server
//
// This example demonstrates how to build a full-featured Modbus TCP server
// using the goindustrial library. It covers:
//
//   - Creating and configuring a Modbus TCP server
//   - Pre-populating the in-memory data store with sample data
//   - Registering client connect/disconnect lifecycle callbacks
//   - Periodically printing server status (connected clients, transaction counts)
//   - Graceful shutdown via OS signals (SIGINT / SIGTERM)
//
// The server listens on a configurable address and port, accepts Modbus TCP
// clients, and serves all standard Modbus function codes out of the box. The
// default handler set includes read/write for coils, discrete inputs, holding
// registers, and input registers.
//
// Usage:
//
//	go run ./examples/modbus/server -addr 0.0.0.0 -port 5020
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	modbus "github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// ---------------------------------------------------------------------------
	// Parse command-line flags
	// ---------------------------------------------------------------------------

	// -addr: the network interface address to bind to.
	//   "0.0.0.0" binds to all interfaces (the default).
	//   "127.0.0.1" restricts to loopback only.
	addr := flag.String("addr", "0.0.0.0", "Bind address for the Modbus TCP server")

	// -port: the TCP port to listen on.
	//   The standard Modbus TCP port is 502, but that requires root/admin privileges
	//   on most systems. We default to 5020 for convenience during development.
	port := flag.Int("port", 5020, "TCP port for the Modbus TCP server")

	flag.Parse()

	// ---------------------------------------------------------------------------
	// Set up structured logging
	// ---------------------------------------------------------------------------

	// NewDefaultLogger writes timestamped, leveled log lines to stdout.
	// We use LevelInfo so that routine request/response traffic (logged at Debug)
	// stays hidden, but lifecycle events and errors are visible.
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	ctx := context.Background()

	// ---------------------------------------------------------------------------
	// Create and populate the data store
	// ---------------------------------------------------------------------------

	// MemoryStore is a thread-safe, map-backed implementation of the DataStore
	// interface. It holds all four Modbus data areas:
	//
	//   1. Coils            (FC 01/05/0F) - boolean, read/write
	//   2. Discrete Inputs  (FC 02)       - boolean, read-only from client perspective
	//   3. Holding Registers(FC 03/06/10) - uint16, read/write
	//   4. Input Registers  (FC 04)       - uint16, read-only from client perspective
	//
	// "Read-only" here means the Modbus protocol does not define write function
	// codes for discrete inputs or input registers. The server-side application
	// can (and should) update them via the SetDiscreteInput / SetInputRegister
	// helper methods.
	store := modbus.NewMemoryStore()

	// Pre-populate coils (addresses 0-9).
	// In a real application these might represent physical relay outputs, motor
	// starters, valve actuators, or indicator lights.
	logger.Info(ctx, "Pre-populating data store with sample data")

	store.SetCoil(0, true)  // Coil 0: ON  - e.g. "system running" indicator
	store.SetCoil(1, false) // Coil 1: OFF - e.g. "alarm active" flag
	store.SetCoil(2, true)  // Coil 2: ON  - e.g. "pump 1 enabled"
	store.SetCoil(3, true)  // Coil 3: ON  - e.g. "pump 2 enabled"
	store.SetCoil(4, false) // Coil 4: OFF - e.g. "heater enabled"
	store.SetCoil(5, true)  // Coil 5: ON  - e.g. "fan enabled"
	store.SetCoil(6, false) // Coil 6: OFF - spare
	store.SetCoil(7, false) // Coil 7: OFF - spare
	store.SetCoil(8, true)  // Coil 8: ON  - e.g. "lighting zone 1"
	store.SetCoil(9, false) // Coil 9: OFF - e.g. "lighting zone 2"

	// Pre-populate discrete inputs (addresses 0-7).
	// Discrete inputs are typically wired to physical sensors, limit switches,
	// or status contacts that the server reads from the field and the Modbus
	// client queries via FC 02.
	store.SetDiscreteInput(0, true)  // DI 0: e.g. "door closed" sensor
	store.SetDiscreteInput(1, true)  // DI 1: e.g. "pressure OK" switch
	store.SetDiscreteInput(2, false) // DI 2: e.g. "high temperature" alarm
	store.SetDiscreteInput(3, true)  // DI 3: e.g. "level OK" float switch
	store.SetDiscreteInput(4, false) // DI 4: e.g. "emergency stop" (NC contact)
	store.SetDiscreteInput(5, true)  // DI 5: e.g. "motor running" feedback
	store.SetDiscreteInput(6, true)  // DI 6: e.g. "VFD ready" status
	store.SetDiscreteInput(7, false) // DI 7: e.g. "UPS on battery"

	// Pre-populate holding registers (addresses 0-9).
	// Holding registers are the primary read/write data area and are commonly used
	// for setpoints, configuration parameters, and control values.
	store.SetHoldingRegister(0, 1000) // HR 0: e.g. temperature setpoint (x10, so 100.0 deg)
	store.SetHoldingRegister(1, 500)  // HR 1: e.g. pressure setpoint (x10, so 50.0 psi)
	store.SetHoldingRegister(2, 60)   // HR 2: e.g. speed setpoint (Hz)
	store.SetHoldingRegister(3, 100)  // HR 3: e.g. flow rate setpoint (GPM)
	store.SetHoldingRegister(4, 4000) // HR 4: e.g. analog output 1 (0-4095 DAC)
	store.SetHoldingRegister(5, 2048) // HR 5: e.g. analog output 2 (0-4095 DAC)
	store.SetHoldingRegister(6, 0)    // HR 6: e.g. operating mode (0=auto)
	store.SetHoldingRegister(7, 1)    // HR 7: e.g. PID enable (1=enabled)
	store.SetHoldingRegister(8, 300)  // HR 8: e.g. PID proportional gain (x100)
	store.SetHoldingRegister(9, 50)   // HR 9: e.g. PID integral time (seconds)

	// Pre-populate input registers (addresses 0-9).
	// Input registers are read-only from the Modbus client side and typically hold
	// measured process values, counters, and status words.
	store.SetInputRegister(0, 985)   // IR 0: e.g. measured temperature (x10, so 98.5 deg)
	store.SetInputRegister(1, 487)   // IR 1: e.g. measured pressure (x10, so 48.7 psi)
	store.SetInputRegister(2, 59)    // IR 2: e.g. measured speed (Hz)
	store.SetInputRegister(3, 97)    // IR 3: e.g. measured flow rate (GPM)
	store.SetInputRegister(4, 3950)  // IR 4: e.g. analog input 1 raw (0-4095 ADC)
	store.SetInputRegister(5, 2100)  // IR 5: e.g. analog input 2 raw (0-4095 ADC)
	store.SetInputRegister(6, 24)    // IR 6: e.g. supply voltage (x10, so 2.4V or 24V)
	store.SetInputRegister(7, 1250)  // IR 7: e.g. current draw (mA)
	store.SetInputRegister(8, 12345) // IR 8: e.g. operating hours counter
	store.SetInputRegister(9, 42)    // IR 9: e.g. firmware version (4.2)

	// ---------------------------------------------------------------------------
	// Create the Modbus TCP server
	// ---------------------------------------------------------------------------

	// NewServer creates a server bound to the given address. Options customize
	// the port, logger, data store, and lifecycle callbacks.
	//
	// The server automatically registers default handlers for all standard
	// function codes: FC 01-04 (reads), FC 05/06 (single writes),
	// FC 0F/10 (multiple writes), FC 17 (read/write multiple registers),
	// and FC 2B (device identification).
	server := modbus.NewServer(*addr,
		// Set the TCP port. The Modbus specification defines 502 as the standard
		// port, but non-privileged ports (e.g. 5020) are easier for development.
		modbus.WithServerPort(*port),

		// Attach a logger so we can see server activity on stdout.
		modbus.WithServerLogger(logger),

		// Inject our pre-populated data store. Without this option the server
		// creates its own empty MemoryStore.
		modbus.WithServerDataStore(store),

		// OnClientConnect fires once for every new TCP connection.
		// The ConnectedClient snapshot contains the remote address, connection
		// time, and an initially empty FunctionCodeStats map.
		modbus.WithOnClientConnect(func(client modbus.ConnectedClient) {
			logger.Info(ctx, "CLIENT CONNECTED: %s", client.RemoteAddr)
		}),

		// OnClientDisconnect fires when a client connection is closed (either
		// by the client, by network failure, or during server shutdown).
		// At this point the snapshot includes accumulated transaction counters.
		modbus.WithOnClientDisconnect(func(client modbus.ConnectedClient) {
			logger.Info(ctx, "CLIENT DISCONNECTED: %s (rx=%d, tx=%d)",
				client.RemoteAddr, client.RxTransactions, client.TxTransactions)
		}),
	)

	// ---------------------------------------------------------------------------
	// Start the server
	// ---------------------------------------------------------------------------

	logger.Info(ctx, "Starting Modbus TCP server on %s:%d", *addr, *port)

	if err := server.Start(ctx); err != nil {
		logger.Error(ctx, "Failed to start server: %v", err)
		os.Exit(1)
	}

	logger.Info(ctx, "Server is running. Press Ctrl+C to stop.")

	// ---------------------------------------------------------------------------
	// Periodic status reporting
	// ---------------------------------------------------------------------------

	// Print a status summary every 10 seconds. This goroutine inspects the list
	// of currently connected clients, their transaction counts, and per-function-code
	// statistics. In a production system you might push these metrics to Prometheus,
	// InfluxDB, or another time-series database instead.
	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()

	go func() {
		for range statusTicker.C {
			// ConnectedClients() returns a snapshot slice; safe to iterate without
			// holding any server locks.
			clients := server.ConnectedClients()

			if len(clients) == 0 {
				logger.Info(ctx, "[STATUS] Server running | No clients connected")
				continue
			}

			logger.Info(ctx, "[STATUS] Server running | %d client(s) connected:", len(clients))

			for i, c := range clients {
				// ConnectedClient.String() formats a one-line summary including
				// remote address, connection duration, rx/tx counts, and
				// per-function-code breakdowns.
				logger.Info(ctx, "[STATUS]   Client %d: %s", i+1, c.String())
			}
		}
	}()

	// ---------------------------------------------------------------------------
	// Wait for OS signal, then shut down gracefully
	// ---------------------------------------------------------------------------

	// Catch SIGINT (Ctrl+C) and SIGTERM (docker stop, systemd stop, kill).
	// A buffered channel of size 1 ensures we don't miss the signal even if
	// we're not yet blocked on the receive.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info(ctx, "Received signal: %v. Shutting down...", sig)

	// Stop the server. This closes the listener, terminates all client
	// connections, and fires OnClientDisconnect for each one.
	if err := server.Stop(ctx); err != nil {
		logger.Error(ctx, "Error stopping server: %v", err)
		os.Exit(1)
	}

	// Print final data store state so we can see any writes that clients made.
	fmt.Println("\n--- Final Data Store State ---")
	fmt.Println(store.DumpRegisters())

	logger.Info(ctx, "Server stopped cleanly. Goodbye.")
}
