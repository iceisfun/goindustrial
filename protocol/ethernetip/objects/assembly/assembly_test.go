package assembly

import (
	"bytes"
	"sync"
	"testing"

	"github.com/iceisfun/goindustrial/protocol/ethernetip/cip"
)

func TestRegisterAndGetInstance(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(100, make([]byte, 8))
	ao.RegisterAssembly(200, make([]byte, 16))

	inst := ao.GetInstance(100)
	if inst == nil {
		t.Fatal("GetInstance(100) returned nil")
	}
	if inst.ID != 100 {
		t.Errorf("ID = %d, want 100", inst.ID)
	}
	if len(inst.Data) != 8 {
		t.Errorf("len(Data) = %d, want 8", len(inst.Data))
	}

	inst2 := ao.GetInstance(200)
	if inst2 == nil {
		t.Fatal("GetInstance(200) returned nil")
	}
	if len(inst2.Data) != 16 {
		t.Errorf("len(Data) = %d, want 16", len(inst2.Data))
	}
}

func TestGetInstanceNonexistent(t *testing.T) {
	ao := NewAssemblyObject()
	inst := ao.GetInstance(999)
	if inst != nil {
		t.Errorf("GetInstance(999) = %v, want nil", inst)
	}
}

func TestGetSetAttributeSingle(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(10, make([]byte, 4))

	// Write data
	writeData := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := ao.SetAttributeSingle(10, 3, writeData); err != nil {
		t.Fatalf("SetAttributeSingle: %v", err)
	}

	// Read it back
	readData, err := ao.GetAttributeSingle(10, 3)
	if err != nil {
		t.Fatalf("GetAttributeSingle: %v", err)
	}
	if !bytes.Equal(readData, writeData) {
		t.Errorf("read = %X, want %X", readData, writeData)
	}

	// Verify the returned slice is a copy (not a reference to internal data).
	readData[0] = 0x00
	readData2, _ := ao.GetAttributeSingle(10, 3)
	if readData2[0] != 0xDE {
		t.Error("GetAttributeSingle returned a reference to internal data instead of a copy")
	}
}

func TestSetAttributeSingleSizeMismatch(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(10, make([]byte, 4))

	err := ao.SetAttributeSingle(10, 3, []byte{1, 2})
	if err == nil {
		t.Fatal("expected error for size mismatch")
	}
	cipErr, ok := err.(cip.Error)
	if !ok {
		t.Fatalf("expected cip.Error, got %T", err)
	}
	if cipErr.Status != cip.StatusInvalidAttributeValue {
		t.Errorf("status = 0x%02X, want 0x%02X", cipErr.Status, cip.StatusInvalidAttributeValue)
	}
}

func TestGetAttributeSingleInvalidInstance(t *testing.T) {
	ao := NewAssemblyObject()

	_, err := ao.GetAttributeSingle(999, 3)
	if err == nil {
		t.Fatal("expected error for nonexistent instance")
	}
	cipErr, ok := err.(cip.Error)
	if !ok {
		t.Fatalf("expected cip.Error, got %T", err)
	}
	if cipErr.Status != cip.StatusObjectDoesNotExist {
		t.Errorf("status = 0x%02X, want 0x%02X", cipErr.Status, cip.StatusObjectDoesNotExist)
	}
}

func TestSetAttributeSingleInvalidInstance(t *testing.T) {
	ao := NewAssemblyObject()

	err := ao.SetAttributeSingle(999, 3, []byte{1, 2, 3, 4})
	if err == nil {
		t.Fatal("expected error for nonexistent instance")
	}
}

func TestUnsupportedAttribute(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(10, make([]byte, 4))

	// Attribute 4 (Size) is not implemented.
	_, err := ao.GetAttributeSingle(10, 4)
	if err == nil {
		t.Fatal("expected error for unsupported attribute")
	}

	err = ao.SetAttributeSingle(10, 99, []byte{1, 2, 3, 4})
	if err == nil {
		t.Fatal("expected error for unsupported attribute on set")
	}
}

func TestConcurrentAccess(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(1, make([]byte, 4))

	var wg sync.WaitGroup
	const goroutines = 20
	const iterations = 100

	// Half writers, half readers.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					ao.SetAttributeSingle(1, 3, []byte{byte(j), byte(j), byte(j), byte(j)})
				}
			}()
		} else {
			go func() {
				defer wg.Done()
				for j := 0; j < iterations; j++ {
					ao.GetAttributeSingle(1, 3)
				}
			}()
		}
	}

	wg.Wait()

	// If we get here without a race detector complaint, the test passes.
	data, err := ao.GetAttributeSingle(1, 3)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if len(data) != 4 {
		t.Errorf("final data len = %d, want 4", len(data))
	}
}

func TestHandleRequestGetAttribute(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(5, []byte{0xAA, 0xBB, 0xCC, 0xDD})

	// Build path: Instance 5 (8-bit), Attribute 3 (8-bit)
	path := cip.Path([]byte{0x24, 0x05, 0x30, 0x03})

	data, err := ao.HandleRequest(cip.ServiceGetAttributeSingle, path, nil)
	if err != nil {
		t.Fatalf("HandleRequest Get: %v", err)
	}
	if !bytes.Equal(data, []byte{0xAA, 0xBB, 0xCC, 0xDD}) {
		t.Errorf("data = %X, want AABBCCDD", data)
	}
}

func TestHandleRequestSetAttribute(t *testing.T) {
	ao := NewAssemblyObject()
	ao.RegisterAssembly(5, make([]byte, 4))

	path := cip.Path([]byte{0x24, 0x05, 0x30, 0x03})
	writeData := []byte{0x01, 0x02, 0x03, 0x04}

	_, err := ao.HandleRequest(cip.ServiceSetAttributeSingle, path, writeData)
	if err != nil {
		t.Fatalf("HandleRequest Set: %v", err)
	}

	// Read back to verify.
	readData, err := ao.HandleRequest(cip.ServiceGetAttributeSingle, path, nil)
	if err != nil {
		t.Fatalf("HandleRequest Get after Set: %v", err)
	}
	if !bytes.Equal(readData, writeData) {
		t.Errorf("data = %X, want %X", readData, writeData)
	}
}
