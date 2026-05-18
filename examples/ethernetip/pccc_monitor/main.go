// Command pccc_monitor polls one or more PCCC data-table addresses on an
// SLC 500 / MicroLogix controller and prints every value change.
//
// It demonstrates the protocol-agnostic monitor package wrapping the
// high-level pccc.Client. The pccc.Client implements plc.Reader, so the
// monitor needs no PCCC-specific code.
//
// Usage:
//
//	go run . -addr 10.30.40.71:44818 -tags N7:0,N7:1,F8:5,B3:0/2
//	go run . -addr 10.30.40.71:44818 -tags T4:0.ACC -freq 200ms
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iceisfun/goindustrial/monitor"
	"github.com/iceisfun/goindustrial/plc"
	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/pccc"
)

func main() {
	addr := flag.String("addr", "10.30.40.71:44818", "SLC/MicroLogix address (host:port)")
	tagList := flag.String("tags", "N7:0", "comma-separated PCCC addresses to poll")
	freq := flag.Duration("freq", 500*time.Millisecond, "poll frequency per address")
	flag.Parse()

	tags := strings.Split(*tagList, ",")
	for i := range tags {
		tags[i] = strings.TrimSpace(tags[i])
	}
	if len(tags) == 0 || tags[0] == "" {
		fmt.Fprintln(os.Stderr, "error: -tags is required")
		os.Exit(1)
	}

	// Dial the controller and register an EIP session.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eip, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("connect %s: %v", *addr, err)
	}
	defer eip.Close()
	fmt.Printf("Connected to %s\n", *addr)

	// Wrap the EIP client in a PCCC client. pccc.Client implements
	// plc.Reader, so the monitor accepts it directly.
	client := pccc.NewClient(eip)

	mon, err := monitor.NewMonitor(client)
	if err != nil {
		log.Fatalf("NewMonitor: %v", err)
	}
	defer mon.Close()

	sub, err := mon.NewSubscriber(64)
	if err != nil {
		log.Fatalf("NewSubscriber: %v", err)
	}
	defer sub.Done()

	for _, t := range tags {
		_, err := mon.Subscribe(
			pccc.File{Address: t},
			monitor.WithFrequency(*freq),
		)
		if err != nil {
			log.Fatalf("subscribe %q: %v", t, err)
		}
		fmt.Printf("  subscribed %s @ %s\n", t, *freq)
	}

	// Graceful shutdown on Ctrl-C.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	fmt.Println("Polling — Ctrl-C to stop.")
	for {
		select {
		case <-sig:
			fmt.Println("\nstopping…")
			return
		case ev := <-sub.Events():
			ts := ev.Snapshot.Timestamp.Format("15:04:05.000")
			if ev.Err != nil {
				fmt.Printf("[%s] %s: error: %v\n", ts, ev.Snapshot.Point, ev.Err)
				continue
			}
			if !ev.Changed {
				continue
			}
			fmt.Printf("[%s] %s = %s\n", ts, ev.Snapshot.Point, formatValue(ev.Snapshot.Value))
		}
	}
}

func formatValue(v plc.Value) string {
	switch v.Type {
	case plc.TypeBool:
		return fmt.Sprintf("%v", v.Bool())
	case plc.TypeFloat32:
		if f, err := v.Float32(); err == nil {
			return fmt.Sprintf("%g", f)
		}
	case plc.TypeInt16:
		if i, err := v.Int(); err == nil {
			return fmt.Sprintf("%d", i)
		}
	}
	return fmt.Sprintf("% X", v.Raw)
}
