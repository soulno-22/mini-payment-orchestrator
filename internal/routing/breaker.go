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

type CircuitBreaker struct {
	mu sync.Mutex

	FailureThreshold int           // consecutive failures to trip
	CooldownPeriod   time.Duration // open -> half-open wait time

	state           BreakerState
	failures        int
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

func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(b.lastStateChange) > b.CooldownPeriod {
			b.state = StateHalfOpen
			b.lastStateChange = time.Now()
			return true // let one probe through
		}
		return false
	default: // StateHalfOpen
		return true
	}
}

func (b *CircuitBreaker) RecordResult(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		b.failures = 0
		b.state = StateClosed
		return
	}

	b.failures++

	// A failed probe in half-open immediately reopens the breaker.
	if b.state == StateHalfOpen || (b.state == StateClosed && b.failures >= b.FailureThreshold) {
		b.state = StateOpen
		b.lastStateChange = time.Now()
	}
}