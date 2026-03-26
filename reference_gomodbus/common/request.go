package common

// Request represents a Modbus request
type Request interface {
	// GetTransactionID returns the transaction ID.
	GetTransactionID() TransactionID
	// SetTransactionID sets the transaction ID.
	SetTransactionID(id TransactionID)
	// GetUnitID returns the unit ID.
	GetUnitID() UnitID
	// GetPDU returns the PDU.
	GetPDU() *PDU
	// Encode encodes the request into bytes.
	Encode() ([]byte, error)
}
