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
