package cip

import (
	"bytes"
	"encoding/binary"
)

// MessageRouterRequest represents a CIP Message Router request. It carries a
// service code, the EPATH to the target object, and optional request data.
// The Message Router (class 0x02) dispatches the request to the addressed
// object.
type MessageRouterRequest struct {
	Service     USINT
	RequestPath Path
	RequestData []byte
}

// Encode serializes the request into the CIP wire format:
// [Service:1][PathSizeWords:1][Path...][RequestData...].
func (r *MessageRouterRequest) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, r.Service); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.RequestPath.LenWords()); err != nil {
		return nil, err
	}
	if _, err := buf.Write(r.RequestPath.Bytes()); err != nil {
		return nil, err
	}
	if len(r.RequestData) > 0 {
		if _, err := buf.Write(r.RequestData); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeMessageRouterRequest decodes a byte slice into a MessageRouterRequest.
// The expected wire format is [Service:1][PathSizeWords:1][Path...][RequestData...].
func DecodeMessageRouterRequest(data []byte) (*MessageRouterRequest, error) {
	r := &MessageRouterRequest{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &r.Service); err != nil {
		return nil, err
	}
	var pathSizeWords uint8
	if err := binary.Read(buf, binary.LittleEndian, &pathSizeWords); err != nil {
		return nil, err
	}
	pathBytes := make([]byte, int(pathSizeWords)*2)
	if _, err := buf.Read(pathBytes); err != nil {
		return nil, err
	}
	r.RequestPath = Path(pathBytes)

	remaining := buf.Len()
	if remaining > 0 {
		r.RequestData = make([]byte, remaining)
		if _, err := buf.Read(r.RequestData); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// MessageRouterResponse represents a CIP Message Router response. The Service
// field echoes the request service code with bit 7 set (OR 0x80).
// GeneralStatus indicates success (0x00) or an error code, and ExtStatus
// carries optional additional detail.
type MessageRouterResponse struct {
	Service       USINT // Reply Service (Request Service | 0x80)
	Reserved      USINT
	GeneralStatus USINT
	ExtStatusSize USINT
	ExtStatus     []UINT
	ResponseData  []byte
}

// Encode serializes the response into the CIP wire format.
func (r *MessageRouterResponse) Encode() ([]byte, error) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, r.Service)
	binary.Write(buf, binary.LittleEndian, r.Reserved)
	binary.Write(buf, binary.LittleEndian, r.GeneralStatus)
	binary.Write(buf, binary.LittleEndian, r.ExtStatusSize)
	for _, ext := range r.ExtStatus {
		binary.Write(buf, binary.LittleEndian, ext)
	}
	buf.Write(r.ResponseData)
	return buf.Bytes(), nil
}

// DecodeMessageRouterResponse decodes a byte slice into a MessageRouterResponse.
func DecodeMessageRouterResponse(data []byte) (*MessageRouterResponse, error) {
	r := &MessageRouterResponse{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &r.Service); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &r.Reserved); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &r.GeneralStatus); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &r.ExtStatusSize); err != nil {
		return nil, err
	}

	if r.ExtStatusSize > 0 {
		r.ExtStatus = make([]UINT, r.ExtStatusSize)
		for i := 0; i < int(r.ExtStatusSize); i++ {
			if err := binary.Read(buf, binary.LittleEndian, &r.ExtStatus[i]); err != nil {
				return nil, err
			}
		}
	}

	// The rest is response data
	remaining := buf.Len()
	if remaining > 0 {
		r.ResponseData = make([]byte, remaining)
		if _, err := buf.Read(r.ResponseData); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// IsSuccess returns true if the GeneralStatus is [StatusSuccess] (0x00).
func (r *MessageRouterResponse) IsSuccess() bool {
	return r.GeneralStatus == StatusSuccess
}

// Error returns a structured [Error] if the response indicates failure, or nil
// on success.
func (r *MessageRouterResponse) Error() error {
	if r.IsSuccess() {
		return nil
	}
	return Error{
		Status:    r.GeneralStatus,
		ExtStatus: r.ExtStatus,
	}
}
