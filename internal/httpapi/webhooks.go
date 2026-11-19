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
	EventID   string      `json:"event_id"`
	PaymentID string      `json:"payment_id"`
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

	// 1. Signature verification (Stub)
	// In production, we would read the signature header, compute HMAC-SHA256 
	// of the body using our shared secret, and compare them.
	// For this project, we'll just accept it.
	
	// 2. Read Body
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

	// 3. Dedupe by Event ID
	if err := h.store.SaveWebhookEvent(ctx, payload.EventID); err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			// Already processed. Return 200 so the provider stops retrying.
			h.logger.Info("webhook duplicate ignored", "request_id", reqID, "event_id", payload.EventID)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "failed to save event", http.StatusInternalServerError)
		return
	}

	// 4. Fetch the payment to get its current state
	payment, err := h.store.Get(ctx, payload.PaymentID)
	if err != nil {
		http.Error(w, "payment not found", http.StatusNotFound)
		return
	}

	// 5. Attempt the state transition atomically
	_, err = h.store.Update(ctx, payment.ID, payment.Version, func(p *domain.Payment) (*domain.Payment, error) {
		// We use the pure function from Phase 2!
		if err := p.Transition(payload.NewState); err != nil {
			return nil, err // illegal transition
		}
		return p, nil
	})

	if err != nil {
		var invalidTransition domain.ErrInvalidTransition
		if errors.As(err, &invalidTransition) {
			// Stale or out-of-order webhook (e.g., CAPTURED arrived after REFUNDED).
			// We ignore it and return 200, because retrying won't help.
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
			// We lost the optimistic lock race. Provider should retry.
			http.Error(w, "version conflict, try again", http.StatusConflict)
			return
		}
		
		http.Error(w, "failed to update payment", http.StatusInternalServerError)
		return
	}

	h.logger.Info("webhook processed", "request_id", reqID, "event_id", payload.EventID)
	w.WriteHeader(http.StatusOK)
}