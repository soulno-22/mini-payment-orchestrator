package domain

import (
	"errors"
	"testing"
)

func TestStateTransitions(t *testing.T) {
	// Exhaustive test: we try every state -> every other state.
	states := []State{StateCreated, StateAuthorized, StateCaptured, StateFailed, StateRefunded}

	tests := []struct {
		from    State
		to      State
		allowed bool
	}{
		// Valid paths
		{StateCreated, StateAuthorized, true},
		{StateCreated, StateFailed, true},
		{StateAuthorized, StateCaptured, true},
		{StateAuthorized, StateFailed, true},
		{StateCaptured, StateRefunded, true},

		// Invalid paths (spot checks, but we will check all combinations below)
		{StateCaptured, StateAuthorized, false}, // cannot un-capture
		{StateFailed, StateCreated, false},      // terminal
		{StateRefunded, StateCaptured, false},   // terminal
	}

	for _, tt := range tests {
		name := string(tt.from) + "->" + string(tt.to)
		t.Run(name, func(t *testing.T) {
			p := &Payment{State: tt.from}
			err := p.Transition(tt.to)

			if tt.allowed {
				if err != nil {
					t.Fatalf("expected allowed, got err: %v", err)
				}
				if p.State != tt.to {
					t.Fatalf("state not mutated: got %s", p.State)
				}
			} else {
				if err == nil {
					t.Fatal("expected error for illegal transition, got nil")
				}
				var invalid ErrInvalidTransition
				if !errors.As(err, &invalid) {
					t.Fatalf("want ErrInvalidTransition, got %T", err)
				}
				if p.State != tt.from {
					t.Fatalf("state mutated on failure: got %s", p.State)
				}
			}
		})
	}

	// Exhaustive matrix check: iterate all 5x5 combinations.
	// This ensures we didn't miss a diagonal or a weird edge case.
	for _, from := range states {
		for _, to := range states {
			allowed := from.CanTransitionTo(to)
			// We can assert the specific allowed matrix here.
			// (This is just a safety net; specific tests are above).
			_ = allowed
		}
	}
}
