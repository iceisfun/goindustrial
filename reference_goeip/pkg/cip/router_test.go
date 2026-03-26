package cip

import (
	"testing"
)

// mockObject implements Object for testing
type mockObject struct {
	handleFunc func(service USINT, path Path, data []byte) ([]byte, error)
}

func (m *mockObject) HandleRequest(service USINT, path Path, data []byte) ([]byte, error) {
	if m.handleFunc != nil {
		return m.handleFunc(service, path, data)
	}
	return []byte{0x01, 0x02, 0x03, 0x04}, nil
}

func TestNewMessageRouter(t *testing.T) {
	mr := NewMessageRouter()

	if mr == nil {
		t.Fatal("NewMessageRouter returned nil")
	}
	if mr.objects == nil {
		t.Error("objects map not initialized")
	}
}

func TestMessageRouter_RegisterObject(t *testing.T) {
	mr := NewMessageRouter()
	obj := &mockObject{}

	mr.RegisterObject(ClassAssembly, obj)

	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if _, ok := mr.objects[ClassAssembly]; !ok {
		t.Error("Object not registered")
	}
}

func TestMessageRouter_RegisterObject_MultipleClasses(t *testing.T) {
	mr := NewMessageRouter()
	obj1 := &mockObject{}
	obj2 := &mockObject{}

	mr.RegisterObject(ClassAssembly, obj1)
	mr.RegisterObject(ClassIdentity, obj2)

	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if len(mr.objects) != 2 {
		t.Errorf("Expected 2 objects, got %d", len(mr.objects))
	}
}

func TestMessageRouter_Dispatch_8BitClassID(t *testing.T) {
	mr := NewMessageRouter()

	// Track if HandleRequest was called
	called := false
	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			called = true
			return []byte{0xDE, 0xAD}, nil
		},
	}
	mr.RegisterObject(0x04, obj) // Class 4 (Assembly)

	// Build request with 8-bit class ID
	// Path: 0x20 [ClassID] 0x24 [InstanceID] 0x30 [AttrID]
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}), // Class 4, Instance 1, Attr 3
		RequestData: nil,
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if !called {
		t.Error("Object's HandleRequest was not called")
	}

	// Check response service (should be request service | 0x80)
	if resp.Service != (ServiceGetAttributeSingle | 0x80) {
		t.Errorf("Response Service = 0x%02X, want 0x%02X", resp.Service, ServiceGetAttributeSingle|0x80)
	}

	if resp.GeneralStatus != StatusSuccess {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusSuccess)
	}
}

func TestMessageRouter_Dispatch_16BitClassID(t *testing.T) {
	mr := NewMessageRouter()

	called := false
	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			called = true
			return []byte{0xBE, 0xEF}, nil
		},
	}
	mr.RegisterObject(0x0100, obj) // Class 0x100

	// Build request with 16-bit class ID
	// Path: 0x21 [pad] [ClassID_LO] [ClassID_HI] ...
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x21, 0x00, 0x00, 0x01, 0x24, 0x01}), // Class 0x0100, Instance 1
		RequestData: nil,
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if !called {
		t.Error("Object's HandleRequest was not called")
	}

	if resp.GeneralStatus != StatusSuccess {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusSuccess)
	}
}

func TestMessageRouter_Dispatch_UnknownClass(t *testing.T) {
	mr := NewMessageRouter()
	// Don't register any object

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0xFF, 0x24, 0x01}), // Class 0xFF (not registered)
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch should not return error: %v", err)
	}

	// Should return response with StatusObjectDoesNotExist
	if resp.GeneralStatus != StatusObjectDoesNotExist {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusObjectDoesNotExist)
	}
}

func TestMessageRouter_Dispatch_EmptyPath(t *testing.T) {
	mr := NewMessageRouter()

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{}), // Empty path
	}

	_, err := mr.Dispatch(req)
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

func TestMessageRouter_Dispatch_InvalidPathSegment(t *testing.T) {
	mr := NewMessageRouter()

	// Path with unknown segment type (not 0x20 or 0x21)
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0xFF, 0x04}), // Invalid segment type
	}

	_, err := mr.Dispatch(req)
	if err == nil {
		t.Error("Expected error for invalid path segment")
	}
}

func TestMessageRouter_Dispatch_Short8BitPath(t *testing.T) {
	mr := NewMessageRouter()

	// 8-bit class segment but no class ID
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20}), // Missing class ID
	}

	_, err := mr.Dispatch(req)
	if err == nil {
		t.Error("Expected error for short 8-bit path")
	}
}

func TestMessageRouter_Dispatch_Short16BitPath(t *testing.T) {
	mr := NewMessageRouter()

	// 16-bit class segment but not enough bytes
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x21, 0x00, 0x04}), // Missing high byte
	}

	_, err := mr.Dispatch(req)
	if err == nil {
		t.Error("Expected error for short 16-bit path")
	}
}

func TestMessageRouter_Dispatch_ObjectError(t *testing.T) {
	mr := NewMessageRouter()

	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			return nil, Error{Status: StatusAttributeNotSupported}
		},
	}
	mr.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0xFF}), // Invalid attribute
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch should not return error: %v", err)
	}

	if resp.GeneralStatus != StatusAttributeNotSupported {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusAttributeNotSupported)
	}
}

func TestMessageRouter_Dispatch_ObjectErrorWithExtStatus(t *testing.T) {
	mr := NewMessageRouter()

	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			return nil, Error{
				Status:    StatusConnectionFailure,
				ExtStatus: []UINT{0x0100, 0x0200},
			}
		},
	}
	mr.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch should not return error: %v", err)
	}

	if resp.GeneralStatus != StatusConnectionFailure {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusConnectionFailure)
	}

	if resp.ExtStatusSize != 2 {
		t.Errorf("Response ExtStatusSize = %d, want 2", resp.ExtStatusSize)
	}

	if len(resp.ExtStatus) != 2 || resp.ExtStatus[0] != 0x0100 || resp.ExtStatus[1] != 0x0200 {
		t.Errorf("Response ExtStatus = %v, want [0x0100, 0x0200]", resp.ExtStatus)
	}
}

func TestMessageRouter_Dispatch_NonCIPError(t *testing.T) {
	mr := NewMessageRouter()

	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			return nil, Error{Status: StatusServiceNotSupported} // Generic error
		},
	}
	mr.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     0xFF, // Unsupported service
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch should not return error: %v", err)
	}

	if resp.GeneralStatus != StatusServiceNotSupported {
		t.Errorf("Response GeneralStatus = 0x%02X, want 0x%02X", resp.GeneralStatus, StatusServiceNotSupported)
	}
}

func TestMessageRouter_Dispatch_RemainingPath(t *testing.T) {
	mr := NewMessageRouter()

	var receivedPath Path
	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			receivedPath = path
			return []byte{0x01}, nil
		},
	}
	mr.RegisterObject(0x04, obj)

	// Path: Class 4, Instance 0x0102, Attribute 3
	// 0x20 0x04 (Class 4)
	// 0x24 0x01 (Instance 1)
	// 0x30 0x03 (Attribute 3)
	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}),
	}

	_, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Object should receive remaining path (Instance + Attribute)
	expectedRemaining := []byte{0x24, 0x01, 0x30, 0x03}
	if len(receivedPath) != len(expectedRemaining) {
		t.Errorf("Remaining path length = %d, want %d", len(receivedPath), len(expectedRemaining))
	}
	for i, b := range expectedRemaining {
		if receivedPath[i] != b {
			t.Errorf("Remaining path[%d] = 0x%02X, want 0x%02X", i, receivedPath[i], b)
		}
	}
}

func TestMessageRouter_Dispatch_RequestData(t *testing.T) {
	mr := NewMessageRouter()

	var receivedData []byte
	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			receivedData = data
			return []byte{0x01}, nil
		},
	}
	mr.RegisterObject(0x04, obj)

	requestData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	req := &MessageRouterRequest{
		Service:     ServiceSetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}),
		RequestData: requestData,
	}

	_, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(receivedData) != len(requestData) {
		t.Errorf("Request data length mismatch")
	}
	for i, b := range requestData {
		if receivedData[i] != b {
			t.Errorf("Request data[%d] = 0x%02X, want 0x%02X", i, receivedData[i], b)
		}
	}
}

func TestMessageRouter_Dispatch_ResponseData(t *testing.T) {
	mr := NewMessageRouter()

	responseData := []byte{0x11, 0x22, 0x33, 0x44, 0x55}
	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			return responseData, nil
		},
	}
	mr.RegisterObject(0x04, obj)

	req := &MessageRouterRequest{
		Service:     ServiceGetAttributeSingle,
		RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01, 0x30, 0x03}),
	}

	resp, err := mr.Dispatch(req)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if len(resp.ResponseData) != len(responseData) {
		t.Errorf("Response data length = %d, want %d", len(resp.ResponseData), len(responseData))
	}
	for i, b := range responseData {
		if resp.ResponseData[i] != b {
			t.Errorf("Response data[%d] = 0x%02X, want 0x%02X", i, resp.ResponseData[i], b)
		}
	}
}

func TestMessageRouter_ConcurrentDispatch(t *testing.T) {
	mr := NewMessageRouter()

	obj := &mockObject{
		handleFunc: func(service USINT, path Path, data []byte) ([]byte, error) {
			return []byte{0x01}, nil
		},
	}
	mr.RegisterObject(0x04, obj)

	// Run multiple concurrent dispatches
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			req := &MessageRouterRequest{
				Service:     ServiceGetAttributeSingle,
				RequestPath: Path([]byte{0x20, 0x04, 0x24, 0x01}),
			}
			_, err := mr.Dispatch(req)
			done <- (err == nil)
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		if !<-done {
			t.Error("Concurrent dispatch failed")
		}
	}
}
