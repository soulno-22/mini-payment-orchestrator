package domain

import (
	"errors"
	"testing"
)

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		from    State
		to      State
		allowed bool
	}{
		{StateCreated, StateAuthorized, true},
		{StateCreated, StateFailed, true},
		{StateAuthorized, StateCaptured, true},
		{StateAuthorized, StateFailed, true},
		{StateCaptured, StateRefunded, true},

		{StateCaptured, StateAuthorized, false}, // can't un-capture
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
				return
			}

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
		})
	}
}

// TestEveryPairExhaustive walks all 25 combinations and asserts the
// transition matrix matches. This catches a typo in CanTransitionTo that
// the curated cases above might miss.
func TestEveryPairExhaustive(t *testing.T) {
	states := []State{StateCreated, StateAuthorized, StateCaptured, StateFailed, StateRefunded}

	// Hardcoded ground truth: which from->to pairs are legal.
	legal := map[State]map[State]bool{
		StateCreated:    {StateAuthorized: true, StateFailed: true},
		StateAuthorized: {StateCaptured: true, StateFailed: true},
		StateCaptured:   {StateRefunded: true},
		StateFailed:     {},
		StateRefunded:   {},
	}

	for _, from := range states {
		for _, to := range states {
			want := legal[from][to]
			got := from.CanTransitionTo(to)
			if got != want {
				t.Errorf("%s -> %s: want %v, got %v", from, to, want, got)
			}
		}
	}
}