package provider

import (
	"context"
	"errors"
)

// These errors drive the retry/fallback policy in the router.
// A Declined is fatal (do not retry), but a Timeout is retryable.
var (
	ErrProviderDeclined = errors.New("provider: payment declined")
	ErrProviderTimeout  = errors.New("provider: network timeout")
	ErrProviderInternal = errors.New("provider: internal error")
)

type ChargeRequest struct {
	PaymentID   string
	AmountCents int64
	Currency    string
}

type ChargeResponse struct {
	ProviderReference string // ex. "ch_123" from the provider
	Status            string // ex. "authorized"
}

// ProviderAdapter is the extension point. Adding Stripe or Adyen means
// writing a new struct that satisfies these two methods. Nothing else in
// the orchestrator needs to change.
type ProviderAdapter interface {
	Name() string

	// Charge must respect context cancellation so the router can time
	// out a hung provider and fall back to another.
	Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error)
}