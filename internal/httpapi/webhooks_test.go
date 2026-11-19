package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

func TestWebhook_DedupeAndStaleTransitions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mem := store.NewMemoryStore()
	
	// Setup a payment directly in the store
	payment := &domain.Payment{
		ID:          "pay_123",
		MerchantID:  "merchant_1",
		AmountCents: 1000,
		Currency:    "INR",
		State:       domain.StateAuthorized, // Currently Authorized
	}
	_ = mem.Create(context.Background(), payment)

	handler := NewWebhookHandler(mem, logger)
	
	// 1. Process a valid CAPTURED webhook
	payload := webhookPayload{
		EventID:   "evt_1",
		PaymentID: "pay_123",
		NewState:  domain.StateCaptured,
	}
	body, _ := json.Marshal(payload)
	
	req := httptest.NewRequest("POST", "/webhooks/provider_a", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	
	// Verify the payment is now Captured
	updated, _ := mem.Get(context.Background(), "pay_123")
	if updated.State != domain.StateCaptured {
		t.Fatalf("expected CAPTURED, got %s", updated.State)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}

	// 2. Send the EXACT same webhook again (Dedupe test)
	req = httptest.NewRequest("POST", "/webhooks/provider_a", bytes.NewReader(body))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for dedupe, got %d", w.Code)
	}
	// Version should NOT increment because it was deduped
	updated, _ = mem.Get(context.Background(), "pay_123")
	if updated.Version != 2 {
		t.Fatalf("expected version 2 after dedupe, got %d", updated.Version)
	}

	// 3. Send a stale/out-of-order webhook: "AUTHORIZED" arrives late
	// But wait, the current state is now CAPTURED. We can't go CAPTURED -> AUTHORIZED.
	stalePayload := webhookPayload{
		EventID:   "evt_2",
		PaymentID: "pay_123",
		NewState:  domain.StateAuthorized, // Stale state
	}
	staleBody, _ := json.Marshal(stalePayload)
	req = httptest.NewRequest("POST", "/webhooks/provider_a", bytes.NewReader(staleBody))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// It should return 200 OK (to stop retries) but NOT change the state.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for stale webhook, got %d", w.Code)
	}
	updated, _ = mem.Get(context.Background(), "pay_123")
	if updated.State != domain.StateCaptured {
		t.Fatalf("expected state to remain CAPTURED, got %s", updated.State)
	}
}