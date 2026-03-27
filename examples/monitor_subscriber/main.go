// Example: monitor_subscriber
//
// Demonstrates the Subscriber API for consuming monitor events. Subscribers
// are independent consumers that each receive a copy of every event through
// their own buffered channel. A slow subscriber never blocks the monitor or
// other subscribers — events are silently dropped when the buffer is full.
//
// Key features:
//
//   - Multiple subscribers receive the same events (broadcast fan-out)
//   - Each subscriber has its own buffered channel (configurable size)
//   - Subscribers implement iter.Seq[Event] for use in for-range loops
//   - Done() unregisters the subscriber and closes its channel
//   - Monitor.Close() closes all subscriber channels automatically
//
// Architecture:
//
//	  Monitor (polls PLC data points)
//	     │
//	     ├─── Subscriber A (buffer=128)  → for evt := range subA.All() { ... }
//	     ├─── Subscriber B (buffer=128)  → for evt := range subB.All() { ... }
//	     └─── Events() channel           → legacy channel-based consumption
//
// This example creates two concurrent subscribers that each process events
// independently: one logs all events, the other only prints changes. Both
// use the iter.Seq[Event] pattern with for-range.
//
// Usage:
//
//	go run . -modbus-addr 127.0.0.1 -modbus-port 5020
//	go run . -modbus-addr 192.168.1.10 -modbus-port 502 -interval 500ms
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/monitor"
	"github.com/iceisfun/goindustrial/protocol/modbus"
)

func main() {
	addr := flag.String("modbus-addr", "127.0.0.1", "Modbus TCP server address")
	port := flag.Int("modbus-port", modbus.DefaultTCPPort, "Modbus TCP port")
	register := flag.Int("register", 0, "Holding register address to monitor")
	interval := flag.Duration("interval", 1*time.Second, "Poll interval")
	flag.Parse()

	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Connect to Modbus TCP server
	// -------------------------------------------------------------------

	fmt.Printf("Connecting to Modbus TCP server at %s:%d...\n", *addr, *port)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := modbus.Connect(ctx, *addr,
		modbus.WithPort(*port),
		modbus.WithUnitID(1),
		modbus.WithLogger(logger),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: start a server with: go run ../modbus/server -port 5020")
		os.Exit(1)
	}
	defer client.Close()
	fmt.Println("Connected.")

	// -------------------------------------------------------------------
	// Create a Monitor and subscribe to a register
	// -------------------------------------------------------------------

	mon, err := monitor.NewMonitor(client,
		monitor.WithLogger(logger),
		monitor.WithEventBuffer(128),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create monitor: %v\n", err)
		os.Exit(1)
	}
	defer mon.Close()

	point := modbus.HoldingRegister{
		Addr: modbus.Address(*register),
		Qty:  1,
	}

	_, err = mon.Subscribe(point,
		monitor.WithFrequency(*interval),
		monitor.WithReadVariance(10*time.Millisecond),
		monitor.WithChangeDetector(monitor.ByteChangeDetector{}),
		monitor.WithInitialRead(0),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe: %v\n", err)
		os.Exit(1)
	}

	// -------------------------------------------------------------------
	// Create two subscribers
	// -------------------------------------------------------------------
	// Each subscriber gets its own buffered channel. The monitor broadcasts
	// every event to every subscriber without blocking.

	// Subscriber A: logs every event.
	subA, err := mon.NewSubscriber(128)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create subscriber A: %v\n", err)
		os.Exit(1)
	}
	defer subA.Done()

	// Subscriber B: only prints changes.
	subB, err := mon.NewSubscriber(128)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create subscriber B: %v\n", err)
		os.Exit(1)
	}
	defer subB.Done()

	fmt.Printf("Monitoring %s with 2 subscribers — poll every %s\n", point.String(), *interval)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// -------------------------------------------------------------------
	// Run subscribers concurrently using for-range over All()
	// -------------------------------------------------------------------

	var wg sync.WaitGroup
	wg.Add(2)

	// Subscriber A: log every event using the iter.Seq[Event] pattern.
	go func() {
		defer wg.Done()
		count := 0
		for evt := range subA.All() {
			count++
			ts := evt.Snapshot.Timestamp.Format("15:04:05.000")

			if evt.Err != nil {
				fmt.Printf("[A] %s  #%d  ERROR: %v\n", ts, count, evt.Err)
				continue
			}

			val := formatRegister(evt.Snapshot.Value.Raw)
			changed := "  "
			if evt.Changed {
				changed = " *"
			}
			fmt.Printf("[A] %s  #%d  %s = %s%s\n",
				ts, count, evt.Snapshot.Point, val, changed)
		}
		fmt.Println("[A] subscriber done")
	}()

	// Subscriber B: only act on changes, ignore unchanged polls.
	go func() {
		defer wg.Done()
		for evt := range subB.All() {
			if evt.Err != nil || !evt.Changed {
				continue
			}

			ts := evt.Snapshot.Timestamp.Format("15:04:05.000")
			val := formatRegister(evt.Snapshot.Value.Raw)
			fmt.Printf("[B] %s  CHANGED  %s → %s\n",
				ts, evt.Snapshot.Point, val)
		}
		fmt.Println("[B] subscriber done")
	}()

	// -------------------------------------------------------------------
	// Graceful shutdown
	// -------------------------------------------------------------------

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	fmt.Printf("\nReceived %v, shutting down...\n", sig)

	// Close the monitor, which closes all subscriber channels.
	// The for-range loops in both goroutines will terminate.
	mon.Close()
	wg.Wait()

	fmt.Println("Done.")
}

func formatRegister(raw []byte) string {
	if len(raw) >= 2 {
		val := binary.BigEndian.Uint16(raw)
		return fmt.Sprintf("%d (0x%04X)", val, val)
	}
	return fmt.Sprintf("0x%X", raw)
}
