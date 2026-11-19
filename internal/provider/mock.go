package provider

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// MockProvider is a deterministic provider for testing.
// We inject the exact behavior we want via the struct fields.
type MockProvider struct {
	providerName string

	// InjectLatency forces the provider to sleep before responding.
	InjectLatency time.Duration

	// InjectError forces the provider to return this error.
	InjectError error

	// CallCount tracks how many times Charge was called.
	// Useful for Phase 3's idempotency tests.
	CallCount atomic.Int64
}

func NewMockProvider(name string) *MockProvider {
	return &MockProvider{providerName: name}
}

func (m *MockProvider) Name() string {
	return m.providerName
}

func (m *MockProvider) Charge(ctx context.Context, req ChargeRequest) (*ChargeResponse, error) {
	m.CallCount.Add(1)

	// Simulate network latency. We must respect the context deadline.
	if m.InjectLatency > 0 {
		select {
		case <-time.After(m.InjectLatency):
			// Latency finished
		case <-ctx.Done():
			// The orchestrator cancelled the request (timeout)
			return nil, ctx.Err()
		}
	}

	// If an error was injected, return it.
	if m.InjectError != nil {
		return nil, m.InjectError
	}

	// Success!
	return &ChargeResponse{
		ProviderReference: fmt.Sprintf("%s_ref_%s", m.providerName, req.PaymentID),
		Status:            "authorized",
	}, nil
}
