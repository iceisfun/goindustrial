package gomodbus

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Moonlight-Companies/gomodbus/client"
	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
	"github.com/Moonlight-Companies/gomodbus/server"
	"github.com/Moonlight-Companies/gomodbus/transport"
)

// TestClientServerIntegration performs an integration test with a real TCP client and server
func TestClientServerIntegration(t *testing.T) {
	// Create a test logger
	logger := logging.NewLogger(logging.WithLevel(common.LevelDebug))

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the server with a memory store
	store := server.NewMemoryStore()

	// Pre-load some test data
	store.SetCoil(common.Address(1000), true)
	store.SetCoil(common.Address(1001), false)
	store.SetCoil(common.Address(1002), true)

	store.SetHoldingRegister(common.Address(2000), 0x1234)
	store.SetHoldingRegister(common.Address(2001), 0x5678)

	store.SetInputRegister(common.Address(3000), 0xABCD)
	store.SetInputRegister(common.Address(3001), 0xEF01)

	// Create a listener on a random free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Get the actual port
	serverPort := listener.Addr().(*net.TCPAddr).Port

	// Create the server
	modbusServer := server.NewTCPServer(
		"127.0.0.1",
		server.WithServerListener(listener), // Use the pre-created listener
		server.WithServerLogger(logger),
		server.WithServerDataStore(store),
	)

	// Start the server in a goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- modbusServer.Start(ctx)
	}()

	// Wait briefly for the server to start
	time.Sleep(100 * time.Millisecond)

	// Create a client that connects to the server
	modbusClient := client.NewTCPClient(
		"127.0.0.1",
		transport.WithPort(serverPort),
		transport.WithTimeoutOption(5*time.Second),
		transport.WithTransportLogger(logger),
	).WithOptions(
		client.WithTCPUnitID(1),
		client.WithTCPLogger(logger),
	)

	// Connect to the server
	err = modbusClient.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer modbusClient.Disconnect(context.Background())

	// Test reading coils
	coils, err := modbusClient.ReadCoils(ctx, common.Address(1000), common.Quantity(3))
	if err != nil {
		t.Fatalf("ReadCoils failed: %v", err)
	}

	expectedCoils := []common.CoilValue{true, false, true}
	if len(coils) != len(expectedCoils) {
		t.Fatalf("Expected %d coils, got %d", len(expectedCoils), len(coils))
	}

	for i, expected := range expectedCoils {
		if coils[i] != expected {
			t.Errorf("Coil %d: expected %t, got %t", i, expected, coils[i])
		}
	}

	// Test reading holding registers
	holdingRegisters, err := modbusClient.ReadHoldingRegisters(ctx, common.Address(2000), common.Quantity(2))
	if err != nil {
		t.Fatalf("ReadHoldingRegisters failed: %v", err)
	}

	expectedHoldingRegisters := []common.RegisterValue{0x1234, 0x5678}
	if len(holdingRegisters) != len(expectedHoldingRegisters) {
		t.Fatalf("Expected %d holding registers, got %d",
			len(expectedHoldingRegisters), len(holdingRegisters))
	}

	for i, expected := range expectedHoldingRegisters {
		if holdingRegisters[i] != expected {
			t.Errorf("Holding register %d: expected 0x%04X, got 0x%04X",
				i, expected, holdingRegisters[i])
		}
	}

	// Test reading input registers
	inputRegisters, err := modbusClient.ReadInputRegisters(ctx, common.Address(3000), common.Quantity(2))
	if err != nil {
		t.Fatalf("ReadInputRegisters failed: %v", err)
	}

	expectedInputRegisters := []common.InputRegisterValue{0xABCD, 0xEF01}
	if len(inputRegisters) != len(expectedInputRegisters) {
		t.Fatalf("Expected %d input registers, got %d",
			len(expectedInputRegisters), len(inputRegisters))
	}

	for i, expected := range expectedInputRegisters {
		if inputRegisters[i] != expected {
			t.Errorf("Input register %d: expected 0x%04X, got 0x%04X",
				i, expected, inputRegisters[i])
		}
	}

	// Test writing a single coil
	err = modbusClient.WriteSingleCoil(ctx, common.Address(1010), common.CoilValue(true))
	if err != nil {
		t.Fatalf("WriteSingleCoil failed: %v", err)
	}

	// Verify the coil was written
	coilValue, ok := store.GetCoil(common.Address(1010))
	if !ok {
		t.Fatal("Coil at address 1010 was not written")
	}

	if coilValue != true {
		t.Errorf("Expected coil value true, got %t", coilValue)
	}

	// Test writing a single register
	err = modbusClient.WriteSingleRegister(ctx, common.Address(2010), common.RegisterValue(0x4321))
	if err != nil {
		t.Fatalf("WriteSingleRegister failed: %v", err)
	}

	// Verify the register was written
	registerValue, ok := store.GetHoldingRegister(common.Address(2010))
	if !ok {
		t.Fatal("Register at address 2010 was not written")
	}

	if registerValue != 0x4321 {
		t.Errorf("Expected register value 0x4321, got 0x%04X", registerValue)
	}

	// Test writing multiple coils
	coilValues := []common.CoilValue{true, false, true, false}
	err = modbusClient.WriteMultipleCoils(ctx, common.Address(1020), coilValues)
	if err != nil {
		t.Fatalf("WriteMultipleCoils failed: %v", err)
	}

	// Verify the coils were written
	for i, expected := range coilValues {
		addr := common.Address(1020 + i)
		coilValue, ok := store.GetCoil(addr)
		if !ok {
			t.Fatalf("Coil at address %d was not written", addr)
		}

		if coilValue != expected {
			t.Errorf("Coil at address %d: expected %t, got %t", addr, expected, coilValue)
		}
	}

	// Test writing multiple registers
	registerValues := []common.RegisterValue{0x1111, 0x2222, 0x3333}
	err = modbusClient.WriteMultipleRegisters(ctx, common.Address(2020), registerValues)
	if err != nil {
		t.Fatalf("WriteMultipleRegisters failed: %v", err)
	}

	// Verify the registers were written
	for i, expected := range registerValues {
		addr := common.Address(2020 + i)
		registerValue, ok := store.GetHoldingRegister(addr)
		if !ok {
			t.Fatalf("Register at address %d was not written", addr)
		}

		if registerValue != expected {
			t.Errorf("Register at address %d: expected 0x%04X, got 0x%04X",
				addr, expected, registerValue)
		}
	}

	// Test read-write multiple registers
	readAddress := common.Address(2000)
	readQuantity := common.Quantity(2)
	writeAddress := common.Address(2030)
	writeValues := []common.RegisterValue{0xAAAA, 0xBBBB}

	readValues, err := modbusClient.ReadWriteMultipleRegisters(
		ctx, readAddress, readQuantity, writeAddress, writeValues)
	if err != nil {
		t.Fatalf("ReadWriteMultipleRegisters failed: %v", err)
	}

	// Verify the read values
	expectedReadValues := []common.RegisterValue{0x1234, 0x5678}
	if len(readValues) != len(expectedReadValues) {
		t.Fatalf("Expected %d read values, got %d",
			len(expectedReadValues), len(readValues))
	}

	for i, expected := range expectedReadValues {
		if readValues[i] != expected {
			t.Errorf("Read value %d: expected 0x%04X, got 0x%04X",
				i, expected, readValues[i])
		}
	}

	// Verify the write values
	for i, expected := range writeValues {
		addr := writeAddress + common.Address(i)
		registerValue, ok := store.GetHoldingRegister(addr)
		if !ok {
			t.Fatalf("Register at address %d was not written", addr)
		}

		if registerValue != expected {
			t.Errorf("Written register at address %d: expected 0x%04X, got 0x%04X",
				addr, expected, registerValue)
		}
	}

	// Stop the server
	err = modbusServer.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}

	// Check if there was an error starting the server
	select {
	case err := <-serverErrCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Server error: %v", err)
		}
	default:
		// Server is still running, this is fine
	}
}
