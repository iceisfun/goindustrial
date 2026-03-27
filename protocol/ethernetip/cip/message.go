package cip

import (
	"bytes"
	"encoding/binary"
)

// MessageRouterRequest represents a request to the Message Router Object
type MessageRouterRequest struct {
	Service     USINT
	RequestPath Path
	RequestData []byte
}

// Encode encodes the request into a byte slice
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
// This is the inverse of Encode: [Service:1][PathSizeWords:1][Path...][RequestData...].
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

// MessageRouterResponse represents a response from the Message Router Object
type MessageRouterResponse struct {
	Service       USINT // Reply Service (Request Service | 0x80)
	Reserved      USINT
	GeneralStatus USINT
	ExtStatusSize USINT
	ExtStatus     []UINT
	ResponseData  []byte
}

// Encode encodes the response into a byte slice.
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

// DecodeMessageRouterResponse decodes a byte slice into a MessageRouterResponse
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

// IsSuccess checks if the response indicates success
func (r *MessageRouterResponse) IsSuccess() bool {
	return r.GeneralStatus == StatusSuccess
}

// Error returns a structured error if the response failed
func (r *MessageRouterResponse) Error() error {
	if r.IsSuccess() {
		return nil
	}
	return Error{
		Status:    r.GeneralStatus,
		ExtStatus: r.ExtStatus,
	}
}
