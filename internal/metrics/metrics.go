package metrics

import (
	"fmt"
	"net/http"
	"sync"
)

// Registry holds all our counters.
type Registry struct {
	mu sync.RWMutex
	counters map[string]int64
}

func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]int64),
	}
}

func (r *Registry) Inc(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[name]++
}

func (r *Registry) Add(name string, val int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[name] += val
}

// Handler exposes the metrics in Prometheus text format.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		defer r.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for k, v := range r.counters {
			// Prometheus text format: metric_name value
			fmt.Fprintf(w, "%s %d\n", k, v)
		}
	}
}