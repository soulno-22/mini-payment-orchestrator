package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
	"github.com/soulno-22/mini-payment-orchestrator/internal/routing"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

func NewRouter(logger *slog.Logger, s store.Store, r *routing.Router, m *metrics.Registry) http.Handler {
	mux := http.NewServeMux()

	locker := NewKeyLocker()
	paymentsHandler := NewPaymentHandler(s, r, locker, logger, m)
	webhooksHandler := NewWebhookHandler(s, logger)

	mux.HandleFunc("GET /health", HealthHandler())
	mux.Handle("POST /payments", paymentsHandler)
	mux.Handle("POST /webhooks/{provider}", webhooksHandler)
	mux.HandleFunc("GET /metrics", m.Handler())

	return RequestID(Logging(logger)(mux))
}