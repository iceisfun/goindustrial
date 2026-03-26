package protocol

import (
	"bytes"
	"testing"

	"github.com/Moonlight-Companies/gomodbus/common"
)

func TestGenerateReadDeviceIdentificationRequest(t *testing.T) {
	handler := NewProtocolHandler()

	// Print the actual constant values for debugging
	t.Logf("MEIReadDeviceID = 0x%02X", common.MEIReadDeviceID)
	t.Logf("ReadDeviceIDBasic = 0x%02X", common.ReadDeviceIDBasic)
	t.Logf("ReadDeviceIDSpecific = 0x%02X", common.ReadDeviceIDSpecific)
	t.Logf("DeviceIDVendorName = 0x%02X", common.DeviceIDVendorName)

	// Test with valid parameters for basic identification
	data, err := handler.GenerateReadDeviceIdentificationRequest(common.ReadDeviceIDBasic, common.DeviceIDObjectCode(0))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := []byte{byte(common.MEIReadDeviceID), byte(common.ReadDeviceIDBasic), 0}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}

	// Test with valid parameters for specific object
	data, err = handler.GenerateReadDeviceIdentificationRequest(common.ReadDeviceIDSpecific, common.DeviceIDVendorName)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected = []byte{byte(common.MEIReadDeviceID), byte(common.ReadDeviceIDSpecific), byte(common.DeviceIDVendorName)}
	if !bytes.Equal(data, expected) {
		t.Errorf("Expected %v, got %v", expected, data)
	}

	// Test with invalid parameters: invalid code
	_, err = handler.GenerateReadDeviceIdentificationRequest(common.ReadDeviceIDCode(0), common.DeviceIDObjectCode(0))
	if err == nil {
		t.Error("Expected error for invalid read device ID code, got nil")
	}

	// Since we removed these validations, these tests are no longer needed
	// // Test with invalid parameters: specific access without object ID
	// _, err = handler.GenerateReadDeviceIdentificationRequest(common.ReadDeviceIDSpecific, 0)
	// if err == nil {
	// 	t.Error("Expected error for specific access without object ID, got nil")
	// }
	//
	// // Test with invalid parameters: stream access with object ID
	// _, err = handler.GenerateReadDeviceIdentificationRequest(common.ReadDeviceIDBasic, common.DeviceIDVendorName)
	// if err == nil {
	// 	t.Error("Expected error for stream access with object ID, got nil")
	// }
}

func TestParseReadDeviceIdentificationResponse(t *testing.T) {
	handler := NewProtocolHandler()

	// Test with valid response data for basic identification
	responseData := []byte{
		byte(common.MEIReadDeviceID),    // MEI type
		byte(common.ReadDeviceIDBasic),  // Read device ID code
		0x01,                            // Conformity level
		0x00,                            // More follows
		0x00,                            // Next object ID
		0x03,                            // Number of objects
		// Object 1: Vendor name
		byte(common.DeviceIDVendorName), // ID
		0x0B,                            // Length
		'A', 'c', 'm', 'e', ' ', 'D', 'e', 'v', 'i', 'c', 'e', // Value
		// Object 2: Product code
		byte(common.DeviceIDProductCode), // ID
		0x05,                             // Length
		'A', 'B', 'C', '1', '2',          // Value
		// Object 3: Revision
		byte(common.DeviceIDMajorMinorRevision), // ID
		0x05,                                    // Length
		'V', '1', '.', '0', '0',                 // Value
	}

	deviceID, err := handler.ParseReadDeviceIdentificationResponse(responseData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if deviceID.ReadDeviceIDCode != common.ReadDeviceIDBasic {
		t.Errorf("Expected read device ID code %d, got %d", common.ReadDeviceIDBasic, deviceID.ReadDeviceIDCode)
	}

	if deviceID.ConformityLevel != common.ConformityLevelBasic {
		t.Errorf("Expected conformity level %s, got %s", common.ConformityLevelBasic, deviceID.ConformityLevel)
	}

	if deviceID.MoreFollows != common.MoreFollowsNo {
		t.Errorf("Expected MoreFollows %s, got %s", common.MoreFollowsNo, deviceID.MoreFollows)
	}

	if deviceID.NumberOfObjects != 3 {
		t.Errorf("Expected 3 objects, got %d", deviceID.NumberOfObjects)
	}

	// Check object 1
	obj := deviceID.GetObject(common.DeviceIDVendorName)
	if obj == nil {
		t.Fatal("Vendor name object not found")
	}
	if obj.Value != "Acme Device" {
		t.Errorf("Expected vendor name 'Acme Device', got '%s'", obj.Value)
	}

	// Check object 2
	obj = deviceID.GetObject(common.DeviceIDProductCode)
	if obj == nil {
		t.Fatal("Product code object not found")
	}
	if obj.Value != "ABC12" {
		t.Errorf("Expected product code 'ABC12', got '%s'", obj.Value)
	}

	// Check object 3
	obj = deviceID.GetObject(common.DeviceIDMajorMinorRevision)
	if obj == nil {
		t.Fatal("Revision object not found")
	}
	if obj.Value != "V1.00" {
		t.Errorf("Expected revision 'V1.00', got '%s'", obj.Value)
	}

	// Check helper methods
	if deviceID.GetVendorName() != "Acme Device" {
		t.Errorf("Expected GetVendorName() to return 'Acme Device', got '%s'", deviceID.GetVendorName())
	}
	if deviceID.GetProductCode() != "ABC12" {
		t.Errorf("Expected GetProductCode() to return 'ABC12', got '%s'", deviceID.GetProductCode())
	}
	if deviceID.GetRevision() != "V1.00" {
		t.Errorf("Expected GetRevision() to return 'V1.00', got '%s'", deviceID.GetRevision())
	}

	// Test with invalid response data: too short
	_, err = handler.ParseReadDeviceIdentificationResponse([]byte{0x0E, 0x01})
	if err == nil {
		t.Error("Expected error for too short response, got nil")
	}

	// Test with invalid response data: wrong MEI type
	badMEIData := make([]byte, len(responseData))
	copy(badMEIData, responseData)
	badMEIData[0] = 0x0F // Wrong MEI type
	_, err = handler.ParseReadDeviceIdentificationResponse(badMEIData)
	if err == nil {
		t.Error("Expected error for wrong MEI type, got nil")
	}

	// Test with invalid response data: invalid object data
	badObjectData := make([]byte, len(responseData)-1) // Remove last byte
	copy(badObjectData, responseData)
	_, err = handler.ParseReadDeviceIdentificationResponse(badObjectData)
	if err == nil {
		t.Error("Expected error for invalid object data, got nil")
	}
}