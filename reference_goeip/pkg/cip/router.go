package cip

import (
	"encoding/binary"
	"sync"
)

// Object is an interface that CIP objects must implement to handle requests
type Object interface {
	// HandleRequest dispatches a service request to the object
	// It returns the response data or an error
	HandleRequest(service USINT, path Path, data []byte) ([]byte, error)
}

// MessageRouter implements the Message Router Object (Class 0x02)
type MessageRouter struct {
	mu      sync.RWMutex
	objects map[UINT]Object // Map of Class ID -> Object
}

// NewMessageRouter creates a new Message Router
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		objects: make(map[UINT]Object),
	}
}

// RegisterObject registers a CIP object with the router
func (mr *MessageRouter) RegisterObject(classID UINT, obj Object) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.objects[classID] = obj
}

// Dispatch routes a message to the appropriate object
func (mr *MessageRouter) Dispatch(req *MessageRouterRequest) (*MessageRouterResponse, error) {
	// Parse Path to find destination Class/Instance
	// The path in the request is the "Request Path".
	// It usually starts with Class ID.

	// We need a path parser.
	// Let's assume req.RequestPath is already parsed into segments or we can iterate.
	// But req.RequestPath is just []byte (Path type).

	// We need to decode the path.
	// For now, let's just peek at the first segment.
	// If it's a Class segment, we look up the object.

	// This is a simplified router.
	// Real router needs to handle complex paths.

	// Let's implement a simple decoder for the first segment.
	// Or use existing path tools if any.

	// Since we don't have a full path parser exposed easily, let's do a basic check.
	// Class 8-bit: 0x20 [ClassID]
	// Class 16-bit: 0x21 [00] [ClassID_Low] [ClassID_High]

	pathBytes := req.RequestPath.Bytes()
	if len(pathBytes) == 0 {
		return nil, Error{Status: StatusPathSegmentError}
	}

	var classID UINT
	var remainingPath Path

	segType := pathBytes[0]
	if segType == 0x20 {
		if len(pathBytes) < 2 {
			return nil, Error{Status: StatusPathSegmentError}
		}
		classID = UINT(pathBytes[1])
		remainingPath = Path(pathBytes[2:])
	} else if segType == 0x21 {
		if len(pathBytes) < 4 {
			return nil, Error{Status: StatusPathSegmentError}
		}
		// Padding byte at index 1 is ignored? Spec says "Logical Segment - Class ID"
		// 0x21 is 16-bit Class ID.
		// Format: 0x21 [pad] [ID_LO] [ID_HI]
		classID = UINT(binary.LittleEndian.Uint16(pathBytes[2:4]))
		remainingPath = Path(pathBytes[4:])
	} else {
		// Maybe Instance segment if we are already at the object?
		// But Message Router usually receives full path from root.
		return nil, Error{Status: StatusPathSegmentError}
	}

	mr.mu.RLock()
	obj, ok := mr.objects[classID]
	mr.mu.RUnlock()

	if !ok {
		return &MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: StatusObjectDoesNotExist, // Or ServiceNotSupported if class unknown?
		}, nil
	}

	// Dispatch to Object
	// We pass the remaining path (Instance, Attribute, etc.)
	respData, err := obj.HandleRequest(req.Service, remainingPath, req.RequestData)
	if err != nil {
		if cipErr, ok := err.(Error); ok {
			return &MessageRouterResponse{
				Service:       req.Service | 0x80,
				GeneralStatus: cipErr.Status,
				ExtStatus:     cipErr.ExtStatus,
				ExtStatusSize: USINT(len(cipErr.ExtStatus)),
			}, nil
		}
		// Generic error
		return &MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: StatusServiceNotSupported, // Fallback
		}, nil
	}

	return &MessageRouterResponse{
		Service:       req.Service | 0x80,
		GeneralStatus: StatusSuccess,
		ResponseData:  respData,
	}, nil
}
