// Example: monitor
//
// Demonstrates cross-protocol monitoring using the goindustrial monitor
// package. A single Monitor instance polls data points from both a Modbus
// TCP server and an EtherNet/IP server simultaneously, demonstrating
// that the monitor works with any implementation of the plc.Reader interface.
//
// The monitor package provides a unified way to:
//   - Subscribe to data points with configurable poll frequencies
//   - Detect value changes using pluggable change detectors
//   - Receive events through per-subscription handlers AND a unified channel
//   - Monitor multiple protocols through a single event stream
//
// Architecture:
//
//	                  monitor.Monitor
//	                 /               \
//	   modbus.Client                  ethernetip.Client
//	   (plc.Reader)                   (plc.Reader)
//	        |                              |
//	   Modbus TCP Server            EtherNet/IP Server
//
// Because both modbus.Client and ethernetip.Client implement plc.Reader,
// the monitor does not need to know anything about the underlying protocol.
// You can add subscriptions for Modbus holding registers and EtherNet/IP
// tags to the same Monitor instance, and they will be polled independently
// at their own frequencies.
//
// This example shows:
//   - Creating clients for both protocols
//   - Creating a single Monitor instance
//   - Subscribing with per-subscription handlers (callback-style)
//   - Subscribing with change detection (ByteChangeDetector)
//   - Consuming the unified event channel
//   - Graceful shutdown with OS signal handling
//
// Usage:
//
//	go run . \
//	  -modbus-addr 127.0.0.1 -modbus-port 5020 -modbus-register 0 \
//	  -eip-addr 127.0.0.1:44818 -eip-tag MyDINT \
//	  -interval 1s
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/monitor"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	// -------------------------------------------------------------------
	// Parse command-line flags
	// -------------------------------------------------------------------

	// Modbus connection flags
	modbusAddr := flag.String("modbus-addr", "127.0.0.1", "Modbus TCP server address")
	modbusPort := flag.Int("modbus-port", modbus.DefaultTCPPort, "Modbus TCP port")
	modbusRegister := flag.Int("modbus-register", 0, "Modbus holding register address to monitor")

	// EtherNet/IP connection flags
	eipAddr := flag.String("eip-addr", "127.0.0.1:44818", "EtherNet/IP server address (host:port)")
	eipTag := flag.String("eip-tag", "MyDINT", "EtherNet/IP tag name to monitor")

	// Common monitoring flags
	interval := flag.Duration("interval", 1*time.Second, "Poll interval for all subscriptions")

	flag.Parse()

	// -------------------------------------------------------------------
	// Set up logging
	// -------------------------------------------------------------------
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Create protocol clients
	// -------------------------------------------------------------------
	// Both modbus.Client and ethernetip.Client implement the plc.Reader
	// interface, which is all the monitor needs to poll data points.
	//
	// We use the convenience Connect() constructors here, which establish
	// the initial connection. In a production system, you might use
	// reconnecting transports for fault tolerance.

	// --- Modbus Client ---
	// modbus.Connect creates a client with a ReconnectingTransport and
	// verifies the initial connection.
	fmt.Printf("Connecting to Modbus TCP server at %s:%d...\n", *modbusAddr, *modbusPort)

	modbusCtx, modbusCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer modbusCancel()

	modbusClient, err := modbus.Connect(modbusCtx, *modbusAddr,
		modbus.WithPort(*modbusPort),         // TCPConnOption: set the TCP port
		modbus.WithUnitID(1),                 // ClientOption: Modbus unit ID
		modbus.WithLogger(logger),            // ClientOption: attach logger
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Modbus server: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: Make sure a Modbus TCP server is running.")
		fmt.Fprintln(os.Stderr, "You can use the modbus/server example: go run ../../modbus/server")
		os.Exit(1)
	}
	defer modbusClient.Close()
	fmt.Println("Modbus client connected.")

	// --- EtherNet/IP Client ---
	// ethernetip.Connect creates a direct (non-reconnecting) client and
	// connects immediately.
	fmt.Printf("Connecting to EtherNet/IP server at %s...\n", *eipAddr)

	eipCtx, eipCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer eipCancel()

	eipClient, err := ethernetip.Connect(eipCtx, *eipAddr,
		ethernetip.WithLogger(logger),  // ClientOption: attach logger
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to EtherNet/IP server: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: Make sure an EtherNet/IP server is running.")
		fmt.Fprintln(os.Stderr, "You can use the ethernetip/server example: go run ../../ethernetip/server")
		os.Exit(1)
	}
	defer eipClient.Close()
	fmt.Println("EtherNet/IP client connected.")
	fmt.Println()

	// -------------------------------------------------------------------
	// Create the Monitor
	// -------------------------------------------------------------------
	// The monitor is the central component that manages all subscriptions.
	// It takes a plc.Reader, which means it can only work with one protocol
	// at a time per monitor instance.
	//
	// IMPORTANT: Since the monitor takes a single plc.Reader, and we want
	// to monitor two different protocols, we need to create two separate
	// Monitor instances -- one for each protocol. Each monitor manages its
	// own subscriptions and emits events independently.
	//
	// If you only need one protocol, a single monitor is sufficient.
	//
	// monitor.WithEventBuffer sets the size of the internal event channel.
	// If events are not consumed fast enough, the oldest events are dropped.
	// A larger buffer helps absorb bursts of changes.

	// Create a monitor for Modbus data points.
	modbusMon, err := monitor.NewMonitor(modbusClient,
		monitor.WithLogger(logger),        // Attach logger for diagnostics
		monitor.WithEventBuffer(128),      // Buffer up to 128 events
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Modbus monitor: %v\n", err)
		os.Exit(1)
	}
	defer modbusMon.Close()

	// Create a monitor for EtherNet/IP data points.
	eipMon, err := monitor.NewMonitor(eipClient,
		monitor.WithLogger(logger),        // Attach logger for diagnostics
		monitor.WithEventBuffer(128),      // Buffer up to 128 events
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create EtherNet/IP monitor: %v\n", err)
		os.Exit(1)
	}
	defer eipMon.Close()

	// -------------------------------------------------------------------
	// Subscribe to Modbus holding registers
	// -------------------------------------------------------------------
	// We subscribe to a holding register with:
	//   - A specific poll frequency
	//   - A ByteChangeDetector to detect when the value changes
	//   - A per-subscription handler that prints changes immediately
	//   - An initial read to get the current value right away
	//
	// The data point is a modbus.HoldingRegister, which specifies the
	// register address and quantity (number of 16-bit registers to read).

	modbusPoint := modbus.HoldingRegister{
		Addr: modbus.Address(*modbusRegister), // Starting register address
		Qty:  1,                                // Read 1 register (2 bytes)
	}

	fmt.Printf("Subscribing to Modbus %s...\n", modbusPoint.String())

	// The handler is called after every successful poll. It receives a
	// Snapshot containing the data point, the raw value, and a timestamp.
	// This is the "callback style" of consuming events -- useful when you
	// want per-subscription logic without checking subscription IDs.
	modbusSub, err := modbusMon.Subscribe(modbusPoint,
		// WithFrequency sets how often this specific subscription is polled.
		// Each subscription can have its own frequency -- fast-changing values
		// can be polled more often than slow-changing ones.
		monitor.WithFrequency(*interval),

		// WithReadVariance adds random jitter to the poll timing. This
		// prevents all subscriptions from hitting the server at exactly the
		// same instant (thundering herd). A variance of 10ms means the
		// actual poll delay will be frequency +/- up to 10ms.
		monitor.WithReadVariance(10*time.Millisecond),

		// WithChangeDetector attaches a change detector. The monitor compares
		// each poll result with the previous one using this detector. The
		// Event.Changed field reflects whether the detector saw a change.
		//
		// ByteChangeDetector is the simplest detector: it compares the raw
		// byte slices using bytes.Equal(). For floating-point values, you
		// might want a deadband detector that ignores small fluctuations.
		monitor.WithChangeDetector(monitor.ByteChangeDetector{}),

		// WithHandler registers a callback that fires after every successful
		// poll. The handler receives the Snapshot (data point + value + time).
		// The handler runs synchronously in the poll goroutine, so keep it
		// fast to avoid delaying subsequent polls.
		monitor.WithHandler(func(snap monitor.Snapshot) {
			// Display the raw register value as a 16-bit unsigned integer.
			if len(snap.Value.Raw) >= 2 {
				val := binary.BigEndian.Uint16(snap.Value.Raw)
				fmt.Printf("[%s] HANDLER: Modbus %s = %d (0x%04X)\n",
					snap.Timestamp.Format("15:04:05"),
					snap.Point.String(), val, val)
			}
		}),

		// WithInitialRead triggers an immediate read when the subscription
		// is created, instead of waiting for the first tick.
		monitor.WithInitialRead(true),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to Modbus register: %v\n", err)
		os.Exit(1)
	}
	_ = modbusSub // We keep the reference so we could call modbusSub.Stop() later.

	// -------------------------------------------------------------------
	// Subscribe to an EtherNet/IP tag
	// -------------------------------------------------------------------
	// Same concept as the Modbus subscription, but with an EtherNet/IP
	// data point (ethernetip.Tag). The tag specifies a name and element
	// count. The monitor doesn't care about the protocol -- it just calls
	// the plc.Reader.Read() method, which the ethernetip.Client implements.

	eipPoint := ethernetip.Tag{
		Name:     *eipTag, // Tag name on the PLC (e.g., "MyDINT")
		Elements: 1,       // Read 1 element
	}

	fmt.Printf("Subscribing to EtherNet/IP tag %q...\n", eipPoint.Name)

	// This subscription uses the same options as the Modbus one, showing
	// that the API is identical regardless of the underlying protocol.
	eipSub, err := eipMon.Subscribe(eipPoint,
		monitor.WithFrequency(*interval),
		monitor.WithReadVariance(10*time.Millisecond),
		monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
		monitor.WithHandler(func(snap monitor.Snapshot) {
			// EtherNet/IP ReadTag responses include a 2-byte type prefix
			// followed by the value data. We display the raw bytes here.
			fmt.Printf("[%s] HANDLER: EtherNet/IP %s = 0x%X (%d bytes)\n",
				snap.Timestamp.Format("15:04:05"),
				snap.Point.String(), snap.Value.Raw, len(snap.Value.Raw))
		}),
		monitor.WithInitialRead(true),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe to EtherNet/IP tag: %v\n", err)
		os.Exit(1)
	}
	_ = eipSub

	fmt.Println()
	fmt.Println("Monitor is running. Press Ctrl+C to stop.")
	fmt.Println("Events from both protocols will appear below:")
	fmt.Println()

	// -------------------------------------------------------------------
	// Set up graceful shutdown
	// -------------------------------------------------------------------
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal: %v. Shutting down...\n", sig)
		cancel()
	}()

	// -------------------------------------------------------------------
	// Consume the unified event channels
	// -------------------------------------------------------------------
	// In addition to (or instead of) per-subscription handlers, you can
	// consume events from the monitor's Events() channel. This is useful
	// when you want to process all events in a single place, for example
	// to log them to a database, forward them to a message broker, or
	// display them on a dashboard.
	//
	// Each monitor has its own Events() channel. We use a select to
	// multiplex events from both monitors into a single processing loop.
	//
	// The Event struct contains:
	//   - SubscriptionID: identifies which subscription produced this event
	//   - Snapshot: the data point, raw value, and timestamp
	//   - Err: non-nil if the read failed
	//   - Changed: true if the value changed since the last poll (only
	//     meaningful when a ChangeDetector is attached)
	for {
		select {
		case <-ctx.Done():
			// Shutdown requested. Close both monitors (which stops all
			// subscriptions and closes the event channels).
			fmt.Println("\nShutting down monitors...")
			modbusMon.Close()
			eipMon.Close()
			fmt.Println("Monitors stopped.")
			fmt.Println("Done.")
			return

		case event, ok := <-modbusMon.Events():
			if !ok {
				// Channel closed -- monitor was shut down.
				continue
			}
			printEvent("Modbus", event)

		case event, ok := <-eipMon.Events():
			if !ok {
				// Channel closed -- monitor was shut down.
				continue
			}
			printEvent("EtherNet/IP", event)
		}
	}
}

// printEvent formats and prints a monitor event to stdout.
func printEvent(protocol string, event monitor.Event) {
	timestamp := event.Snapshot.Timestamp.Format("15:04:05")

	// Check if the read failed.
	if event.Err != nil {
		fmt.Printf("[%s] EVENT [%s] sub=%d ERROR: %v\n",
			timestamp, protocol, event.SubscriptionID, event.Err)
		return
	}

	// Build a change indicator.
	changeStr := "unchanged"
	if event.Changed {
		changeStr = "CHANGED"
	}

	// Print the event with protocol, subscription ID, change status, and value.
	fmt.Printf("[%s] EVENT [%s] sub=%d %s point=%s value=0x%X (%d bytes)\n",
		timestamp, protocol, event.SubscriptionID, changeStr,
		event.Snapshot.Point.String(),
		event.Snapshot.Value.Raw,
		len(event.Snapshot.Value.Raw))
}
