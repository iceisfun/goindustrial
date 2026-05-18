// Command pccc_write writes a value to a single PCCC data-table address.
//
// Usage:
//
//	# Write an integer
//	go run . -addr 10.30.40.71:44818 -tag N7:0 -value 42
//
//	# Write a float
//	go run . -addr 10.30.40.71:44818 -tag F8:5 -value 3.14
//
//	# Set or clear a bit (read-modify-write under the hood)
//	go run . -addr 10.30.40.71:44818 -tag B3:0/2 -value 1
//	go run . -addr 10.30.40.71:44818 -tag B3:0/2 -value 0
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/pccc"
)

func main() {
	addr := flag.String("addr", "10.30.40.71:44818", "SLC/MicroLogix address (host:port)")
	tag := flag.String("tag", "N7:0", "PCCC data-table address (e.g. N7:0, F8:5, B3:0/2)")
	val := flag.String("value", "0", "value to write (int, float, or 0/1 for bits)")
	flag.Parse()

	a, err := pccc.ParseAddress(*tag)
	if err != nil {
		log.Fatalf("parse %q: %v", *tag, err)
	}

	// Coerce -value to the right Go type for the address.
	var goVal any
	switch {
	case a.BitNum >= 0:
		switch strings.ToLower(*val) {
		case "1", "true", "on", "high":
			goVal = true
		case "0", "false", "off", "low":
			goVal = false
		default:
			log.Fatalf("bit write: -value must be 0 or 1 (got %q)", *val)
		}
	case a.FileType == pccc.FileTypeFloat:
		f, err := strconv.ParseFloat(*val, 32)
		if err != nil {
			log.Fatalf("float write: %v", err)
		}
		goVal = float32(f)
	default:
		n, err := strconv.Atoi(*val)
		if err != nil {
			log.Fatalf("integer write: %v", err)
		}
		goVal = int16(n)
	}

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

	client := pccc.NewClient(eip)
	if err := client.WriteAddress(ctx, *tag, goVal); err != nil {
		log.Fatalf("WriteAddress %s = %v: %v", *tag, goVal, err)
	}
	fmt.Printf("wrote %s = %v\n", *tag, goVal)
}
