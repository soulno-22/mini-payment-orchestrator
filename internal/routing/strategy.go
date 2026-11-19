package routing

import (
	"context"
	"sync"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
)

// RoutingStrategy is an extension point.
// You can implement a new strategy (e.g., lowest cost, geo-based) without
// changing the Router.
type RoutingStrategy interface {
	Pick(providers []provider.ProviderAdapter) provider.ProviderAdapter
}

// Router handles retries, fallbacks, and uses Circuit Breakers.
type Router struct {
	Providers []provider.ProviderAdapter
	Breakers  map[string]*CircuitBreaker
	Strategy  RoutingStrategy
	Metrics   *metrics.Registry
}

func NewRouter(providers []provider.ProviderAdapter, strategy RoutingStrategy, m *metrics.Registry) *Router {
	breakers := make(map[string]*CircuitBreaker)
	for _, p := range providers {
		// Default parameters for all providers:
		// Trip after 3 consecutive failures. Stay open for 5 seconds.
		breakers[p.Name()] = NewCircuitBreaker(3, 5*time.Second)
	}
	return &Router{
		Providers: providers,
		Breakers:  breakers,
		Strategy:  strategy,
		Metrics:   m,
	}
}

// Charge attempts to charge using the routing logic.
func (r *Router) Charge(ctx context.Context, req provider.ChargeRequest) (*provider.ChargeResponse, error) {
	// For simplicity, we just loop through the chosen provider, then fallback.
	// A true production system would use a graph of fallbacks.
	
	chosen := r.Strategy.Pick(r.Providers)
	if chosen == nil {
		return nil, provider.ErrProviderDeclined
	}

	// 1. Try chosen provider
	resp, err := r.tryProvider(ctx, chosen, req)
	if err == nil {
		return resp, nil
	}

	// 2. Fallback: try the other providers
	for _, p := range r.Providers {
		if p.Name() == chosen.Name() {
			continue // already tried
		}
		resp, err := r.tryProvider(ctx, p, req)
		if err == nil {
			return resp, nil
		}
	}

	return nil, provider.ErrProviderDeclined // All failed
}

func (r *Router) tryProvider(ctx context.Context, p provider.ProviderAdapter, req provider.ChargeRequest) (*provider.ChargeResponse, error) {
	cb := r.Breakers[p.Name()]
	if !cb.Allow() {
		r.Metrics.Inc("provider_circuit_breaker_blocked_total")
		return nil, provider.ErrProviderTimeout // Circuit is open
	}

	// Bounded retry (max 2 attempts) with backoff
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := p.Charge(ctx, req)
		if err == nil {
			cb.RecordResult(true)
			r.Metrics.Inc("provider_success_total_" + p.Name())
			return resp, nil
		}
		lastErr = err
		cb.RecordResult(false)
		r.Metrics.Inc("provider_failure_total_" + p.Name())
		
		// Backoff before retry
		time.Sleep(100 * time.Millisecond) 
	}
	
	return nil, lastErr
}

// --- The Sliding Window Strategy ---

// SlidingWindowStrategy tracks success rates over a window of time.
type SlidingWindowStrategy struct {
	mu sync.Mutex
	// We'll use a simple fixed-size slice of recent results per provider.
	// true = success, false = failure
	history map[string][]bool
	windowSize int
}

func NewSlidingWindowStrategy(windowSize int) *SlidingWindowStrategy {
	return &SlidingWindowStrategy{
		history:    make(map[string][]bool),
		windowSize: windowSize,
	}
}

func (s *SlidingWindowStrategy) RecordResult(providerName string, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	hist := s.history[providerName]
	hist = append(hist, success)
	if len(hist) > s.windowSize {
		hist = hist[1:] // drop oldest
	}
	s.history[providerName] = hist
}

func (s *SlidingWindowStrategy) Pick(providers []provider.ProviderAdapter) provider.ProviderAdapter {
	s.mu.Lock()
	defer s.mu.Unlock()

	var best provider.ProviderAdapter
	bestRate := -1.0

	for _, p := range providers {
		hist := s.history[p.Name()]
		if len(hist) == 0 {
			if best == nil {
				best = p // Default to first provider if no history
			}
			continue
		}

		successes := 0
		for _, success := range hist {
			if success {
				successes++
			}
		}
		rate := float64(successes) / float64(len(hist))
		if rate > bestRate {
			bestRate = rate
			best = p
		}
	}

	if best == nil && len(providers) > 0 {
		return providers[0]
	}
	return best
}