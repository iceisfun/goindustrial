// Command timer_counter reads Timer and Counter structured tags from a
// Rockwell Logix PLC and displays all their fields.
//
// Rockwell Logix controllers represent Timer (TON, TOF, RTO) and Counter
// (CTU, CTD, CTUD) instructions as structured tags with a fixed 14-byte
// memory layout:
//
//	Offset  Size   Field
//	------  -----  --------------------------------------------------
//	0-1     INT    Reserved (2 bytes, typically zero, alignment padding)
//	2-5     DINT   Status bits (packed boolean flags as bit positions)
//	6-9     DINT   PRE (preset value)
//	10-13   DINT   ACC (accumulated value)
//
// Timer status bits (within the 32-bit status DINT):
//
//	Bit 31: EN  (Enable)       - true while the timer instruction is enabled
//	Bit 30: TT  (Timer Timing) - true while the timer is actively counting
//	Bit 29: DN  (Done)         - true when ACC >= PRE
//
// Counter status bits (within the 32-bit status DINT):
//
//	Bit 31: CU  (Count Up)    - true when count-up is active
//	Bit 30: CD  (Count Down)  - true when count-down is active
//	Bit 29: DN  (Done)        - true when ACC >= PRE
//	Bit 28: OV  (Overflow)    - true when ACC overflows past +2^31-1
//	Bit 27: UN  (Underflow)   - true when ACC underflows past -2^31
//
// Both structures share the same 14-byte layout. The only difference is
// which status bits are meaningful.
//
// The CIP type code for these tags is >= 0x02A0 (STRUCT), followed by a
// 2-byte structure handle that identifies the specific structure definition.
// The response header is therefore 4 bytes instead of the usual 2.
//
// Usage:
//
//	go run . -addr 192.168.1.10:44818 -timer-tag Timer_1 -counter-tag Counter_1
//	go run . -addr 192.168.1.10:44818 -timer-tag Timer_1
//	go run . -addr 192.168.1.10:44818 -counter-tag Counter_1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/iceisfun/goindustrial/protocol/ethernetip"
	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func main() {
	// -----------------------------------------------------------------------
	// Parse command-line flags.
	// -----------------------------------------------------------------------
	addr := flag.String("addr", "192.168.1.10:44818", "PLC address in host:port format")
	timerTag := flag.String("timer-tag", "", "Timer tag name (e.g. Timer_1)")
	counterTag := flag.String("counter-tag", "", "Counter tag name (e.g. Counter_1)")
	flag.Parse()

	if *timerTag == "" && *counterTag == "" {
		fmt.Println("error: at least one of -timer-tag or -counter-tag is required")
		flag.Usage()
		return
	}

	// -----------------------------------------------------------------------
	// Connect to the PLC.
	//
	// The connection sequence is:
	//   1. TCP dial to host:44818
	//   2. EIP RegisterSession (command 0x0065) -> session handle
	//   3. All CIP requests are wrapped in EIP SendRRData (command 0x006F)
	// -----------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := ethernetip.Connect(ctx, *addr,
		ethernetip.WithRetries(2),
		ethernetip.WithRetryDelay(500*time.Millisecond),
	)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", *addr, err)
	}
	defer client.Close()

	fmt.Printf("Connected to %s\n\n", *addr)

	// -----------------------------------------------------------------------
	// Read the Timer tag if specified.
	//
	// client.ReadTimer is a convenience method that:
	//   1. Sends CIP Read Tag (0x4C) for the tag name.
	//   2. Parses the 4-byte struct header (2-byte type code + 2-byte handle).
	//   3. Calls cip.DecodeTimer on the remaining 14 bytes.
	//
	// The 14-byte structure is the canonical Rockwell memory layout for
	// TON/TOF/RTO timer instructions. The PRE and ACC fields are in
	// milliseconds. For example, a timer with a 5-second preset will have
	// PRE = 5000.
	// -----------------------------------------------------------------------
	if *timerTag != "" {
		fmt.Printf("=== Timer: %s ===\n\n", *timerTag)

		timer, err := client.ReadTimer(ctx, *timerTag)
		if err != nil {
			log.Fatalf("ReadTimer(%q) failed: %v", *timerTag, err)
		}

		printTimer(timer)
		fmt.Println()
	}

	// -----------------------------------------------------------------------
	// Read the Counter tag if specified.
	//
	// There is no convenience ReadCounter method, so we use ReadTagInto
	// which calls ReadTag, strips the struct header, and then calls
	// cip.Unmarshal. Because cip.Counter implements cip.Unmarshaler, the
	// custom UnmarshalCIP method handles the 14-byte layout and bit
	// extraction.
	//
	// Counter tags are created by CTU (Count Up), CTD (Count Down), or
	// CTUD (Count Up/Down) instructions in the PLC program. The PRE and
	// ACC fields are dimensionless integer counts.
	// -----------------------------------------------------------------------
	if *counterTag != "" {
		fmt.Printf("=== Counter: %s ===\n\n", *counterTag)

		var counter cip.Counter
		if err := client.ReadTagInto(ctx, *counterTag, &counter); err != nil {
			log.Fatalf("ReadTagInto(%q) failed: %v", *counterTag, err)
		}

		printCounter(&counter)
		fmt.Println()
	}

	fmt.Println("Done.")
}

// printTimer displays all fields of a Timer in a human-readable format.
func printTimer(t *cip.Timer) {
	// PRE (Preset) is the target time in milliseconds. When ACC reaches PRE,
	// the DN (Done) bit is set. For TON timers, TT (Timer Timing) is true
	// while ACC < PRE and the rung is true.
	fmt.Printf("  PRE (Preset):       %d ms", t.PRE)
	if t.PRE > 0 {
		fmt.Printf("  (%.1f seconds)", float64(t.PRE)/1000.0)
	}
	fmt.Println()

	// ACC (Accumulated) is the current elapsed time in milliseconds.
	fmt.Printf("  ACC (Accumulated):  %d ms", t.ACC)
	if t.ACC > 0 {
		fmt.Printf("  (%.1f seconds)", float64(t.ACC)/1000.0)
	}
	fmt.Println()

	// Progress bar showing ACC relative to PRE.
	if t.PRE > 0 {
		pct := float64(t.ACC) / float64(t.PRE) * 100.0
		if pct > 100 {
			pct = 100
		}
		fmt.Printf("  Progress:           %.1f%%\n", pct)
	}

	fmt.Println()

	// Status bits packed in the DINT at offset 2-5 of the structure:
	//   Bit 31: EN (Enable)       - Set when the timer rung-in is true.
	//   Bit 30: TT (Timer Timing) - Set while EN is true and ACC < PRE.
	//   Bit 29: DN (Done)         - Set when ACC >= PRE.
	fmt.Printf("  EN  (Enable):       %v\n", t.EN)
	fmt.Printf("  TT  (Timer Timing): %v\n", t.TT)
	fmt.Printf("  DN  (Done):         %v\n", t.DN)

	// Interpret the overall state for clarity.
	fmt.Printf("\n  State: ")
	switch {
	case !t.EN:
		fmt.Println("Idle (timer is not enabled)")
	case t.TT && !t.DN:
		fmt.Println("Timing (actively counting up)")
	case t.DN:
		fmt.Println("Done (preset reached)")
	default:
		fmt.Println("Unknown")
	}
}

// printCounter displays all fields of a Counter in a human-readable format.
func printCounter(c *cip.Counter) {
	// PRE (Preset) is the target count. When ACC reaches PRE, DN is set.
	fmt.Printf("  PRE (Preset):       %d\n", c.PRE)

	// ACC (Accumulated) is the current count value. It increments on each
	// CTU false-to-true transition, or decrements on each CTD transition.
	fmt.Printf("  ACC (Accumulated):  %d\n", c.ACC)

	if c.PRE > 0 {
		pct := float64(c.ACC) / float64(c.PRE) * 100.0
		fmt.Printf("  Progress:           %.1f%%\n", pct)
	}

	fmt.Println()

	// Status bits packed in the DINT at offset 2-5 of the structure:
	//   Bit 31: CU (Count Up)    - Set on rising edge of CTU rung-in.
	//   Bit 30: CD (Count Down)  - Set on rising edge of CTD rung-in.
	//   Bit 29: DN (Done)        - Set when ACC >= PRE.
	//   Bit 28: OV (Overflow)    - Set when ACC wraps past +2,147,483,647.
	//   Bit 27: UN (Underflow)   - Set when ACC wraps past -2,147,483,648.
	fmt.Printf("  CU  (Count Up):     %v\n", c.CU)
	fmt.Printf("  CD  (Count Down):   %v\n", c.CD)
	fmt.Printf("  DN  (Done):         %v\n", c.DN)
	fmt.Printf("  OV  (Overflow):     %v\n", c.OV)
	fmt.Printf("  UN  (Underflow):    %v\n", c.UN)

	// Interpret the overall state.
	fmt.Printf("\n  State: ")
	switch {
	case c.OV:
		fmt.Println("Overflow (ACC wrapped past max int32)")
	case c.UN:
		fmt.Println("Underflow (ACC wrapped past min int32)")
	case c.DN:
		fmt.Println("Done (preset reached)")
	case c.CU || c.CD:
		fmt.Println("Counting")
	default:
		fmt.Println("Idle")
	}
}
