package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Moonlight-Companies/gomodbus/common"
	"github.com/Moonlight-Companies/gomodbus/logging"
)

// TransactionPool manages a pool of active transactions
type TransactionPool struct {
	logger          common.LoggerInterface
	transactions    map[common.TransactionID]*Transaction
	transactionsMu  sync.Mutex
	freeIDs         chan common.TransactionID // Use a channel as a queue for free IDs
	done            chan struct{}
	timeoutDuration time.Duration
}

// TransactionPoolOption is a function that configures a TransactionPool
type TransactionPoolOption func(*TransactionPool)

// WithTimeout sets the timeout duration for transactions
func WithTimeout(timeout time.Duration) TransactionPoolOption {
	return func(tp *TransactionPool) {
		if timeout > 0 {
			tp.timeoutDuration = timeout
		}
	}
}

// WithLogger sets the logger for the transaction pool
func WithLogger(logger common.LoggerInterface) TransactionPoolOption {
	return func(tp *TransactionPool) {
		tp.logger = logger
	}
}

const (
	// MaxTransactions is the maximum number of possible transaction IDs such that the
	// buffered channel never blocks
	MaxTransactions = 0xFFFF + 1
	// DefaultTimeout is the default timeout for transactions
	DefaultTimeout = 5 * time.Second
)

// NewTransactionPool creates a new transaction pool
func NewTransactionPool(options ...TransactionPoolOption) *TransactionPool {
	pool := &TransactionPool{
		logger:          logging.NewLogger(), // Default logger
		transactions:    make(map[common.TransactionID]*Transaction),
		freeIDs:         make(chan common.TransactionID, MaxTransactions),
		done:            make(chan struct{}),
		timeoutDuration: DefaultTimeout,
	}

	// Apply options
	for _, option := range options {
		option(pool)
	}

	// Pre-populate the free IDs channel
	for i := 0; i < MaxTransactions; i++ {
		pool.freeIDs <- common.TransactionID(i)
	}

	// Start the timeout monitor goroutine
	go pool.timeoutMonitor()

	return pool
}

// Close shuts down the transaction pool
func (tp *TransactionPool) Close() {
	ctx := context.Background()
	tp.logger.Info(ctx, "Closing transaction pool")

	// Use a mutex to protect against concurrent Close calls
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	// Check if done channel is already closed
	select {
	case <-tp.done:
		// Already closed, don't close again
	default:
		close(tp.done)
	}

	// Check if freeIDs channel is already closed
	// We can do this by trying to receive from it in a non-blocking way
	select {
	case _, ok := <-tp.freeIDs:
		if ok {
			// Channel is still open, close it
			close(tp.freeIDs)
		}
	default:
		// Try closing it anyway, but recover from panic if it's already closed
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Channel was already closed, ignore the panic
				}
			}()
			close(tp.freeIDs)
		}()
	}

	// Cancel all pending transactions
	for txID, t := range tp.transactions {
		tp.logger.Debug(ctx, "Cancelling transaction %d", txID)
		if t != nil {
			t.Cancel(common.ErrTransportClosing)
			delete(tp.transactions, txID)
		}
	}
}

// timeoutMonitor periodically checks for timed out transactions
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

// checkTimeouts looks for timed out transactions and cancels them
func (tp *TransactionPool) checkTimeouts() {
	ctx := context.Background()
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	for txID, tx := range tp.transactions {
		if tx.GetLifetime() > tp.timeoutDuration {
			tp.logger.Warn(ctx, "Transaction %d timed out after %v", txID, tx.GetLifetime())
			tp.unsafeRelease(txID)

			// Cancel the transaction with timeout error
			tx.Cancel(common.ErrTransactionTimeout)
		}
	}
}

// GetCount returns the current count of active transactions
func (tp *TransactionPool) GetCount() int {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()
	return len(tp.transactions)
}

// Place adds a transaction to the pool and assigns it a transaction ID
func (tp *TransactionPool) Place(ctx context.Context, request common.Request) (*Transaction, error) {
	var txID common.TransactionID
	var ok bool

	// Try to get an ID from the free list
	select {
	case txID, ok = <-tp.freeIDs:
		if !ok {
			return nil, fmt.Errorf("freeIDs channel closed, pool is likely shutting down")
		}
	default:
		// No free IDs available
		return nil, fmt.Errorf("transaction pool is full (no IDs in free list)")
	}

	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	// Check if the pool was closed between receiving the free ID and acquiring the lock.
	// If so, the txID is discarded — freeIDs may already be closed so returning it would panic.
	select {
	case <-tp.done:
		return nil, fmt.Errorf("transaction pool is closed")
	default:
	}

	// Set the transaction ID on the request
	request.SetTransactionID(txID)

	tp.logger.Debug(ctx, "Placing transaction with ID: %d", txID)

	// Create a new transaction
	tx := NewTransaction(ctx, request)

	// Store in the pool
	tp.transactions[txID] = tx

	return tx, nil
}

// Get retrieves a transaction by its ID without removing it
func (tp *TransactionPool) Get(txID common.TransactionID) (*Transaction, bool) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	tx, exists := tp.transactions[txID]
	return tx, exists
}

// Release removes a transaction from the pool and returns it
func (tp *TransactionPool) Release(txID common.TransactionID) (result *Transaction, ok bool) {
	tp.transactionsMu.Lock()
	defer tp.transactionsMu.Unlock()

	result, ok = tp.transactions[txID]
	if ok {
		tp.unsafeRelease(txID)
	}

	return
}

func (tp *TransactionPool) unsafeRelease(txID common.TransactionID) {
	// Caller must hold mu
	delete(tp.transactions, txID)

	// Only send to freeIDs if the channel is still open
	// This prevents panics during shutdown
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Channel was closed, ignore the panic
			}
		}()

		// Use non-blocking send with a select to avoid deadlocks
		select {
		case tp.freeIDs <- txID:
			// Successfully sent ID back to pool
		default:
			// Channel is full or closed, just drop the ID
		}
	}()
}

func (tp *TransactionPool) unsafeReset() {
	// Caller must hold mu
	ctx := context.Background()

	// Cancel all transactions with a consistent error message
	for txID, tx := range tp.transactions {
		if tx != nil {
			tp.logger.Debug(ctx, "Cancelling transaction %d during reset", txID)
			tx.Cancel(common.ErrTransportClosing)
		}
	}

	// Create fresh transaction map and freeIDs channel
	tp.transactions = make(map[common.TransactionID]*Transaction)
	tp.freeIDs = make(chan common.TransactionID, MaxTransactions)

	// Pre-populate the free IDs channel
	for i := range MaxTransactions {
		tp.freeIDs <- common.TransactionID(i)
	}
}
