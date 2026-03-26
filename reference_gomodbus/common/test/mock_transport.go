package test

import (
	"context"
	"sync"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// MockTransport implements the common.Transport interface for testing
type MockTransport struct {
	connected     bool
	mu            sync.Mutex
	responseQueue []common.Response
	requests      []common.Request
	errorQueue    []error
	logger        common.LoggerInterface
}

// NewMockTransport creates a new mock transport
func NewMockTransport() *MockTransport {
	return &MockTransport{
		connected:     false,
		responseQueue: make([]common.Response, 0),
		requests:      make([]common.Request, 0),
		errorQueue:    make([]error, 0),
		logger:        nil,
	}
}

// Connect establishes a connection
func (t *MockTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = true
	return nil
}

// Disconnect closes the connection
func (t *MockTransport) Disconnect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.connected = false
	return nil
}

// IsConnected returns true if the transport is connected
func (t *MockTransport) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.connected
}

// Send sends a request and returns a response
func (t *MockTransport) Send(ctx context.Context, request common.Request) (common.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if connected
	if !t.connected {
		return nil, common.ErrNotConnected
	}

	// Record the request
	t.requests = append(t.requests, request)

	// Return the next queued response or error
	if len(t.errorQueue) > 0 {
		err := t.errorQueue[0]
		t.errorQueue = t.errorQueue[1:]
		return nil, err
	}

	if len(t.responseQueue) > 0 {
		resp := t.responseQueue[0]
		t.responseQueue = t.responseQueue[1:]
		return resp, nil
	}

	return nil, common.ErrNoResponse
}

// QueueResponse adds a response to the queue
func (t *MockTransport) QueueResponse(response common.Response) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responseQueue = append(t.responseQueue, response)
}

// QueueError adds an error to the queue
func (t *MockTransport) QueueError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorQueue = append(t.errorQueue, err)
}

// GetRequests returns the received requests
func (t *MockTransport) GetRequests() []common.Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.requests
}

// Clear clears all queues
func (t *MockTransport) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.responseQueue = make([]common.Response, 0)
	t.requests = make([]common.Request, 0)
	t.errorQueue = make([]error, 0)
}

// WithLogger sets the logger for the transport
func (t *MockTransport) WithLogger(logger common.LoggerInterface) common.Transport {
	t.mu.Lock()
	defer t.mu.Unlock()

	newTransport := &MockTransport{
		connected:     t.connected,
		responseQueue: t.responseQueue,
		requests:      t.requests,
		errorQueue:    t.errorQueue,
		logger:        logger,
	}

	return newTransport
}