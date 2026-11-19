package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

type webhookPayload struct {
	EventID   string       `json:"event_id"`
	PaymentID string       `json:"payment_id"`
	NewState  domain.State `json:"new_state"`
}

type WebhookHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewWebhookHandler(s store.Store, logger *slog.Logger) *WebhookHandler {
	return &WebhookHandler{store: s, logger: logger}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqID := RequestIDFrom(ctx)

	// Signature verification is stubbed. In production we'd compute an
	// HMAC-SHA256 of the body and compare it against the provider's header.
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var payload webhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Dedupe by event ID. Providers retry webhooks up to 5 times, so the
	// first delivery does the work and every subsequent one is a no-op.
	if err := h.store.SaveWebhookEvent(ctx, payload.EventID); err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			h.logger.Info("webhook duplicate ignored", "request_id", reqID, "event_id", payload.EventID)
			w.WriteHeader(http.StatusOK) // 200 so the provider stops retrying
			return
		}
		http.Error(w, "failed to save event", http.StatusInternalServerError)
		return
	}

	payment, err := h.store.Get(ctx, payload.PaymentID)
	if err != nil {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	_, err = h.store.Update(ctx, payment.ID, payment.Version, func(p *domain.Payment) (*domain.Payment, error) {
		if err := p.Transition(payload.NewState); err != nil {
			return nil, err
		}
		return p, nil
	})

	if err != nil {
		var invalidTransition domain.ErrInvalidTransition
		if errors.As(err, &invalidTransition) {
			// Stale/out-of-order webhook (e.g., CAPTURED arrived after
			// REFUNDED). Return 200 so the provider stops retrying, but
			// don't touch state.
			h.logger.Warn("stale webhook rejected",
				"request_id", reqID,
				"event_id", payload.EventID,
				"from", invalidTransition.From,
				"to", invalidTransition.To,
			)
			w.WriteHeader(http.StatusOK)
			return
		}

		if errors.Is(err, store.ErrVersionConflict) {
			// Lost the optimistic lock race. Retry is safe here.
			http.Error(w, "version conflict, try again", http.StatusConflict)
			return
		}

		http.Error(w, "failed to update payment", http.StatusInternalServerError)
		return
	}

	h.logger.Info("webhook processed", "request_id", reqID, "event_id", payload.EventID)
	w.WriteHeader(http.StatusOK)
}