package cip

import (
	"encoding/binary"
	"sync"
)

// Object is the interface that CIP objects must implement to receive service
// requests dispatched by the [MessageRouter].
type Object interface {
	// HandleRequest processes a CIP service request addressed to this object.
	// path contains the remaining EPATH segments after the class segment
	// (typically instance and attribute). It returns the response data or an
	// error.
	HandleRequest(service USINT, path Path, data []byte) ([]byte, error)
}

// MessageRouter implements the CIP Message Router Object (class 0x02). It
// maintains a registry of [Object] implementations keyed by class ID and
// dispatches incoming requests to the appropriate object.
type MessageRouter struct {
	mu              sync.RWMutex
	objects         map[UINT]Object // Map of Class ID -> Object
	symbolicHandler Object          // Handler for symbolic segment (0x91) requests
}

// NewMessageRouter creates a new empty MessageRouter with no registered objects.
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		objects: make(map[UINT]Object),
	}
}

// RegisterObject registers a CIP [Object] implementation with the router under
// the given class ID.
func (mr *MessageRouter) RegisterObject(classID UINT, obj Object) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.objects[classID] = obj
}

// RegisterSymbolicHandler registers an [Object] to handle requests whose path
// starts with an ANSI Extended Symbol segment (0x91). This is used for
// Logix-style tag access where the request path contains a symbolic tag name
// rather than a class/instance path.
func (mr *MessageRouter) RegisterSymbolicHandler(obj Object) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.symbolicHandler = obj
}

// Dispatch routes a CIP request to the registered object that owns the
// target class ID. The class ID is extracted from the first path segment
// (supports 8-bit 0x20 and 16-bit 0x21 formats). The remaining path
// (instance, attribute, etc.) is forwarded to the object's HandleRequest.
func (mr *MessageRouter) Dispatch(req *MessageRouterRequest) (*MessageRouterResponse, error) {
	pathBytes := req.RequestPath.Bytes()
	if len(pathBytes) == 0 {
		return nil, Error{Status: StatusPathSegmentError}
	}

	var classID UINT
	var remainingPath Path

	switch pathBytes[0] {
	case 0x20: // 8-bit class ID
		if len(pathBytes) < 2 {
			return nil, Error{Status: StatusPathSegmentError}
		}
		classID = UINT(pathBytes[1])
		remainingPath = Path(pathBytes[2:])
	case 0x21: // 16-bit class ID: [0x21] [pad] [ID_LO] [ID_HI]
		if len(pathBytes) < 4 {
			return nil, Error{Status: StatusPathSegmentError}
		}
		classID = UINT(binary.LittleEndian.Uint16(pathBytes[2:4]))
		remainingPath = Path(pathBytes[4:])
	case 0x91: // ANSI Extended Symbol segment — route to symbolic handler
		mr.mu.RLock()
		handler := mr.symbolicHandler
		mr.mu.RUnlock()

		if handler == nil {
			return nil, Error{Status: StatusPathSegmentError}
		}

		respData, err := handler.HandleRequest(req.Service, req.RequestPath, req.RequestData)
		if err != nil {
			if cipErr, ok := err.(Error); ok {
				return &MessageRouterResponse{
					Service:       req.Service | 0x80,
					GeneralStatus: cipErr.Status,
					ExtStatus:     cipErr.ExtStatus,
					ExtStatusSize: USINT(len(cipErr.ExtStatus)),
				}, nil
			}
			return &MessageRouterResponse{
				Service:       req.Service | 0x80,
				GeneralStatus: StatusServiceNotSupported,
			}, nil
		}

		return &MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: StatusSuccess,
			ResponseData:  respData,
		}, nil
	default:
		return nil, Error{Status: StatusPathSegmentError}
	}

	mr.mu.RLock()
	obj, ok := mr.objects[classID]
	mr.mu.RUnlock()

	if !ok {
		return &MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: StatusObjectDoesNotExist,
		}, nil
	}

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
		return &MessageRouterResponse{
			Service:       req.Service | 0x80,
			GeneralStatus: StatusServiceNotSupported,
		}, nil
	}

	return &MessageRouterResponse{
		Service:       req.Service | 0x80,
		GeneralStatus: StatusSuccess,
		ResponseData:  respData,
	}, nil
}
