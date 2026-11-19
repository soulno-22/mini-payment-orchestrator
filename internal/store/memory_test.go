package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
)

func newPayment(id string) *domain.Payment {
	return &domain.Payment{
		ID:          id,
		MerchantID:  "m1",
		AmountCents: 1000,
		Currency:    "INR",
		State:       domain.StateCreated,
	}
}

func TestCreateAndGet(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	p := newPayment("p1")
	if err := s.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.Get(ctx, "p1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("want version 1, got %d", got.Version)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}
	got.State = domain.StateCaptured
	again, _ := s.Get(ctx, "p1")
	if again.State != domain.StateCreated {
		t.Fatal("Get leaked internal pointer")
	}
}

func TestUpdateVersionConflict(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_ = s.Create(ctx, newPayment("p1"))

	_, err := s.Update(ctx, "p1", 1, func(p *domain.Payment) (*domain.Payment, error) {
		p.State = domain.StateAuthorized
		return p, nil
	})
	if err != nil {
		t.Fatalf("update A: %v", err)
	}

	_, err = s.Update(ctx, "p1", 1, func(p *domain.Payment) (*domain.Payment, error) {
		p.State = domain.StateFailed
		return p, nil
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("want ErrVersionConflict, got %v", err)
	}
}

func TestUpdateFnErrorDoesNotPersist(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_ = s.Create(ctx, newPayment("p1"))

	sentinel := errors.New("boom")
	_, err := s.Update(ctx, "p1", 1, func(p *domain.Payment) (*domain.Payment, error) {
		p.State = domain.StateFailed
		return nil, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
	got, _ := s.Get(ctx, "p1")
	if got.State != domain.StateCreated || got.Version != 1 {
		t.Fatalf("state mutated despite fn error: %+v", got)
	}
}

func TestConcurrentUpdatesOnlyOneWins(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()
	_ = s.Create(ctx, newPayment("p1"))

	const N = 50
	var wins, conflicts int64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, err := s.Update(ctx, "p1", 1, func(p *domain.Payment) (*domain.Payment, error) {
				p.State = domain.StateAuthorized
				return p, nil
			})
			switch {
			case err == nil:
				atomic.AddInt64(&wins, 1)
			case errors.Is(err, ErrVersionConflict):
				atomic.AddInt64(&conflicts, 1)
			default:
				t.Errorf("unexpected err: %v", err)
			}
		}()
	}
	wg.Wait()

	if wins != 1 {
		t.Fatalf("want exactly 1 winner, got %d", wins)
	}
	if conflicts != N-1 {
		t.Fatalf("want %d conflicts, got %d", N-1, conflicts)
	}
}
