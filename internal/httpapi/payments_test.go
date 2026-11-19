package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
	"github.com/soulno-22/mini-payment-orchestrator/internal/routing"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

func TestIdempotency_ConcurrentRequestsOnlyChargeOnce(t *testing.T) {
	// 1. Setup
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mem := store.NewMemoryStore()
	mockProv := provider.NewMockProvider("mock_a")

	// Wrap the mock in our routing layer
	m := metrics.NewRegistry()
	strategy := routing.NewSlidingWindowStrategy(10)
	router := routing.NewRouter([]provider.ProviderAdapter{mockProv}, strategy, m)
	
	// Pass the router AND metrics to the HTTP layer
	httpRouter := NewRouter(logger, mem, router, m)
	
	srv := httptest.NewServer(httpRouter)
	defer srv.Close()

	// 2. Prepare the payload
	body := createPaymentRequest{AmountCents: 5000, Currency: "INR"}
	bodyBytes, _ := json.Marshal(body)
	key := "test-key-123"

	// 3. Fire 50 concurrent requests
	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	
	responses := make([]int, N)
	replayed := make([]bool, N)

	for i := 0; i < N; i++ {
		go func(index int) {
			defer wg.Done()
			req, _ := http.NewRequest("POST", srv.URL+"/payments", bytes.NewReader(bodyBytes))
			req.Header.Set("Idempotency-Key", key)
			
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("request failed: %v", err)
				return
			}
			defer resp.Body.Close()
			
			responses[index] = resp.StatusCode
			replayed[index] = resp.Header.Get("Idempotent-Replayed") == "true"
		}(i)
	}
	wg.Wait()

	// 4. Assertions
	charges := mockProv.CallCount.Load()
	if charges != 1 {
		t.Fatalf("FAIL: expected exactly 1 charge, got %d", charges)
	}

	for i, code := range responses {
		if code != http.StatusCreated {
			t.Fatalf("request %d got status %d, want 201", i, code)
		}
	}

	var replayCount int
	for _, r := range replayed {
		if r {
			replayCount++
		}
	}
	if replayCount != N-1 {
		t.Fatalf("expected %d replayed responses, got %d", N-1, replayCount)
	}

	fmt.Printf("SUCCESS: 50 concurrent requests -> 1 charge, %d replays\n", replayCount)
}