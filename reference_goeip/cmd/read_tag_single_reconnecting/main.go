package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iceisfun/goeip/internal"
	"github.com/iceisfun/goeip/pkg/client"
	"github.com/iceisfun/goeip/pkg/utils"
)

/*
Testing Network Interrupts with SOCAT
====================================

You can simulate network disconnections using `socat`. This allows you to
create a TCP proxy that you can kill and restart to test reconnection.

1.  **Assume your PLC is at 192.168.1.10:44818**
2.  **Start `socat` to proxy local port 44818 to the PLC**:
    ```bash
    socat -v TCP4-LISTEN:44818,fork,reuseaddr TCP4:192.168.1.10:44818
    ```
    (You might need `sudo` to listen on 44818, or use a higher port like 8888)

    If you use port 8888:
    ```bash
    socat -v TCP4-LISTEN:8888,fork,reuseaddr TCP4:192.168.1.10:44818
    ```

3.  **Run this tool pointing to the proxy**:
    ```bash
    go run cmd/read_tag_single_reconnecting/main.go --addr localhost:8888 --tag MyTag
    ```

4.  **Interrupt the Connection**:
    - Kill the `socat` process (Ctrl+C).
    - Observe the tool reporting errors.
    - Restart `socat`.
    - Observe the tool recovering and reading data again.

5.  **Observe reconnecting behavior**:

$ go run ./cmd/read_tag_single_reconnecting/ --addr 127.0.0.1 --tag TheTestTag


	Connecting to 127.0.0.1...
	Reading tag 'TheTestTag' every second. Press Ctrl+C to stop.
	---------------------------------------------------------
	[12:53:40] #1 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:41] #2 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:42] #3 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:43] #4 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:47] #5 ERROR: operation failed after 6 retries
	[12:53:49] #6 ERROR: operation failed after 6 retries
	[12:53:50] #7 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:50] #8 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

	[12:53:51] #9 SUCCESS: 3 bytes
	00000000  c1 00 01                                          |...|

*/

func main() {
	addr := flag.String("addr", "localhost:44818", "PLC address")
	tagName := flag.String("tag", "", "Tag name to read")
	verbose := flag.Bool("v", false, "Verbose logging")
	flag.Parse()

	if *tagName == "" {
		fmt.Println("Error: --tag is required")
		flag.Usage()
		os.Exit(1)
	}

	logger := internal.NopLogger()
	if *verbose {
		logger = internal.NewConsoleLogger()
	}

	fmt.Printf("Connecting to %s...\n", *addr)

	// Create a reconnecting transport with lifecycle hooks
	t := client.NewReconnectingTransport(*addr, logger,
		client.WithOnConnect(func() {
			fmt.Println("[transport] connected")
		}),
		client.WithOnDisconnect(func(err error) {
			fmt.Printf("[transport] disconnected: %v\n", err)
		}),
	)

	// Create client with retries
	c := client.NewClient(t,
		client.WithRetries(5),
		client.WithRetryDelay(500*time.Millisecond),
		client.WithLogger(logger),
	)
	defer c.Close()

	fmt.Printf("Reading tag '%s' every second. Press Ctrl+C to stop.\n", *tagName)
	fmt.Println("---------------------------------------------------------")

	idx := 1
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			val, err := c.ReadTag(*tagName)
			ts := time.Now().Format("15:04:05")

			if err != nil {
				fmt.Printf("[%s] #%d ERROR: %v\n", ts, idx, err)
			} else {
				fmt.Printf("[%s] #%d SUCCESS: %d bytes\n%s\n", ts, idx, len(val), utils.HexDump(val))
			}
			idx++
		}
	}
}
