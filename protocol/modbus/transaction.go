package modbus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// Transaction represents an in-flight Modbus TCP request/response pair. The
// MBAP transaction ID is used to correlate the response from the server back to
// the original request. Callers receive the result via the ResponseCh or ErrCh
// channels.
type Transaction struct {
	Request    *Request        // The Modbus request
	ResponseCh chan *Response  // Channel for receiving the response
	ErrCh      chan error      // Channel for receiving errors
	ctx        context.Context
	cancelFunc context.CancelFunc
	createTime time.Time
}

// NewTransaction creates a new Transaction with buffered response and error
// channels. The transaction inherits a cancellable child of the given context.
func NewTransaction(ctx context.Context, request *Request) *Transaction {
	ctx, cancel := context.WithCancel(ctx)

	return &Transaction{
		Request:    request,
		ResponseCh: make(chan *Response, 1),
		ErrCh:      make(chan error, 1),
		ctx:        ctx,
		cancelFunc: cancel,
		createTime: time.Now(),
	}
}

// Complete signals that the transaction has finished. Exactly one of response or
// err should be non-nil. The result is delivered via the ResponseCh or ErrCh
// channel respectively, and the transaction's context is cancelled.
func (t *Transaction) Complete(response *Response, err error) {
	if err != nil {
		select {
		case t.ErrCh <- err:
		default:
		}
	} else {
		select {
		case t.ResponseCh <- response:
		default:
		}
	}
	t.cancelFunc()
}

// Cancel cancels the transaction with an error.
func (t *Transaction) Cancel(err error) {
	t.Complete(nil, err)
}

// Context returns the transaction's context.
func (t *Transaction) Context() context.Context {
	return t.ctx
}

// GetLifetime returns the elapsed time since the transaction was created.
func (t *Transaction) GetLifetime() time.Duration {
	return time.Since(t.createTime)
}

// ---------------------------------------------------------------------------
// TransactionPool
// ---------------------------------------------------------------------------

const (
	// DefaultTransactionTimeout is the default timeout for transactions.
	DefaultTransactionTimeout = 5 * time.Second

	// maxIDAttempts is the number of IDs to try before giving up when
	// there is a collision with an in-flight transaction.
	maxIDAttempts = 16
)

// TransactionPool manages the set of active (in-flight) Modbus TCP
// transactions. It assigns unique 16-bit transaction IDs, tracks pending
// requests, and automatically cancels transactions that exceed the configured
// timeout.
type TransactionPool struct {
	logger          logging.Logger
	transactions    map[TransactionID]*Transaction
	transactionsMu  sync.Mutex
	nextID          atomic.Uint32
	done            chan struct{}
	timeoutDuration time.Duration
}

// TransactionPoolOption is a functional option for configuring a
// [TransactionPool].
type TransactionPoolOption func(*TransactionPool)

// WithPoolTimeout sets the maximum time a transaction may remain in-flight
// before being automatically cancelled. The default is 5 seconds.
func WithPoolTimeout(timeout time.Duration) TransactionPoolOption {
	return func(tp *TransactionPool) {
		if timeout > 0 {
			tp.timeoutDuration = timeout
		}
	}
}

// WithPoolLogger sets the logger for the transaction pool.
func WithPoolLogger(logger logging.Logger) TransactionPoolOption {
	return func(tp *TransactionPool) {
		tp.logger = logger
	}
}

// NewTransactionPool creates a new TransactionPool and starts a background
// goroutine that monitors for timed-out transactions. Call [TransactionPool.Close]
// to stop the monitor and cancel all pending transactions.
func NewTransactionPool(options ...TransactionPoolOption) *TransactionPool {
	pool := &TransactionPool{
		logger:          logging.NewNopLogger(),
		transactions:    make(map[TransactionID]*Transaction),
		done:            make(chan struct{}),
		timeoutDuration: DefaultTransactionTimeout,
	}

	for _, option := range options {
		option(pool)
	}

	// Start the timeout monitor goroutine.
	go pool.timeoutMonitor()

	return pool
}

// Close stops the timeout monitor, cancels all pending transactions, and
// prevents new transactions from being placed.
func (tp *TransactionPool) Close() {
	ctx := context.Background()
	tp.logger.Info(ctx, "Closing transaction pool")

	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	// Check if done channel is already closed.
	select {
	case <-tp.done:
		return
	default:
		close(tp.done)
	}

	// Cancel all pending transactions.
	for txID, t := range tp.transactions {
		tp.logger.Debug(ctx, "Cancelling transaction %d", txID)
		if t != nil {
			t.Cancel(ErrTransportClosing)
			delete(tp.transactions, txID)
		}
	}
}

// timeoutMonitor periodically checks for timed-out transactions.
func (tp *TransactionPool) timeoutMonitor() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tp.done:
			return
		case <-ticker.C:
			tp.checkTimeouts()
		}
	}
}

// checkTimeouts looks for timed-out transactions and cancels them.
func (tp *TransactionPool) checkTimeouts() {
	ctx := context.Background()
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	for txID, tx := range tp.transactions {
		if tx.GetLifetime() > tp.timeoutDuration {
			tp.logger.Warn(ctx, "Transaction %d timed out after %v", txID, tx.GetLifetime())
			tp.unsafeRelease(txID)
			tx.Cancel(ErrTransactionTimeout)
		}
	}
}

// GetCount returns the current count of active transactions.
func (tp *TransactionPool) GetCount() int {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()
	return len(tp.transactions)
}

// Place creates a new [Transaction] for the given request, assigns it a unique
// transaction ID, and adds it to the pool. The transaction ID is also written
// into the request's MBAP header.
func (tp *TransactionPool) Place(ctx context.Context, request *Request) (*Transaction, error) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	// Check if the pool was closed.
	select {
	case <-tp.done:
		return nil, fmt.Errorf("transaction pool is closed")
	default:
	}

	// Try to find a free transaction ID.
	var txID TransactionID
	found := false
	for i := 0; i < maxIDAttempts; i++ {
		id := tp.nextID.Add(1) - 1 // post-increment semantics
		candidate := TransactionID(id & 0xFFFF)
		if _, exists := tp.transactions[candidate]; !exists {
			txID = candidate
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("transaction pool is full (no free IDs after %d attempts)", maxIDAttempts)
	}

	request.SetTransactionID(txID)

	tp.logger.Debug(ctx, "Placing transaction with ID: %d", txID)

	tx := NewTransaction(ctx, request)
	tp.transactions[txID] = tx

	return tx, nil
}

// Get retrieves a transaction by its ID without removing it from the pool.
func (tp *TransactionPool) Get(txID TransactionID) (*Transaction, bool) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	tx, exists := tp.transactions[txID]
	return tx, exists
}

// Release removes a transaction from the pool and returns it. The caller is
// responsible for completing the transaction via [Transaction.Complete].
func (tp *TransactionPool) Release(txID TransactionID) (result *Transaction, ok bool) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	result, ok = tp.transactions[txID]
	if ok {
		tp.unsafeRelease(txID)
	}

	return
}

// unsafeRelease removes a transaction without locking. Caller must hold transactionsMu.
func (tp *TransactionPool) unsafeRelease(txID TransactionID) {
	delete(tp.transactions, txID)
}

// unsafeReset cancels all transactions and re-initialises the pool.
// Caller must hold transactionsMu.
func (tp *TransactionPool) unsafeReset() {
	ctx := context.Background()

	for txID, tx := range tp.transactions {
		if tx != nil {
			tp.logger.Debug(ctx, "Cancelling transaction %d during reset", txID)
			tx.Cancel(ErrTransportClosing)
		}
	}

	tp.transactions = make(map[TransactionID]*Transaction)
}
