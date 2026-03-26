package test

import (
	"github.com/Moonlight-Companies/gomodbus/common"
)

// NewMockDeviceIdentificationResponse creates a mock response for a read device identification request
func NewMockDeviceIdentificationResponse(readDeviceIDCode common.ReadDeviceIDCode) *MockResponse {
	// Create a mock device identification response with some sample data
	responseData := []byte{
		byte(common.MEIReadDeviceID),     // MEI type
		byte(readDeviceIDCode),           // Read device ID code
		0x01,                       // Conformity level
		0x00,                       // More follows
		0x00,                       // Next object ID
		0x03,                       // Number of objects
		// Object 1: Vendor name
		byte(common.DeviceIDVendorName),  // ID
		0x09,                       // Length
		'A', 'c', 'm', 'e', ' ', 'I', 'n', 'c', '.', // Value
		// Object 2: Product code
		byte(common.DeviceIDProductCode), // ID
		0x06,                       // Length
		'A', 'B', 'C', '1', '2', '3', // Value
		// Object 3: Revision
		byte(common.DeviceIDMajorMinorRevision), // ID
		0x04,                              // Length
		'V', '1', '.', '0',                // Value
	}

	return NewMockResponse(1, 1, common.FuncReadDeviceIdentification, responseData)
}