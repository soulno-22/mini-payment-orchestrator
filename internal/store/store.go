package store

import (
	"context"
	"errors"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
)

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("store: not found")

// ErrVersionConflict is returned by Update when the caller's expected
// version does not match the stored version.
var ErrVersionConflict = errors.New("store: version conflict")

// Store is the persistence boundary.
type Store interface {
	Create(ctx context.Context, p *domain.Payment) error
	Get(ctx context.Context, id string) (*domain.Payment, error)
	Update(ctx context.Context, id string, expectedVersion int64, fn func(*domain.Payment) (*domain.Payment, error)) (*domain.Payment, error)

	// SaveIdempotencyKey stores the response for a given key.
	SaveIdempotencyKey(ctx context.Context, rec *domain.IdempotencyRecord) error

	// GetIdempotencyKey fetches the record. Returns ErrNotFound if missing.
	GetIdempotencyKey(ctx context.Context, key string) (*domain.IdempotencyRecord, error)

	// SaveWebhookEvent marks an event ID as processed. 
	// Returns ErrVersionConflict if it was already processed.
	SaveWebhookEvent(ctx context.Context, eventID string) error
}
