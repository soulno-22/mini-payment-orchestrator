package provider

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

// MockProvider is a deterministic provider for testing. Configure behavior
// via the exported fields; CallCount is atomic because Phase 3's flagship
// test hammers this from 50 goroutines.
type MockProvider struct {
	providerName string

	InjectLatency time.Duration
	InjectError   error

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

	// select instead of time.Sleep so a cancelled context short-circuits.
	if m.InjectLatency > 0 {
		select {
		case <-time.After(m.InjectLatency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.InjectError != nil {
		return nil, m.InjectError
	}

	return &ChargeResponse{
		ProviderReference: fmt.Sprintf("%s_ref_%s", m.providerName, req.PaymentID),
		Status:            "authorized",
	}, nil
}