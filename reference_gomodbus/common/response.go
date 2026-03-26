package common

// Response represents a Modbus response
type Response interface {
	// GetTransactionID returns the transaction ID.
	GetTransactionID() TransactionID
	// GetUnitID returns the unit ID.
	GetUnitID() UnitID
	// GetPDU returns the PDU.
	GetPDU() *PDU
	// IsException checks if the response is an exception.
	IsException() bool
	// GetException returns the exception code if the response is an exception.
	GetException() ExceptionCode
	// ToError converts an exception response to an error.
	ToError() error
	// Encode encodes the response into bytes.
	Encode() ([]byte, error)
}
