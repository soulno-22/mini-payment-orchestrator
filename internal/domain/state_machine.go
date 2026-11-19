package domain

import "fmt"

// ErrInvalidTransition is returned when an illegal state change is attempted.
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("domain: illegal transition %s -> %s", e.From, e.To)
}

// CanTransitionTo returns true if moving from the current state to the
// target state is allowed.
func (s State) CanTransitionTo(target State) bool {
	switch s {
	case StateCreated:
		return target == StateAuthorized || target == StateFailed
	case StateAuthorized:
		return target == StateCaptured || target == StateFailed
	case StateCaptured:
		return target == StateRefunded
	case StateFailed, StateRefunded:
		return false // Terminal states: nothing comes after them
	default:
		return false // Unknown state
	}
}

// Transition validates the move and mutates the payment.
// If the transition is illegal, it returns ErrInvalidTransition and
// DOES NOT mutate the payment.
func (p *Payment) Transition(target State) error {
	if !p.State.CanTransitionTo(target) {
		return ErrInvalidTransition{From: p.State, To: target}
	}
	p.State = target
	return nil
}
