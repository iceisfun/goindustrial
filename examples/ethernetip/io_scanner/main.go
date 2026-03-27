// Example: ethernetip/io_scanner
//
// Establishes an implicit I/O connection to a Rockwell Logix PLC via
// Forward_Open and exchanges cyclic assembly data over UDP.
//
// Before running, you need to know the PLC's assembly instance numbers
// for the I/O connection. Common defaults for a CompactLogix/ControlLogix:
//
//   - Input assembly (T→O):  instance 100 (0x64)
//   - Output assembly (O→T): instance 150 (0x96)
//   - Config assembly:       instance 151 (0x97)
//
// These depend on the PLC configuration. Check the controller properties
// in Studio 5000 under the Ethernet module's connection parameters.
//
// Usage:
//
//	go run . -addr 192.168.1.10 -ot-instance 150 -to-instance 100 -config-instance 151 -ot-size 8 -to-size 8
//	go run . -addr 192.168.1.10 -rpi 20ms -cycles 100
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	addr := flag.String("addr", "", "PLC address (host or host:port, default port 44818)")
	otInstance := flag.Int("ot-instance", 150, "O→T (output) assembly instance on the PLC")
	toInstance := flag.Int("to-instance", 100, "T→O (input) assembly instance on the PLC")
	cfgInstance := flag.Int("config-instance", 151, "Config assembly instance (0 to skip)")
	otSize := flag.Int("ot-size", 8, "O→T assembly size in bytes")
	toSize := flag.Int("to-size", 8, "T→O assembly size in bytes")
	rpi := flag.Duration("rpi", 10*time.Millisecond, "Requested Packet Interval")
	timeoutMult := flag.Int("timeout-mult", 3, "Timeout multiplier (timeout = RPI * 4 << mult)")
	cycles := flag.Int("cycles", 0, "Number of poll cycles to display (0 = run until Ctrl+C)")
	udpPort := flag.Int("udp-port", 2222, "Target UDP port on PLC")
	flag.Parse()

	if *addr == "" {
		fmt.Fprintln(os.Stderr, "Usage: go run . -addr <PLC_IP> [options]")
		fmt.Fprintln(os.Stderr, "  Example: go run . -addr 10.0.10.70")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// --- TCP Session ---

	fmt.Printf("Connecting to %s...\n", *addr)

	tcpAddr := *addr
	if _, _, err := net.SplitHostPort(tcpAddr); err != nil {
		tcpAddr = net.JoinHostPort(tcpAddr, "44818")
	}

	tc, err := ethernetip.NewTCPConn(tcpAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewTCPConn: %v\n", err)
		os.Exit(1)
	}

	sess := ethernetip.NewSession(tc, nil)
	if err := sess.Register(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Register session: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		sess.Unregister(context.Background())
		tc.Close()
	}()

	fmt.Printf("TCP session registered.\n")

	// --- Discover what we're connecting to ---

	fmt.Println()
	fmt.Println("Connection parameters:")
	fmt.Printf("  O→T assembly:  instance %d, %d bytes\n", *otInstance, *otSize)
	fmt.Printf("  T→O assembly:  instance %d, %d bytes\n", *toInstance, *toSize)
	if *cfgInstance > 0 {
		fmt.Printf("  Config assembly: instance %d\n", *cfgInstance)
	}
	fmt.Printf("  RPI:           %s\n", *rpi)
	fmt.Printf("  Timeout mult:  %d (timeout = %s)\n", *timeoutMult, *rpi*time.Duration(4<<*timeoutMult))

	// --- Try a tag read first to verify connectivity ---

	fmt.Println()
	readReq := &cip.MessageRouterRequest{
		Service:     cip.ServiceGetAttributeAll,
		RequestPath: cip.BuildPath(cip.ClassIdentity, 1, 0),
	}
	resp, err := sess.SendCIPRequest(ctx, readReq)
	if err != nil {
		fmt.Printf("Warning: Identity read failed: %v\n", err)
	} else if resp.IsSuccess() {
		fmt.Printf("Identity object read OK (%d bytes)\n", len(resp.ResponseData))
	} else {
		fmt.Printf("Warning: Identity read status 0x%02X\n", resp.GeneralStatus)
	}

	// --- IOScanner ---

	host, _, _ := net.SplitHostPort(tcpAddr)
	targetUDP, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, *udpPort))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Resolve UDP addr: %v\n", err)
		os.Exit(1)
	}

	scanner, err := ethernetip.NewIOScanner(sess, ":0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewIOScanner: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSending Forward_Open...\n")

	// Build the connection path. For a Logix PLC, the standard path uses
	// connection point segments to identify the O→T and T→O assemblies.
	connPath := cip.NewPath()
	connPath.AddClass(cip.ClassAssembly)
	connPath.AddInstance(cip.UINT(*cfgInstance))
	connPath.AddConnectionPoint(cip.UINT(*otInstance))
	connPath.AddConnectionPoint(cip.UINT(*toInstance))

	cfg := ethernetip.IOConnectionConfig{
		OTConnectionPoint: uint16(*otInstance),
		TOConnectionPoint: uint16(*toInstance),
		ConfigInstance:    uint16(*cfgInstance),
		OTSize:            uint16(*otSize),
		TOSize:            uint16(*toSize),
		RPI:               *rpi,
		TimeoutMultiplier: uint8(*timeoutMult),
		TargetAddr:        targetUDP,
		ConnectionPath:    connPath.Bytes(),
	}

	fmt.Printf("  Connection path: % X\n", cfg.ConnectionPath)

	ioConn, err := scanner.Open(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Forward_Open failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nCommon causes:")
		fmt.Fprintln(os.Stderr, "  - Wrong assembly instance numbers for this PLC")
		fmt.Fprintln(os.Stderr, "  - Wrong assembly sizes")
		fmt.Fprintln(os.Stderr, "  - PLC not configured for I/O connections")
		fmt.Fprintln(os.Stderr, "  - Another scanner already owns this connection")
		scanner.Shutdown(context.Background())
		os.Exit(1)
	}

	fmt.Printf("Forward_Open succeeded!\n")
	fmt.Printf("  OT Connection ID: 0x%08X\n", ioConn.OTConnectionID)
	fmt.Printf("  TO Connection ID: 0x%08X\n", ioConn.TOConnectionID)
	fmt.Printf("  Actual RPI:       %s\n", ioConn.RPI)
	fmt.Printf("\nCyclic I/O running. Displaying input assembly:\n")
	fmt.Println("---")

	// --- Cyclic display loop ---

	cycle := 0
	ticker := time.NewTicker(500 * time.Millisecond) // display rate
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			goto shutdown
		case <-ticker.C:
			cycle++
			if *cycles > 0 && cycle > *cycles {
				goto shutdown
			}

			input := ioConn.Input()
			age := ioConn.InputAge()
			timedOut := ""
			if ioConn.IsTimedOut() {
				timedOut = " [TIMED OUT]"
			}

			fmt.Printf("[%4d] T→O: % X  (age: %6s)%s\n",
				cycle, input, age.Truncate(time.Millisecond), timedOut)
		}
	}

shutdown:
	fmt.Println("\n--- Shutting down ---")
	fmt.Println("Sending Forward_Close...")

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()

	if err := scanner.Close(closeCtx, ioConn); err != nil {
		fmt.Printf("Forward_Close: %v\n", err)
	} else {
		fmt.Println("Forward_Close succeeded.")
	}

	scanner.Shutdown(closeCtx)
	fmt.Println("Done.")
}
