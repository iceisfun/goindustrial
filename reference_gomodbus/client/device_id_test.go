package client

import (
	"context"
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/common/test"
	"github.com/Moonlight-Companies/gomodbus/protocol"
)

func TestBaseClient_ReadDeviceIdentification(t *testing.T) {
	// Create a mock transport and protocol
	mockTransport := test.NewMockTransport()
	mockProtocol := protocol.NewProtocolHandler()

	// Create a client with the mock transport and protocol
	client := NewBaseClient(
		mockTransport,
		WithProtocol(mockProtocol),
	)

	// Create a mock device identification response
	mockResponse := test.NewMockDeviceIdentificationResponse(common.ReadDeviceIDBasic)

	// Set up the mock transport to return the mock response
	mockTransport.QueueResponse(mockResponse)

	// Connect the client
	ctx := context.Background()
	err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Call the method under test
	deviceID, err := client.ReadDeviceIdentification(ctx, common.ReadDeviceIDBasic, common.DeviceIDObjectCode(0))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check that the result is as expected
	if deviceID.ReadDeviceIDCode != common.ReadDeviceIDBasic {
		t.Errorf("Expected read device ID code %d, got %d", common.ReadDeviceIDBasic, deviceID.ReadDeviceIDCode)
	}

	// Check basic objects
	if deviceID.GetVendorName() != "Acme Inc." {
		t.Errorf("Expected vendor name 'Acme Inc.', got '%s'", deviceID.GetVendorName())
	}
	if deviceID.GetProductCode() != "ABC123" {
		t.Errorf("Expected product code 'ABC123', got '%s'", deviceID.GetProductCode())
	}
	if deviceID.GetRevision() != "V1.0" {
		t.Errorf("Expected revision 'V1.0', got '%s'", deviceID.GetRevision())
	}

	// Test error case: transport error
	mockTransport.QueueError(common.ErrTimeout)
	_, err = client.ReadDeviceIdentification(ctx, common.ReadDeviceIDBasic, common.DeviceIDObjectCode(0))
	if err == nil {
		t.Error("Expected error when transport fails, got nil")
	}

	// Test error case: protocol error in request generation
	mockTransport.Clear()
	_, err = client.ReadDeviceIdentification(ctx, common.ReadDeviceIDCode(0xFF), common.DeviceIDObjectCode(0)) // Invalid code
	if err == nil {
		t.Error("Expected error with invalid code, got nil")
	}
}