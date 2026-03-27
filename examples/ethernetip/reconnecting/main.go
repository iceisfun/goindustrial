// Example: ethernetip/reconnecting
//
// Demonstrates building a reconnecting EtherNet/IP client using the manual
// transport construction approach. Instead of using the convenience function
// ethernetip.NewReconnectingClient(), this example assembles the transport
// layer piece by piece to show the full architecture and to attach lifecycle
// hooks (OnConnect/OnDisconnect callbacks).
//
// The client enters a continuous polling loop that reads a tag at a
// configurable interval. The loop is designed to survive server restarts:
// when the TCP connection drops, the reconnecting transport automatically
// re-establishes the EIP session, and polling resumes without any manual
// intervention.
//
// This example also demonstrates the critical difference between CIP errors
// and transport errors:
//
//   - CIP errors are protocol-level responses from the PLC (e.g., "tag not
//     found", "service not supported"). These are NOT retried because the
//     PLC deliberately returned an error -- retrying the same request will
//     produce the same result.
//
//   - Transport errors indicate that the TCP connection was lost or the
//     request could not be delivered. These ARE retried because the problem
//     is transient -- once the connection is re-established, the request
//     may succeed.
//
// Usage:
//
//	go run . -addr 192.168.1.100:44818 -tag MyDINT -interval 2s
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/transport"
)

func main() {
	// -------------------------------------------------------------------
	// Parse command-line flags
	// -------------------------------------------------------------------

	// -addr: the EtherNet/IP server address. This must include the port.
	// The standard EtherNet/IP port is 44818.
	addr := flag.String("addr", "127.0.0.1:44818", "EtherNet/IP server address (host:port)")

	// -tag: the CIP tag name to read. This must exist on the target PLC
	// (or on the server example running alongside this client).
	tag := flag.String("tag", "MyDINT", "Tag name to read")

	// -interval: how often to poll the tag. A shorter interval means more
	// network traffic but faster detection of value changes. In production,
	// you would tune this based on the process dynamics.
	interval := flag.Duration("interval", 2*time.Second, "Polling interval (e.g., 500ms, 2s, 10s)")

	flag.Parse()

	// -------------------------------------------------------------------
	// Set up logging
	// -------------------------------------------------------------------
	// Using Info level so we can see connection/disconnection events and
	// retry attempts without being overwhelmed by protocol-level details.
	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Build the transport layer manually
	// -------------------------------------------------------------------
	// The transport layer in goindustrial is a generic abstraction that
	// manages the lifecycle of a protocol-specific connection. For
	// EtherNet/IP, the "connection" is a *ethernetip.Session (which wraps
	// a TCP connection + EIP session registration).
	//
	// Building the transport manually (instead of using NewReconnectingClient)
	// gives you access to lifecycle hooks and full control over the
	// transport configuration.

	// Step 1: Create a SessionConnector.
	// This knows how to dial TCP and register an EIP session. Every time
	// the transport needs a new connection, it calls connector.Connect().
	connector := ethernetip.NewSessionConnector(*addr, logger)

	// Step 2: Create a SessionCloser.
	// This knows how to unregister and close an EIP session. The transport
	// calls this when it needs to tear down a stale connection.
	closer := ethernetip.SessionCloser{}

	// Step 3: Create the ReconnectingTransport with lifecycle hooks.
	// The transport wraps the connector and closer, adding automatic
	// reconnection behavior. The hooks let us observe connection events.
	//
	// transport.WithOnConnect fires every time a new session is established.
	// transport.WithOnDisconnect fires every time a session is torn down.
	// These are useful for updating dashboards, metrics, or triggering
	// application-level initialization after a reconnection.
	tp := transport.NewReconnectingTransport[*ethernetip.Session](
		connector,
		closer,

		// OnConnect is called whenever a new EIP session is successfully
		// established. This includes the initial connection and every
		// subsequent reconnection after a failure.
		transport.WithOnConnect(func() {
			logger.Info(context.Background(), "HOOK: EIP session established with %s", *addr)
			// In a real application, you might:
			//   - Re-subscribe to implicit I/O connections
			//   - Reset watchdog timers
			//   - Update a "connection status" indicator on an HMI
			//   - Log the event to an audit trail
		}),

		// OnDisconnect is called whenever an EIP session is torn down,
		// either due to an error or an explicit close. The error parameter
		// contains the close result (which may be nil if the close was clean).
		transport.WithOnDisconnect(func(err error) {
			if err != nil {
				logger.Warn(context.Background(), "HOOK: EIP session lost: %v", err)
			} else {
				logger.Info(context.Background(), "HOOK: EIP session closed cleanly")
			}
			// In a real application, you might:
			//   - Set outputs to a safe state
			//   - Trigger an alarm on your SCADA system
			//   - Start a watchdog timer
		}),
	)

	// Step 4: Create the Client, wrapping the transport.
	// The client provides the high-level tag read/write API and implements
	// the retry logic for individual operations.
	//
	// WithRetries(-1) means infinite retries. Combined with the reconnecting
	// transport, this means the client will keep trying forever -- it will
	// reconnect the TCP session if needed, then retry the CIP request.
	//
	// WithRetryDelay controls the pause between retry attempts. This
	// prevents hammering a server that may be rebooting.
	//
	// IMPORTANT: Only transport errors are retried. CIP errors (like "tag
	// not found") are returned immediately to the caller because they
	// indicate a logic error, not a connectivity issue.
	client := ethernetip.NewClient(tp,
		ethernetip.WithRetries(-1),               // Infinite retries for transport errors
		ethernetip.WithRetryDelay(2*time.Second),  // Wait 2s between retries
		ethernetip.WithLogger(logger),             // Attach our logger
	)

	// -------------------------------------------------------------------
	// Set up graceful shutdown
	// -------------------------------------------------------------------
	// We create a cancellable context that drives the polling loop. When
	// a signal arrives, we cancel the context, which causes the polling
	// loop to exit cleanly.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run signal handling in a goroutine so we can start polling immediately.
	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal: %v. Shutting down...\n", sig)
		cancel()
	}()

	// -------------------------------------------------------------------
	// Continuous polling loop
	// -------------------------------------------------------------------
	// This loop reads the tag at the specified interval. Because the client
	// is configured with infinite retries and a reconnecting transport:
	//
	//   - If the server is down, the read call blocks (retrying internally)
	//     until the server comes back up.
	//   - If the server returns a CIP error (e.g., tag not found), the
	//     error is returned immediately -- no retries.
	//   - If the context is cancelled (Ctrl+C), the read call returns with
	//     a context error and the loop exits.
	//
	// This pattern is typical in SCADA/HMI applications where you need
	// continuous data acquisition that survives network interruptions.
	fmt.Printf("Starting polling loop: reading tag %q every %v\n", *tag, *interval)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	// Do an initial read immediately (don't wait for the first tick).
	readAndPrint(ctx, client, *tag)

	for {
		select {
		case <-ctx.Done():
			// Context was cancelled by the signal handler.
			fmt.Println("\nPolling loop stopped.")
			goto cleanup

		case <-ticker.C:
			// Time to read the tag again.
			readAndPrint(ctx, client, *tag)
		}
	}

cleanup:
	// -------------------------------------------------------------------
	// Clean up
	// -------------------------------------------------------------------
	// Close the client, which closes the underlying transport and the
	// EIP session. This triggers the OnDisconnect hook one final time.
	fmt.Println("Closing client...")
	if err := client.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Error closing client: %v\n", err)
	}
	fmt.Println("Done.")
}

// readAndPrint reads a tag and prints the result. It demonstrates how to
// distinguish between CIP errors and transport errors.
func readAndPrint(ctx context.Context, client *ethernetip.Client, tagName string) {
	// ReadTag returns the raw response bytes including the 2-byte type code
	// prefix. On transport errors (with infinite retries), this call blocks
	// until it succeeds or the context is cancelled.
	data, err := client.ReadTag(ctx, tagName)
	if err != nil {
		// Check if this is a CIP error (protocol-level, from the PLC).
		// CIP errors are deterministic -- the PLC received our request but
		// could not fulfill it. Retrying the same request will produce the
		// same error, so the client's retry loop does NOT retry these.
		var cipErr cip.Error
		if errors.As(err, &cipErr) {
			fmt.Printf("[%s] CIP ERROR reading %q: status=0x%02X (%v)\n",
				time.Now().Format("15:04:05"), tagName, cipErr.Status, cipErr)

			// Provide helpful context for common CIP error codes.
			switch cipErr.Status {
			case cip.StatusPathDestinationUnknown:
				fmt.Println("  -> Tag does not exist on the PLC. Check the tag name.")
			case cip.StatusServiceNotSupported:
				fmt.Println("  -> The target object does not support the ReadTag service.")
			case cip.StatusObjectDoesNotExist:
				fmt.Println("  -> The target CIP object class does not exist.")
			}
			return
		}

		// If it's not a CIP error, it's either a transport error (which
		// should have been retried internally) or a context cancellation.
		if ctx.Err() != nil {
			// Context cancelled -- this is expected during shutdown.
			return
		}

		// Unexpected transport error that exhausted all retries (should
		// not happen with infinite retries, but handle it gracefully).
		fmt.Printf("[%s] TRANSPORT ERROR reading %q: %v\n",
			time.Now().Format("15:04:05"), tagName, err)
		return
	}

	// -------------------------------------------------------------------
	// Success! Parse and display the tag value.
	// -------------------------------------------------------------------
	// The ReadTag response format is:
	//   [DataType (2 bytes, little-endian)] [Value data (N bytes)]
	if len(data) < 2 {
		fmt.Printf("[%s] Unexpected: response too short (%d bytes)\n",
			time.Now().Format("15:04:05"), len(data))
		return
	}

	// Extract the CIP data type from the first 2 bytes.
	dataType := cip.DataType(binary.LittleEndian.Uint16(data[0:2]))
	valueBytes := data[2:]

	// Format the value based on its CIP data type.
	valueStr := formatValue(dataType, valueBytes)

	fmt.Printf("[%s] %s (%s) = %s\n",
		time.Now().Format("15:04:05"), tagName, dataType, valueStr)
}

// formatValue converts raw CIP value bytes into a human-readable string
// based on the CIP data type code.
func formatValue(dataType cip.DataType, valueBytes []byte) string {
	switch dataType {
	case cip.TypeDINT:
		// DINT: 32-bit signed integer, little-endian
		if len(valueBytes) >= 4 {
			val := int32(binary.LittleEndian.Uint32(valueBytes))
			return fmt.Sprintf("%d", val)
		}

	case cip.TypeREAL:
		// REAL: 32-bit IEEE 754 float, little-endian
		if len(valueBytes) >= 4 {
			bits := binary.LittleEndian.Uint32(valueBytes)
			val := math.Float32frombits(bits)
			return fmt.Sprintf("%.4f", val)
		}

	case cip.TypeINT:
		// INT: 16-bit signed integer, little-endian
		if len(valueBytes) >= 2 {
			val := int16(binary.LittleEndian.Uint16(valueBytes))
			return fmt.Sprintf("%d", val)
		}

	case cip.TypeSTRING:
		// STRING: [UINT length] [character data]
		if len(valueBytes) >= 2 {
			strLen := int(binary.LittleEndian.Uint16(valueBytes[0:2]))
			if len(valueBytes) >= 2+strLen {
				return fmt.Sprintf("%q", string(valueBytes[2:2+strLen]))
			}
		}

	case cip.TypeBOOL:
		// BOOL: single byte, 0 = false, non-zero = true
		if len(valueBytes) >= 1 {
			if valueBytes[0] != 0 {
				return "true"
			}
			return "false"
		}
	}

	// Unknown or unsupported type: show raw hex
	return fmt.Sprintf("0x%X (type=0x%04X)", valueBytes, uint16(dataType))
}
