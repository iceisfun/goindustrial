package modbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/iceisfun/goindustrial/logging"
)

// Transaction represents an ongoing Modbus TCP transaction with a request,
// response channel, and context. The transaction ID in the MBAP header is used
// to match requests and responses.
// Ref: Modbus_Application_Protocol_V1_1b3.pdf, Section 4.1 (MBAP Header)
type Transaction struct {
	Request    *Request        // The Modbus request
	ResponseCh chan *Response  // Channel for receiving the response
	ErrCh      chan error      // Channel for receiving errors
	ctx        context.Context
	cancelFunc context.CancelFunc
	createTime time.Time
}

// NewTransaction creates a new transaction with a given request and context.
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

// Complete signals the transaction is complete with either a response or error.
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

// GetLifetime returns the transaction's lifetime.
func (t *Transaction) GetLifetime() time.Duration {
	return time.Since(t.createTime)
}

// ---------------------------------------------------------------------------
// TransactionPool
// ---------------------------------------------------------------------------

const (
	// MaxTransactions is the maximum number of possible transaction IDs such
	// that the buffered channel never blocks.
	MaxTransactions = 0xFFFF + 1
	// DefaultTransactionTimeout is the default timeout for transactions.
	DefaultTransactionTimeout = 5 * time.Second
)

// TransactionPool manages a pool of active Modbus TCP transactions.
type TransactionPool struct {
	logger          logging.Logger
	transactions    map[TransactionID]*Transaction
	transactionsMu  sync.Mutex
	freeIDs         chan TransactionID
	done            chan struct{}
	timeoutDuration time.Duration
}

// TransactionPoolOption is a function that configures a TransactionPool.
type TransactionPoolOption func(*TransactionPool)

// WithPoolTimeout sets the timeout duration for transactions.
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

// NewTransactionPool creates a new transaction pool.
func NewTransactionPool(options ...TransactionPoolOption) *TransactionPool {
	pool := &TransactionPool{
		logger:          logging.NewNopLogger(),
		transactions:    make(map[TransactionID]*Transaction),
		freeIDs:         make(chan TransactionID, MaxTransactions),
		done:            make(chan struct{}),
		timeoutDuration: DefaultTransactionTimeout,
	}

	for _, option := range options {
		option(pool)
	}

	// Pre-populate the free IDs channel.
	for i := 0; i < MaxTransactions; i++ {
		pool.freeIDs <- TransactionID(i)
	}

	// Start the timeout monitor goroutine.
	go pool.timeoutMonitor()

	return pool
}

// Close shuts down the transaction pool.
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

	// Safely close freeIDs channel.
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Channel was already closed.
			}
		}()
		select {
		case _, ok := <-tp.freeIDs:
			if ok {
				close(tp.freeIDs)
			}
		default:
			close(tp.freeIDs)
		}
	}()

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

// Place adds a transaction to the pool and assigns it a transaction ID.
func (tp *TransactionPool) Place(ctx context.Context, request *Request) (*Transaction, error) {
	var txID TransactionID
	var ok bool

	select {
	case txID, ok = <-tp.freeIDs:
		if !ok {
			return nil, fmt.Errorf("freeIDs channel closed, pool is likely shutting down")
		}
	default:
		return nil, fmt.Errorf("transaction pool is full (no IDs in free list)")
	}

	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	// Check if the pool was closed between receiving the free ID and acquiring the lock.
	select {
	case <-tp.done:
		return nil, fmt.Errorf("transaction pool is closed")
	default:
	}

	request.SetTransactionID(txID)

	tp.logger.Debug(ctx, "Placing transaction with ID: %d", txID)

	tx := NewTransaction(ctx, request)
	tp.transactions[txID] = tx

	return tx, nil
}

// Get retrieves a transaction by its ID without removing it.
func (tp *TransactionPool) Get(txID TransactionID) (*Transaction, bool) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	tx, exists := tp.transactions[txID]
	return tx, exists
}

// Release removes a transaction from the pool and returns it.
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

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Channel was closed.
			}
		}()

		select {
		case tp.freeIDs <- txID:
		default:
		}
	}()
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
	tp.freeIDs = make(chan TransactionID, MaxTransactions)

	for i := range MaxTransactions {
		tp.freeIDs <- TransactionID(i)
	}
}
