package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMockProvider_Success(t *testing.T) {
	p := NewMockProvider("test_a")
	req := ChargeRequest{PaymentID: "pay_1", AmountCents: 500, Currency: "INR"}

	resp, err := p.Charge(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.ProviderReference != "test_a_ref_pay_1" {
		t.Fatalf("unexpected ref: %s", resp.ProviderReference)
	}
	if p.CallCount.Load() != 1 {
		t.Fatalf("want 1 call, got %d", p.CallCount)
	}
}

func TestMockProvider_InjectedError(t *testing.T) {
	p := NewMockProvider("test_a")
	p.InjectError = ErrProviderDeclined

	_, err := p.Charge(context.Background(), ChargeRequest{PaymentID: "pay_1"})
	if !errors.Is(err, ErrProviderDeclined) {
		t.Fatalf("want ErrProviderDeclined, got %v", err)
	}
}

func TestMockProvider_InjectedLatency(t *testing.T) {
	p := NewMockProvider("test_a")
	p.InjectLatency = 50 * time.Millisecond

	start := time.Now()
	_, err := p.Charge(context.Background(), ChargeRequest{PaymentID: "pay_1"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("latency not injected, took %v", elapsed)
	}
}

func TestMockProvider_ContextCancellation(t *testing.T) {
	p := NewMockProvider("test_a")
	p.InjectLatency = 100 * time.Millisecond

	// Create a context that cancels after 10ms.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := p.Charge(ctx, ChargeRequest{PaymentID: "pay_1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want context.DeadlineExceeded, got %v", err)
	}
}
