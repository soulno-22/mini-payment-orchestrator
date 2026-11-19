package store

import (
	"context"
	"errors"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
)

var (
	// Callers should use errors.Is to check these.
	ErrNotFound        = errors.New("store: not found")
	ErrVersionConflict = errors.New("store: version conflict")
)

// Store is the boundary between our business logic and persistence.
//
// Keeping this as an interface rather than a concrete struct is deliberate:
// right now it's a map in memory, but the only thing stopping us from
// swapping in Postgres is writing a new struct that satisfies these
// signatures. The rest of the app shouldn't know or care.
type Store interface {
	Create(ctx context.Context, p *domain.Payment) error
	Get(ctx context.Context, id string) (*domain.Payment, error)

	// Update is a read-modify-write operation, but it's not as simple as
	// Get + mutate + Save. The expectedVersion check is what prevents two
	// goroutines from silently clobbering each other. If you're looking for
	// the lost-update prevention, it lives here.
	Update(ctx context.Context, id string, expectedVersion int64, fn func(*domain.Payment) (*domain.Payment, error)) (*domain.Payment, error)

	// Idempotency records are stored separately from payments because a
	// "key" can outlive the payment it was attached to (TTL cleanup, etc).
	// SaveIdempotencyKey returns ErrVersionConflict if the key already exists.
	SaveIdempotencyKey(ctx context.Context, rec *domain.IdempotencyRecord) error
	GetIdempotencyKey(ctx context.Context, key string) (*domain.IdempotencyRecord, error)

	// Webhook events get their own table/namespace so we can dedupe by event
	// ID without touching payment data. Returns ErrVersionConflict if the
	// event has already been seen (i.e. this is a retry).
	SaveWebhookEvent(ctx context.Context, eventID string) error
}