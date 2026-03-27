// Example: ethernetip/adapter
//
// Demonstrates an EtherNet/IP adapter (target device) that accepts implicit
// I/O connections from a scanner via Forward_Open and exchanges cyclic
// assembly data over UDP.
//
// This is the server side of implicit messaging. A scanner (PLC or another
// host) sends Forward_Open to establish the connection, then both sides
// exchange assembly data at the negotiated RPI.
//
// The adapter creates two assembly instances:
//   - Instance 100 (consume): receives O→T data from the scanner
//   - Instance 101 (produce): sends T→O data back to the scanner
//
// A simple application loop reads the consumed data and echoes it back
// with a cycle counter in the first two bytes.
//
// Usage:
//
//	go run . -tcp :44818 -udp :2222 -ot-size 12 -to-size 12
//	go run . -tcp :44818 -udp :2222 -ot-size 4 -to-size 4
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/logging"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/eip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/assembly"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/objects/connmgr"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/runtime"
)

func main() {
	tcpAddr := flag.String("tcp", ":44818", "TCP listen address for EIP sessions")
	udpAddr := flag.String("udp", ":2222", "UDP listen address for implicit I/O")
	otSize := flag.Int("ot-size", 12, "O→T (consume) assembly size in bytes")
	toSize := flag.Int("to-size", 12, "T→O (produce) assembly size in bytes")
	flag.Parse()

	logger := logging.NewDefaultLogger(logging.WithLevel(logging.LevelInfo))

	// -------------------------------------------------------------------
	// Assembly Object — I/O data buffers
	// -------------------------------------------------------------------
	// Instance 100: data the scanner sends TO us (O→T, consume)
	// Instance 101: data we send BACK to the scanner (T→O, produce)

	ao := assembly.NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, *otSize))
	ao.RegisterAssembly(101, make([]byte, *toSize))

	fmt.Printf("Assembly instances:\n")
	fmt.Printf("  Instance 100 (consume): %d bytes\n", *otSize)
	fmt.Printf("  Instance 101 (produce): %d bytes\n", *toSize)

	// -------------------------------------------------------------------
	// UDP Runtime — handles implicit I/O packet send/receive
	// -------------------------------------------------------------------

	rt := runtime.NewRuntime(ao)
	if err := rt.Start(*udpAddr); err != nil {
		fmt.Fprintf(os.Stderr, "UDP runtime start: %v\n", err)
		os.Exit(1)
	}
	defer rt.Stop()
	fmt.Printf("UDP runtime listening on %s\n", rt.Addr())

	// -------------------------------------------------------------------
	// Scheduler — sends produce data at the negotiated RPI
	// -------------------------------------------------------------------

	sched := runtime.NewScheduler(rt)
	sched.Start()
	defer sched.Stop()

	// -------------------------------------------------------------------
	// Connection Manager — handles Forward_Open / Forward_Close
	// -------------------------------------------------------------------
	// The OnOpen callback wires the scanner's connection to the runtime
	// so that UDP packets flow to/from the correct assembly instances.

	var activeConns atomic.Int32

	cm := connmgr.NewConnectionManager(
		connmgr.WithOnOpen(func(c *connmgr.Connection, req *connmgr.ForwardOpenRequest) {
			rpi := time.Duration(req.OTRPI) * time.Microsecond
			if rpi < time.Millisecond {
				rpi = 10 * time.Millisecond
			}

			fmt.Printf("\n[OPEN] Connection from scanner\n")
			fmt.Printf("  OT Connection ID: 0x%08X\n", c.OTConnectionID)
			fmt.Printf("  TO Connection ID: 0x%08X\n", c.TOConnectionID)
			fmt.Printf("  RPI:              %s\n", rpi)
			fmt.Printf("  Serial:           0x%04X\n", c.ConnectionSerialNumber)

			// Consumer: receives data from the scanner on OTConnectionID
			rt.AddConnection(&runtime.IOConnection{
				ConnectionID:  c.OTConnectionID,
				RPI:           rpi,
				Assembly:      ao.GetInstance(100),
				IsConsumer:    true,
				TimeoutMult:   uint8(req.ConnectionTimeoutMultiplier),
				RunIdleHeader: false,
				StopChan:      make(chan struct{}),
			})

			// Producer: sends data to the scanner on TOConnectionID
			// Note: RemoteAddr is set when we receive the first UDP packet
			// from the scanner (the runtime records the source address).
			rt.AddConnection(&runtime.IOConnection{
				ConnectionID:  c.TOConnectionID,
				RPI:           rpi,
				Assembly:      ao.GetInstance(101),
				IsProducer:    true,
				RunIdleHeader: false,
				StopChan:      make(chan struct{}),
			})

			activeConns.Add(1)
		}),
		connmgr.WithOnClose(func(c *connmgr.Connection) {
			fmt.Printf("\n[CLOSE] Connection 0x%08X / 0x%08X\n",
				c.OTConnectionID, c.TOConnectionID)
			rt.RemoveConnection(c.OTConnectionID)
			rt.RemoveConnection(c.TOConnectionID)
			activeConns.Add(-1)
		}),
	)

	// -------------------------------------------------------------------
	// Message Router — dispatches CIP requests
	// -------------------------------------------------------------------

	router := cip.NewMessageRouter()
	router.RegisterObject(cip.ClassConnectionMgr, cm)
	router.RegisterObject(cip.ClassAssembly, ao)

	// -------------------------------------------------------------------
	// EIP Server — TCP session management
	// -------------------------------------------------------------------

	srv := ethernetip.NewServer(router,
		ethernetip.WithServerLogger(logger),
		ethernetip.WithIdentity(eip.ListIdentityItem{
			TypeID:        eip.ItemIDListIdentity,
			EncapsVersion: 1,
			VendorID:      1,
			DeviceType:    7, // General Purpose Discrete I/O
			ProductCode:   1,
			Revision:      [2]byte{1, 0},
			ProductName:   "GoIndustrial Adapter",
		}),
	)

	if err := srv.Start(context.Background(), *tcpAddr); err != nil {
		fmt.Fprintf(os.Stderr, "TCP server start: %v\n", err)
		os.Exit(1)
	}
	defer srv.Stop()

	fmt.Printf("EIP server listening on %s\n", *tcpAddr)
	fmt.Println()
	fmt.Println("Waiting for scanner connections (Forward_Open)...")
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// -------------------------------------------------------------------
	// Application loop — process I/O data
	// -------------------------------------------------------------------
	// Read consumed data (from scanner), echo it back in the produce
	// assembly with a cycle counter in the first 2 bytes.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	var cycle uint16
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	displayTicker := time.NewTicker(time.Second)
	defer displayTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Printf("\nActive connections: %d\n", activeConns.Load())
			fmt.Println("Done.")
			return

		case <-ticker.C:
			if activeConns.Load() == 0 {
				continue
			}

			cycle++

			// Read what the scanner sent us.
			consumed, err := ao.GetAttributeSingle(100, 3)
			if err != nil {
				continue
			}

			// Build produce data: cycle counter + echo of consumed data.
			produce := make([]byte, *toSize)
			if len(produce) >= 2 {
				binary.LittleEndian.PutUint16(produce[0:2], cycle)
			}
			copyLen := len(consumed)
			if copyLen > len(produce)-2 {
				copyLen = len(produce) - 2
			}
			if copyLen > 0 {
				copy(produce[2:2+copyLen], consumed)
			}

			ao.SetAttributeSingle(101, 3, produce)

		case <-displayTicker.C:
			if activeConns.Load() == 0 {
				continue
			}

			consumed, _ := ao.GetAttributeSingle(100, 3)
			produced, _ := ao.GetAttributeSingle(101, 3)

			preview := func(d []byte) string {
				if len(d) > 16 {
					return fmt.Sprintf("% X...", d[:16])
				}
				return fmt.Sprintf("% X", d)
			}

			fmt.Printf("[cycle %5d] consume=%-40s produce=%s\n",
				cycle, preview(consumed), preview(produced))
		}
	}
}
