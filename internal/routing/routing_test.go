package routing

import (
	"context"
	"testing"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
)

func TestCircuitBreaker_TripsAndResets(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond) // 2 failures, 50ms cooldown

	// 1. Initially closed
	if !cb.Allow() {
		t.Fatal("expected allowed initially")
	}

	// 2. Two failures trip the breaker
	cb.RecordResult(false)
	cb.RecordResult(false)

	if cb.Allow() {
		t.Fatal("expected blocked after 2 failures")
	}

	// 3. Wait for cooldown -> Half-Open
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected allowed in Half-Open after cooldown")
	}

	// 4. Success in Half-Open closes it
	cb.RecordResult(true)
	if cb.state != StateClosed {
		t.Fatal("expected Closed after success in Half-Open")
	}
}

func TestRouter_Fallback(t *testing.T) {
	// Provider A fails, Provider B succeeds
	pA := provider.NewMockProvider("a")
	pA.InjectError = provider.ErrProviderDeclined
	pB := provider.NewMockProvider("b")
	
	// Strategy: Picks A first (since no history)
	strategy := NewSlidingWindowStrategy(10)
		// Inside TestRouter_Fallback setup:
	m := metrics.NewRegistry()
	router := NewRouter([]provider.ProviderAdapter{pA, pB}, strategy, m)

	req := provider.ChargeRequest{PaymentID: "pay_1", AmountCents: 100, Currency: "INR"}
	
	resp, err := router.Charge(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got err: %v", err)
	}
	if resp.ProviderReference != "b_ref_pay_1" {
		t.Fatalf("expected fallback to provider B, got %s", resp.ProviderReference)
	}
}