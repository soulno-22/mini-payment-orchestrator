package provider

import (
	"context"
	"errors"
)

// Standard errors returned by providers.
// The orchestrator (Phase 4) will use these to decide whether to retry,
// fall back to another provider, or fail the payment permanently.
var (
	ErrProviderDeclined = errors.New("provider: payment declined")
	ErrProviderTimeout  = errors.New("provider: network timeout")
	ErrProviderInternal = errors.New("provider: internal error")
)

// ChargeRequest is what the orchestrator sends to a provider.
type ChargeRequest struct {
	PaymentID   string
	AmountCents int64
	Currency    string
}

// ChargeResponse is what a successful provider returns.
type ChargeResponse struct {
	ProviderReference string // the provider's ID for this charge (e.g., "ch_123")
	Status            string // e.g., "authorized"
}

// ProviderAdapter is the core extension point.
// Adding a real provider (Stripe, Adyen) means writing a new struct that
// implements exactly these two methods. No core orchestrator code changes.
type ProviderAdapter interface {
	// Name returns a stable identifier (e.g., "provider_a").
	Name() string

	// Charge attempts to authorize a payment.
	// It must respect context cancellation (deadlines/timeouts).
	Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
}
