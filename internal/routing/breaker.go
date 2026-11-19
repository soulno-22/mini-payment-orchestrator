package routing

import (
	"sync"
	"time"
)

type BreakerState int

const (
	StateClosed BreakerState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker stops us from hammering a dead provider.
type CircuitBreaker struct {
	mu sync.Mutex

	// Named parameters (document these in README)
	FailureThreshold int           // Number of consecutive failures to trip
	CooldownPeriod   time.Duration // How long to stay Open before Half-Open

	state         BreakerState
	failures      int
	lastStateChange time.Time
}

func NewCircuitBreaker(failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		FailureThreshold: failureThreshold,
		CooldownPeriod:   cooldown,
		state:            StateClosed,
		lastStateChange:  time.Now(),
	}
}

// Allow returns true if the request can proceed.
func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == StateClosed {
		return true
	}

	if b.state == StateOpen {
		// Check if cooldown has elapsed
		if time.Since(b.lastStateChange) > b.CooldownPeriod {
			b.state = StateHalfOpen
			b.lastStateChange = time.Now()
			return true // Allow the probe request
		}
		return false // Still cooling down
	}

	// Half-Open: Allow only one probe request at a time.
	// In a more advanced version, we'd use weighted probes, but for now
	// we allow one request to test the waters.
	return true 
}

// RecordResult updates the breaker based on the outcome.
func (b *CircuitBreaker) RecordResult(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		// If we succeed in Half-Open or Closed, we reset.
		b.failures = 0
		b.state = StateClosed
		return
	}

	// It failed.
	b.failures++
	
	if b.state == StateHalfOpen {
		// Failed the probe: back to Open immediately.
		b.state = StateOpen
		b.lastStateChange = time.Now()
		return
	}

	if b.state == StateClosed && b.failures >= b.FailureThreshold {
		b.state = StateOpen
		b.lastStateChange = time.Now()
	}
}