package cip

// NewGetAttributeSingleRequest creates a CIP Get_Attribute_Single (0x0E)
// request targeting the object addressed by path.
func NewGetAttributeSingleRequest(path Path) *MessageRouterRequest {
	return &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: path,
		RequestData: nil,
	}
}

// NewSetAttributeSingleRequest creates a CIP Set_Attribute_Single (0x10)
// request targeting the object addressed by path with the given data payload.
func NewSetAttributeSingleRequest(path Path, data []byte) *MessageRouterRequest {
	return &MessageRouterRequest{
		Service:     ServiceSetAttributeSingle,
		RequestPath: path,
		RequestData: data,
	}
}

// Rockwell Logix vendor-specific service codes for tag access. These are used
// instead of the generic CIP Get/Set_Attribute services when communicating with
// Logix controllers.
const ServiceReadTag USINT = 0x4C             // Read Tag
const ServiceWriteTag USINT = 0x4D            // Write Tag
const ServiceReadTagFragmented USINT = 0x52   // Read Tag Fragmented
const ServiceWriteTagFragmented USINT = 0x53  // Write Tag Fragmented

// ServiceExecutePCCC is the Allen-Bradley Execute_PCCC service code used to
// tunnel a PCCC command inside a CIP message router request addressed to
// the [ClassPCCC] object.
const ServiceExecutePCCC USINT = 0x4B

// NewExecutePCCCRequest builds an Execute_PCCC (0x4B) request targeting the
// PCCC Object (class 0x67, instance 1). The request data is the
// requestor-ID header followed by the PCCC command. The header format is:
//
//	Length:1 (always 7) Vendor:UINT Serial:UDINT
//
// pcccCmd is the raw PCCC command bytes (built with the pccc package).
func NewExecutePCCCRequest(vendorID UINT, serialNumber UDINT, pcccCmd []byte) *MessageRouterRequest {
	reqData := make([]byte, 7+len(pcccCmd))
	reqData[0] = 0x07 // Requestor ID length, including this byte
	reqData[1] = byte(vendorID)
	reqData[2] = byte(vendorID >> 8)
	reqData[3] = byte(serialNumber)
	reqData[4] = byte(serialNumber >> 8)
	reqData[5] = byte(serialNumber >> 16)
	reqData[6] = byte(serialNumber >> 24)
	copy(reqData[7:], pcccCmd)
	return &MessageRouterRequest{
		Service:     ServiceExecutePCCC,
		RequestPath: BuildPath(ClassPCCC, 1, 0),
		RequestData: reqData,
	}
}

// NewReadTagRequest creates a Rockwell Logix Read Tag (0x4C) request. tagPath
// should contain a symbolic segment addressing the tag by name, and elements
// specifies how many array elements to read (use 1 for scalar tags).
func NewReadTagRequest(tagPath Path, elements uint16) *MessageRouterRequest {
	reqData := make([]byte, 2)
	reqData[0] = byte(elements)
	reqData[1] = byte(elements >> 8)

	return &MessageRouterRequest{
		Service:     ServiceReadTag,
		RequestPath: tagPath,
		RequestData: reqData,
	}
}

// NewWriteTagRequest creates a Rockwell Logix Write Tag (0x4D) request.
// tagPath should contain a symbolic segment, dataType is the CIP type code,
// elements is the number of array elements, and data is the raw payload bytes.
func NewWriteTagRequest(tagPath Path, dataType DataType, elements uint16, data []byte) *MessageRouterRequest {
	// Write Tag Request Data:
	// Data Type (UINT)
	// Number of Elements (UINT)
	// Data (...)

	reqData := make([]byte, 4+len(data))
	// Type
	reqData[0] = byte(dataType)
	reqData[1] = byte(dataType >> 8)
	// Elements
	reqData[2] = byte(elements)
	reqData[3] = byte(elements >> 8)
	// Data
	copy(reqData[4:], data)

	return &MessageRouterRequest{
		Service:     ServiceWriteTag,
		RequestPath: tagPath,
		RequestData: reqData,
	}
}
