package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/httpapi"
	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
	"github.com/soulno-22/mini-payment-orchestrator/internal/routing"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mem := store.NewMemoryStore()
	metricRegistry := metrics.NewRegistry() // <-- NEW

	pA := provider.NewMockProvider("provider_a")
	pB := provider.NewMockProvider("provider_b")

	strategy := routing.NewSlidingWindowStrategy(10)
	router := routing.NewRouter([]provider.ProviderAdapter{pA, pB}, strategy, metricRegistry) // <-- NEW ARG

	httpRouter := httpapi.NewRouter(logger, mem, router, metricRegistry) // <-- NEW ARG

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           httpRouter,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("server starting", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}