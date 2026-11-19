package store

import (
	"context"
	"sync"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
)

// MemoryStore is a process-local Store. It is NOT durable.
type MemoryStore struct {
	mu          sync.RWMutex
	payments    map[string]*domain.Payment
	idempotency map[string]*domain.IdempotencyRecord
	webhooks    map[string]bool // <-- THE MISSING FIELD
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		payments:    make(map[string]*domain.Payment),
		idempotency: make(map[string]*domain.IdempotencyRecord),
		webhooks:    make(map[string]bool), // <-- INITIALIZED
	}
}

func (s *MemoryStore) Create(ctx context.Context, p *domain.Payment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.payments[p.ID]; exists {
		return ErrVersionConflict
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Version = 1
	cp := *p
	s.payments[p.ID] = &cp
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, id string) (*domain.Payment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.payments[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (s *MemoryStore) Update(
	ctx context.Context,
	id string,
	expectedVersion int64,
	fn func(*domain.Payment) (*domain.Payment, error),
) (*domain.Payment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.payments[id]
	if !ok {
		return nil, ErrNotFound
	}
	if cur.Version != expectedVersion {
		return nil, ErrVersionConflict
	}

	working := *cur
	next, err := fn(&working)
	if err != nil {
		return nil, err
	}
	if next == nil {
		return nil, ErrVersionConflict
	}

	next.Version = cur.Version + 1
	next.CreatedAt = cur.CreatedAt
	next.UpdatedAt = time.Now().UTC()
	stored := *next
	s.payments[id] = &stored
	cp := stored
	return &cp, nil
}

func (s *MemoryStore) SaveIdempotencyKey(ctx context.Context, rec *domain.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.idempotency[rec.Key]; exists {
		return ErrVersionConflict
	}
	s.idempotency[rec.Key] = rec
	return nil
}

func (s *MemoryStore) GetIdempotencyKey(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.idempotency[key]
	if !ok {
		return nil, ErrNotFound
	}
	return rec, nil
}

// SaveWebhookEvent marks an event ID as processed.
func (s *MemoryStore) SaveWebhookEvent(ctx context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.webhooks[eventID]; exists {
		return ErrVersionConflict
	}
	s.webhooks[eventID] = true
	return nil
}