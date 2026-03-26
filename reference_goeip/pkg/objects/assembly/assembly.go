package assembly

import (
	"encoding/binary"
	"sync"

	"github.com/iceisfun/goeip/pkg/cip"
)

// AssemblyObject implements the CIP Assembly Object (Class 0x04)
type AssemblyObject struct {
	mu        sync.RWMutex
	instances map[uint32]*AssemblyInstance
}

// AssemblyInstance represents a single assembly instance (Input, Output, or Config)
type AssemblyInstance struct {
	ID   uint32
	Data []byte
}

// NewAssemblyObject creates a new Assembly Object
func NewAssemblyObject() *AssemblyObject {
	return &AssemblyObject{
		instances: make(map[uint32]*AssemblyInstance),
	}
}

// RegisterAssembly registers a new assembly instance
func (ao *AssemblyObject) RegisterAssembly(instanceID uint32, data []byte) {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	ao.instances[instanceID] = &AssemblyInstance{
		ID:   instanceID,
		Data: data,
	}
}

// GetAttributeSingle handles Get_Attribute_Single (0x0E) service
func (ao *AssemblyObject) GetAttributeSingle(instanceID uint32, attrID uint16) ([]byte, error) {
	ao.mu.RLock()
	defer ao.mu.RUnlock()

	instance, ok := ao.instances[instanceID]
	if !ok {
		return nil, cip.Error{Status: cip.StatusObjectDoesNotExist}
	}

	if attrID == 3 { // Data
		// Return a copy of the data
		dataCopy := make([]byte, len(instance.Data))
		copy(dataCopy, instance.Data)
		return dataCopy, nil
	} else if attrID == 4 { // Size (Optional but useful)
		// Return size as UINT? Or UDINT? Spec says UINT usually.
		// Let's stick to Data (3) for now as it's the main one.
		return nil, cip.Error{Status: cip.StatusAttributeNotSupported}
	}

	return nil, cip.Error{Status: cip.StatusAttributeNotSupported}
}

// SetAttributeSingle handles Set_Attribute_Single (0x10) service
func (ao *AssemblyObject) SetAttributeSingle(instanceID uint32, attrID uint16, data []byte) error {
	ao.mu.Lock()
	defer ao.mu.Unlock()

	instance, ok := ao.instances[instanceID]
	if !ok {
		return cip.Error{Status: cip.StatusObjectDoesNotExist}
	}

	if attrID == 3 { // Data
		if len(data) != len(instance.Data) {
			// Strict size check? Or allow partial?
			// Usually Assembly size is fixed.
			// Let's enforce size match for now.
			return cip.Error{Status: cip.StatusInvalidAttributeValue} // Or StatusNotEnoughData / TooMuchData
		}
		copy(instance.Data, data)
		return nil
	}

	return cip.Error{Status: cip.StatusAttributeNotSupported}
}

// HandleRequest implements the cip.Object interface
func (ao *AssemblyObject) HandleRequest(service cip.USINT, path cip.Path, data []byte) ([]byte, error) {
	// Path should contain Instance ID
	// Path format: [Instance Segment] [Attribute Segment?]
	// We need to decode the path to get Instance ID.

	// Simple path decoder for Instance (0x24 or 0x25)
	// 0x24: 8-bit Instance
	// 0x25: 16-bit Instance

	pathBytes := path.Bytes()
	if len(pathBytes) == 0 {
		return nil, cip.Error{Status: cip.StatusPathSegmentError}
	}

	var instanceID uint32
	var remainingPath []byte

	segType := pathBytes[0]
	if segType == 0x24 {
		if len(pathBytes) < 2 {
			return nil, cip.Error{Status: cip.StatusPathSegmentError}
		}
		instanceID = uint32(pathBytes[1])
		remainingPath = pathBytes[2:]
	} else if segType == 0x25 {
		if len(pathBytes) < 4 {
			return nil, cip.Error{Status: cip.StatusPathSegmentError}
		}
		instanceID = uint32(binary.LittleEndian.Uint16(pathBytes[2:4]))
		remainingPath = pathBytes[4:]
	} else {
		// Maybe it's Class level request?
		// If path is empty or different, handle class services?
		return nil, cip.Error{Status: cip.StatusPathSegmentError}
	}

	// Check for Attribute segment if needed?
	// Services like GetAttributeSingle usually don't have Attribute in path if it's in the service params?
	// No, GetAttributeSingle (0x0E) usually expects Attribute ID in the path?
	// Spec says: Request Path: Class, Instance, Attribute.
	// But our Router stripped Class. So we have Instance, Attribute.

	// Let's parse Attribute ID if present.
	var attrID uint16
	// Default to 0 if not present?

	if len(remainingPath) > 0 {
		segType = remainingPath[0]
		if segType == 0x30 { // 8-bit Attribute
			if len(remainingPath) < 2 {
				return nil, cip.Error{Status: cip.StatusPathSegmentError}
			}
			attrID = uint16(remainingPath[1])
		} else if segType == 0x31 { // 16-bit Attribute
			if len(remainingPath) < 4 {
				return nil, cip.Error{Status: cip.StatusPathSegmentError}
			}
			attrID = binary.LittleEndian.Uint16(remainingPath[2:4])
		}
	}

	switch service {
	case cip.ServiceGetAttributeSingle:
		if attrID == 0 {
			return nil, cip.Error{Status: cip.StatusPathSegmentError} // Attribute required
		}
		return ao.GetAttributeSingle(instanceID, attrID)
	case cip.ServiceSetAttributeSingle:
		if attrID == 0 {
			return nil, cip.Error{Status: cip.StatusPathSegmentError}
		}
		return nil, ao.SetAttributeSingle(instanceID, attrID, data)
	default:
		return nil, cip.Error{Status: cip.StatusServiceNotSupported}
	}
}
