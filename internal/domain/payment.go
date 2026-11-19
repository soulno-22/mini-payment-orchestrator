package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// State is the lifecycle state of a payment.
type State string

const (
	StateCreated    State = "CREATED"
	StateAuthorized State = "AUTHORIZED"
	StateCaptured   State = "CAPTURED"
	StateFailed     State = "FAILED"
	StateRefunded   State = "REFUNDED"
)

// Payment is the persisted record.
type Payment struct {
	ID          string
	MerchantID  string
	AmountCents int64
	Currency    string
	Provider    string
	State       State
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewID generates a unique payment ID.
func NewID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "pay_" + hex.EncodeToString(b[:])
}
