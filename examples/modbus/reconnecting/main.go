// Example: Reconnecting Modbus TCP Client
//
// This example demonstrates how to build a resilient Modbus TCP client that
// automatically reconnects after network failures. It shows:
//
//   - Manually constructing a ReconnectingTransport (not using the Connect
//     convenience function) so that you have full control over transport
//     options and lifecycle hooks
//   - OnConnect and OnDisconnect callbacks for visibility into connection state
//   - Configuring client-level retries and retry delay
//   - A continuous polling loop that survives server restarts, network blips,
//     and transient errors
//   - Timeout handling with context.WithTimeout on each poll cycle
//   - Classifying errors: ModbusError (protocol-level, never retried) vs
//     transport errors (network-level, retried automatically)
//
// Usage:
//
//	go run ./examples/modbus/reconnecting -addr 127.0.0.1 -port 5020 -interval 2s
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
	"github.com/iceisfun/goindustrial/transport"
)

func main() {
	// ---------------------------------------------------------------------------
	// Parse command-line flags
	// ---------------------------------------------------------------------------

	// -addr: hostname or IP of the Modbus TCP server to connect to.
	addr := flag.String("addr", "127.0.0.1", "Modbus TCP server address")

	// -port: TCP port of the target server.
	port := flag.Int("port", 5020, "Modbus TCP server port")

	// -unit: Modbus unit ID (also known as slave address). In Modbus TCP this is
	// typically 0 or 1, but gateways that bridge to serial networks use it to
	// address downstream RTU devices.
	unit := flag.Int("unit", 0, "Modbus unit ID (slave address)")

	// -address: the starting Modbus register address to poll.
	address := flag.Int("address", 0, "Starting holding register address to read")

	// -interval: how often the polling loop reads from the server.
	interval := flag.Duration("interval", 2*time.Second, "Polling interval (e.g. 1s, 500ms, 2s)")

	flag.Parse()

	// ---------------------------------------------------------------------------
	// Set up structured logging
	// ---------------------------------------------------------------------------

	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))
	ctx := context.Background()

	// ---------------------------------------------------------------------------
	// Build the ReconnectingTransport manually
	// ---------------------------------------------------------------------------
	//
	// The modbus.Connect() convenience function creates the transport internally.
	// Here we build it by hand to show each moving part:
	//
	//   1. Connector  - knows how to dial and handshake a *TCPConn
	//   2. Closer     - knows how to tear one down
	//   3. Transport   - wraps (1) and (2), provides lazy connect + auto-reconnect
	//
	// This is the approach you would use if you need:
	//   - Custom transport lifecycle hooks (OnConnect / OnDisconnect)
	//   - Multiple clients sharing one transport
	//   - Integration with a connection pool or circuit breaker

	logger.Info(ctx, "Building reconnecting transport to %s:%d", *addr, *port)

	// TCPConnector implements transport.Connector[*modbus.TCPConn]. Each call to
	// Connect() creates a fresh TCPConn, dials the server, and starts the
	// internal read/write goroutines.
	connector := modbus.NewTCPConnector(*addr,
		// Set the port on the connection itself.
		modbus.WithPort(*port),

		// Attach a logger to the low-level connection so we can see MBAP-level
		// traffic when running at Debug level.
		modbus.WithConnLogger(logger),
	)

	// TCPCloser implements transport.Closer[*modbus.TCPConn]. It calls
	// Disconnect() on the connection, which closes the socket and stops
	// the read/write goroutines.
	closer := modbus.NewTCPCloser()

	// NewReconnectingTransport never dials immediately. The first call to
	// Conn() triggers the initial connection. If the connection fails or is
	// Reset(), the next Conn() call will reconnect transparently.
	//
	// Transport lifecycle hooks:
	//
	//   OnConnect:    called after a successful (re)connection. Useful for
	//                 logging, metrics, or re-subscribing to data.
	//
	//   OnDisconnect: called when a connection is torn down (either by Reset
	//                 or Close). The error parameter is the close error (may be
	//                 nil for clean shutdowns).
	tp := transport.NewReconnectingTransport[*modbus.TCPConn](connector, closer,
		transport.WithOnConnect(func() {
			logger.Info(ctx, "TRANSPORT: Connection established to %s:%d", *addr, *port)
		}),
		transport.WithOnDisconnect(func(err error) {
			if err != nil {
				logger.Warn(ctx, "TRANSPORT: Disconnected from %s:%d (error: %v)", *addr, *port, err)
			} else {
				logger.Info(ctx, "TRANSPORT: Disconnected from %s:%d (clean)", *addr, *port)
			}
		}),
	)

	// ---------------------------------------------------------------------------
	// Create the Modbus client from the transport
	// ---------------------------------------------------------------------------
	//
	// NewClient wraps a Transport and adds Modbus-specific behaviour: framing,
	// transaction matching, and retry logic.
	//
	// Retry semantics:
	//   - WithRetries(3) means up to 4 total attempts (1 initial + 3 retries).
	//   - WithRetryDelay(2s) is the pause between consecutive attempts.
	//   - Only transport errors (network failures, timeouts) trigger retries.
	//   - ModbusError (exception responses from the server) are never retried,
	//     because the server has explicitly rejected the request and retrying
	//     will produce the same result.
	client := modbus.NewClient(tp,
		modbus.WithUnitID(modbus.UnitID(*unit)),
		modbus.WithLogger(logger),
		modbus.WithRetries(3),
		modbus.WithRetryDelay(2*time.Second),
	)

	// Ensure the transport is closed when we exit, regardless of how we exit.
	defer func() {
		logger.Info(ctx, "Closing client and transport")
		if err := client.Close(); err != nil {
			logger.Error(ctx, "Error closing client: %v", err)
		}
	}()

	// ---------------------------------------------------------------------------
	// Set up signal handling for graceful shutdown
	// ---------------------------------------------------------------------------

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// ---------------------------------------------------------------------------
	// Continuous polling loop
	// ---------------------------------------------------------------------------
	//
	// This loop reads a block of holding registers on every tick. It is designed
	// to survive server restarts:
	//
	//   1. If the server is unreachable, Conn() inside send() will fail.
	//      The client retries up to 3 times (configured above).
	//   2. If all retries are exhausted, the error is logged and the loop waits
	//      for the next tick to try again.
	//   3. When the server comes back, the transport reconnects transparently
	//      on the next Conn() call.
	//
	// This pattern is common in SCADA, HMI, and data-acquisition systems where
	// the client must keep running even if the remote device is temporarily
	// offline.

	logger.Info(ctx, "Starting polling loop: reading 5 holding registers from address %d every %s", *address, *interval)
	logger.Info(ctx, "Press Ctrl+C to stop.")

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	pollCount := 0

	for {
		select {
		case sig := <-sigCh:
			// Graceful shutdown on SIGINT or SIGTERM.
			logger.Info(ctx, "Received signal: %v. Stopping poll loop.", sig)
			return

		case <-ticker.C:
			pollCount++

			// Create a timeout context for this single poll cycle. This prevents
			// a single hung request from blocking the entire loop forever. The
			// timeout should be shorter than the poll interval so that we don't
			// stack up overlapping requests.
			pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

			logger.Info(ctx, "[Poll #%d] Reading 5 holding registers from address %d...", pollCount, *address)

			// ReadHoldingRegisters sends FC 03. The client's internal send()
			// method handles retries for transport errors.
			registers, err := client.ReadHoldingRegisters(pollCtx, modbus.Address(*address), 5)
			cancel() // Always cancel promptly to release resources.

			if err != nil {
				// ---------------------------------------------------------------------------
				// Error classification
				// ---------------------------------------------------------------------------
				//
				// There are two fundamentally different kinds of errors:
				//
				// 1. ModbusError (protocol-level exception):
				//    The server received our request, understood it, but explicitly
				//    rejected it. Common causes:
				//      - ExceptionDataAddressNotAvailable (0x02): the requested
				//        address range is not valid on this server
				//      - ExceptionFunctionCodeNotSupported (0x01): the server does
				//        not implement the requested function code
				//    These are NOT retried by the client because the server will
				//    return the same exception on every attempt.
				//
				// 2. Transport error (network-level failure):
				//    The request could not be delivered or the response could not
				//    be received. Causes include:
				//      - Server is down / unreachable
				//      - Network partition or timeout
				//      - Connection reset by peer
				//    These ARE retried (up to WithRetries count).

				if modbus.IsModbusError(err) {
					// Protocol-level error from the server. Log it prominently.
					// In a production system you might raise an alarm, disable
					// polling for this address range, or fall back to a different
					// register map.
					logger.Error(ctx, "[Poll #%d] Modbus protocol error (will NOT retry): %v", pollCount, err)

					// Optionally check for specific exception codes:
					if modbus.IsExceptionError(err, modbus.ExceptionDataAddressNotAvailable) {
						logger.Error(ctx, "[Poll #%d] The requested address range is not available on the server.", pollCount)
						logger.Error(ctx, "[Poll #%d] Check that the server has data at address %d.", pollCount, *address)
					}
				} else {
					// Transport-level error. The client already retried internally
					// (3 times). Log it and wait for the next tick. The transport
					// will attempt to reconnect on the next Conn() call.
					logger.Warn(ctx, "[Poll #%d] Transport error after retries: %v", pollCount, err)
					logger.Warn(ctx, "[Poll #%d] Will retry on next poll cycle.", pollCount)
				}

				continue
			}

			// ---------------------------------------------------------------------------
			// Successful read - display the results
			// ---------------------------------------------------------------------------

			fmt.Printf("[Poll #%d] Successfully read 5 holding registers:\n", pollCount)
			for i, val := range registers {
				regAddr := *address + i
				fmt.Printf("  Register %d = %d (0x%04X)\n", regAddr, val, val)
			}
			fmt.Println()
		}
	}
}
