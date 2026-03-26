package transport

import (
	"context"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
)

// Transaction represents an ongoing transaction with a request, response channel, and context
// The Modbus TCP protocol uses a transaction ID to match requests and responses
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
type Transaction struct {
	Request    common.Request      // The Modbus request
	ResponseCh chan common.Response // Channel for receiving the response
	ErrCh      chan error          // Channel for receiving errors
	ctx        context.Context     // Context for cancellation
	cancelFunc context.CancelFunc  // Function to cancel the context
	createTime time.Time           // Time when the transaction was created, used for timeout detection
}

// NewTransaction creates a new transaction with a given request and context
func NewTransaction(ctx context.Context, request common.Request) *Transaction {
	ctx, cancel := context.WithCancel(ctx)

	return &Transaction{
		Request:    request,
		ResponseCh: make(chan common.Response, 1),
		ErrCh:      make(chan error, 1),
		ctx:        ctx,
		cancelFunc: cancel,
		createTime: time.Now(),
	}
}

// Complete signals the transaction is complete with either a response or error
func (t *Transaction) Complete(response common.Response, err error) {
	if err != nil {
		// Non-blocking send to error channel
		select {
		case t.ErrCh <- err:
		default:
			// Channel is full or closed
		}
	} else {
		// Non-blocking send to response channel
		select {
		case t.ResponseCh <- response:
		default:
			// Channel is full or closed
		}
	}
	t.cancelFunc()
}

// Cancel cancels the transaction with an error
func (t *Transaction) Cancel(err error) {
	t.Complete(nil, err)
}

// Context returns the transaction's context
func (t *Transaction) Context() context.Context {
	return t.ctx
}

// GetLifetime returns the transaction's lifetime
func (t *Transaction) GetLifetime() time.Duration {
	return time.Since(t.createTime)
}
