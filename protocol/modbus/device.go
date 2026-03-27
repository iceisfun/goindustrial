package modbus

// DeviceIDObject represents a single object returned by the Read Device
// Identification function (FC 0x2B / MEI 0x0E). Each object has a standard
// or vendor-specific ID code (e.g. vendor name, product code) and a string
// value.
type DeviceIDObject struct {
	ID     DeviceIDObjectCode // Object ID - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21, Table 72
	Length byte               // Length of the object value in bytes - Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 6.21 (Response PDU object format)
	Value  string             // Object value
}

// DeviceIdentification represents a complete response from the Read Device
// Identification function (FC 0x2B / MEI 0x0E). It contains metadata about
// the response pagination and the list of identification objects returned.
type DeviceIdentification struct {
	ReadDeviceIDCode ReadDeviceIDCode   // Echoes request - Ref: Section 6.21, Response PDU
	ConformityLevel  ConformityLevel    // Conformity level of the device - Ref: Section 6.21, Table 74
	MoreFollows      MoreFollows        // 0x00 = No, 0xFF = Yes - Ref: Section 6.21, Response PDU
	NextObjectID     DeviceIDObjectCode // Object ID to request next if MoreFollows is true - Ref: Section 6.21, Response PDU
	NumberOfObjects  byte               // Number of identification objects in this response part - Ref: Section 6.21, Response PDU
	Objects          []DeviceIDObject   // The list of device identification objects
}

// GetObject returns the object with the specified ID, or nil if not found.
func (d *DeviceIdentification) GetObject(id DeviceIDObjectCode) *DeviceIDObject {
	for i := range d.Objects {
		if d.Objects[i].ID == id {
			return &d.Objects[i]
		}
	}
	return nil
}

// GetVendorName returns the vendor name (object ID 0x00), or an empty string
// if not present. This is a mandatory basic identification object.
func (d *DeviceIdentification) GetVendorName() string {
	obj := d.GetObject(DeviceIDVendorName)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetProductCode returns the product code (object ID 0x01), or an empty string
// if not present. This is a mandatory basic identification object.
func (d *DeviceIdentification) GetProductCode() string {
	obj := d.GetObject(DeviceIDProductCode)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetRevision returns the major/minor revision (object ID 0x02), or an empty
// string if not present. This is a mandatory basic identification object.
func (d *DeviceIdentification) GetRevision() string {
	obj := d.GetObject(DeviceIDMajorMinorRevision)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetVendorURL returns the vendor URL (object ID 0x03), or an empty string if
// not present. This is an optional regular identification object.
func (d *DeviceIdentification) GetVendorURL() string {
	obj := d.GetObject(DeviceIDVendorURL)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetProductName returns the product name (object ID 0x04), or an empty string
// if not present. This is an optional regular identification object.
func (d *DeviceIdentification) GetProductName() string {
	obj := d.GetObject(DeviceIDProductName)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetModelName returns the model name (object ID 0x05), or an empty string if
// not present. This is an optional regular identification object.
func (d *DeviceIdentification) GetModelName() string {
	obj := d.GetObject(DeviceIDModelName)
	if obj != nil {
		return obj.Value
	}
	return ""
}

// GetUserApplicationName returns the user application name (object ID 0x06), or
// an empty string if not present. This is an optional regular identification
// object.
func (d *DeviceIdentification) GetUserApplicationName() string {
	obj := d.GetObject(DeviceIDUserAppName)
	if obj != nil {
		return obj.Value
	}
	return ""
}
