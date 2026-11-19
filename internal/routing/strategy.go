package routing

import (
	"context"
	"sync"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
)

// RoutingStrategy is the extension point for picking a provider.
// Want geo-routing? Lowest-cost routing? Write a new struct that
// implements Pick and pass it to NewRouter. The router itself stays dumb.
type RoutingStrategy interface {
	Pick(providers []provider.ProviderAdapter) provider.ProviderAdapter
}

// Router handles retries, fallbacks, and circuit breaking.
type Router struct {
	Providers []provider.ProviderAdapter
	Breakers  map[string]*CircuitBreaker
	Strategy  RoutingStrategy
	Metrics   *metrics.Registry
}

func NewRouter(providers []provider.ProviderAdapter, strategy RoutingStrategy, m *metrics.Registry) *Router {
	breakers := make(map[string]*CircuitBreaker)
	for _, p := range providers {
		// Trip after 3 consecutive failures, stay open for 5s.
		// Hardcoded for now; a real system would read this from config.
		breakers[p.Name()] = NewCircuitBreaker(3, 5*time.Second)
	}
	return &Router{
		Providers: providers,
		Breakers:  breakers,
		Strategy:  strategy,
		Metrics:   m,
	}
}

// Charge tries the preferred provider first, then falls back to the rest.
// The fallback loop is intentionally flat (not a graph) — enough to prove
// the concept without a routing DSL.
func (r *Router) Charge(ctx context.Context, req provider.ChargeRequest) (*provider.ChargeResponse, error) {
	chosen := r.Strategy.Pick(r.Providers)
	if chosen == nil {
		return nil, provider.ErrProviderDeclined
	}

	resp, err := r.tryProvider(ctx, chosen, req)
	if err == nil {
		return resp, nil
	}

	for _, p := range r.Providers {
		if p.Name() == chosen.Name() {
			continue
		}
		resp, err := r.tryProvider(ctx, p, req)
		if err == nil {
			return resp, nil
		}
	}

	return nil, provider.ErrProviderDeclined
}

// tryProvider is where the circuit breaker + retry policy actually live.
// Note: the breaker records failures per-attempt, which means one bad
// payment can trip it faster than expected. That's a deliberate choice —
// we'd rather be too conservative than hammer a dead provider.
func (r *Router) tryProvider(ctx context.Context, p provider.ProviderAdapter, req provider.ChargeRequest) (*provider.ChargeResponse, error) {
	cb := r.Breakers[p.Name()]
	if !cb.Allow() {
		r.Metrics.Inc("provider_circuit_breaker_blocked_total")
		return nil, provider.ErrProviderTimeout
	}

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

		time.Sleep(100 * time.Millisecond)
	}

	return nil, lastErr
}

// SlidingWindowStrategy picks the provider with the highest recent
// success rate. It's intentionally biased toward the current leader —
// there's no exploration (no "give the underdog a chance"). In production
// you'd swap this for Thompson sampling or epsilon-greedy.
type SlidingWindowStrategy struct {
	mu         sync.Mutex
	history    map[string][]bool
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
		hist = hist[1:] // drop the oldest result
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
				best = p // no data yet, just pick the first one
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