package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Moonlight-Companies/gomodbus/client"
	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

func main() {
	// Create a logger with debug level
	logger := logging.NewLogger(
		logging.WithLevel(common.LevelDebug),
	)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get server host from command line or use default
	host := "localhost"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}

	// Create a TCP client with options
	modbusClient := client.NewTCPClient(
		host,
		transport.WithPort(502),
		transport.WithTimeoutOption(5*time.Second),
		transport.WithTransportLogger(logger),
	).WithOptions(
		client.WithTCPUnitID(1),
		client.WithTCPLogger(logger),
	)

	// Connect to the server
	err := modbusClient.Connect(ctx)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		return
	}
	defer modbusClient.Disconnect(context.Background())

	fmt.Println("Connected to Modbus server")

	// Use a wait group to wait for all goroutines to complete
	var wg sync.WaitGroup

	// Run multiple concurrent requests
	numRequests := 10
	wg.Add(numRequests)

	// Create channels to collect results
	results := make(chan string, numRequests)
	errors := make(chan error, numRequests)

	startTime := time.Now()

	// Start multiple goroutines to read holding registers concurrently
	for i := 0; i < numRequests; i++ {
		go func(index int) {
			defer wg.Done()

			// Create a context with timeout for each request
			reqCtx, reqCancel := context.WithTimeout(ctx, 2*time.Second)
			defer reqCancel()

			// Read different holding registers for each request
			address := common.Address(1000 + index*10)
			quantity := common.Quantity(10)

			fmt.Printf("Request %d: Reading %d holding registers starting at %d\n",
				index, quantity, address)

			values, err := modbusClient.ReadHoldingRegisters(reqCtx, address, quantity)
			if err != nil {
				fmt.Printf("Request %d failed: %v\n", index, err)
				errors <- fmt.Errorf("request %d failed: %w", index, err)
				return
			}

			// Process the results
			result := fmt.Sprintf("Request %d completed: Read %d values from address %d",
				index, len(values), address)
			fmt.Println(result)
			results <- result

		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(results)
	close(errors)

	// Process results and errors
	successCount := 0
	errorCount := 0

	for result := range results {
		fmt.Println("Result:", result)
		successCount++
	}

	for err := range errors {
		fmt.Println("Error:", err)
		errorCount++
	}

	elapsedTime := time.Since(startTime)
	fmt.Printf("\nCompleted %d requests with %d successes and %d errors in %v\n",
		numRequests, successCount, errorCount, elapsedTime)
}
