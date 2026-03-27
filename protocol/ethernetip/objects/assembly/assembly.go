// Package assembly implements the CIP Assembly Object (Class 0x04) for
// managing I/O data buffers in EtherNet/IP devices.
//
// In the Common Industrial Protocol (CIP), an Assembly Object groups
// collections of attributes from other objects into a single, contiguous
// block of data. Each assembly instance maps to a set of physical I/O
// points or logical data. Scanners read and write these instances to
// exchange process data with adapters during implicit (cyclic) I/O
// messaging.
//
// A typical device exposes at least three assembly instances:
//   - An Input assembly (data produced by the device, e.g. sensor readings)
//   - An Output assembly (data consumed by the device, e.g. actuator commands)
//   - A Configuration assembly (static parameters sent once at connection time)
package assembly

import (
	"encoding/binary"
	"sync"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

// AssemblyObject implements the CIP Assembly Object (Class 0x04).
// It holds a set of assembly instances, each backed by a contiguous byte
// buffer, and supports the Get_Attribute_Single and Set_Attribute_Single
// services required for implicit I/O messaging. All methods are safe for
// concurrent use.
type AssemblyObject struct {
	mu        sync.RWMutex
	instances map[uint32]*AssemblyInstance
}

// AssemblyInstance represents a single assembly instance such as an Input,
// Output, or Configuration assembly. ID is the CIP instance number and Data
// is the raw I/O buffer whose size is fixed at registration time.
type AssemblyInstance struct {
	ID   uint32
	Data []byte
}

// NewAssemblyObject creates a new, empty AssemblyObject with no registered
// instances. Use RegisterAssembly to add instances before starting I/O.
func NewAssemblyObject() *AssemblyObject {
	return &AssemblyObject{
		instances: make(map[uint32]*AssemblyInstance),
	}
}

// GetInstance returns the AssemblyInstance for the given ID, or nil if not found.
func (ao *AssemblyObject) GetInstance(instanceID uint32) *AssemblyInstance {
	ao.mu.RLock()
	defer ao.mu.RUnlock()
	return ao.instances[instanceID]
}

// RegisterAssembly registers a new assembly instance with the given ID and
// initial data buffer. The length of data defines the fixed size of the
// instance; subsequent Set_Attribute_Single calls must supply exactly this
// many bytes.
func (ao *AssemblyObject) RegisterAssembly(instanceID uint32, data []byte) {
	ao.mu.Lock()
	defer ao.mu.Unlock()
	ao.instances[instanceID] = &AssemblyInstance{
		ID:   instanceID,
		Data: data,
	}
}

// GetAttributeSingle handles the CIP Get_Attribute_Single service (0x0E).
// Attribute 3 (Data) returns a copy of the instance buffer. All other
// attribute IDs return a cip.Error with StatusAttributeNotSupported.
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

// SetAttributeSingle handles the CIP Set_Attribute_Single service (0x10).
// Attribute 3 (Data) copies the provided bytes into the instance buffer.
// The length of data must match the registered buffer size exactly;
// otherwise a cip.Error with StatusInvalidAttributeValue is returned.
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

// HandleRequest implements the cip.Object interface. It decodes the CIP path
// to extract the instance and attribute IDs, then dispatches to
// GetAttributeSingle or SetAttributeSingle. The path is expected to contain
// an instance segment (0x24 or 0x25) and optionally an attribute segment
// (0x30 or 0x31); the class segment should already have been consumed by the
// router.
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
