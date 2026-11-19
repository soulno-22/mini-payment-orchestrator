package httpapi

import (
	"sync"
)

// KeyLocker ensures that for a given idempotency key, only one goroutine
// can execute the charge at a time. Others block until it's done.
type KeyLocker struct {
	mu sync.Mutex
	// map[key] -> chan struct{}
	locks map[string]chan struct{}
}

func NewKeyLocker() *KeyLocker {
	return &KeyLocker{locks: make(map[string]chan struct{})}
}

// Lock blocks until the key is free, then locks it.
// It returns a function to unlock it.
func (l *KeyLocker) Lock(key string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]chan struct{})
	}

	ch, exists := l.locks[key]
	if !exists {
		ch = make(chan struct{}, 1)
		l.locks[key] = ch
	}
	l.mu.Unlock()

	ch <- struct{}{} // Acquire

	return func() {
		<-ch // Release
		l.mu.Lock()
		l.mu.Unlock()
	}
}
