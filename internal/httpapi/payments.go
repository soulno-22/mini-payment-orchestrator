package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/soulno-22/mini-payment-orchestrator/internal/domain"
	"github.com/soulno-22/mini-payment-orchestrator/internal/metrics"
	"github.com/soulno-22/mini-payment-orchestrator/internal/provider"
	"github.com/soulno-22/mini-payment-orchestrator/internal/routing"
	"github.com/soulno-22/mini-payment-orchestrator/internal/store"
)

type createPaymentRequest struct {
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type createPaymentResponse struct {
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Reference string `json:"reference"`
}

type PaymentHandler struct {
	store    store.Store
	router   *routing.Router
	locker   *KeyLocker
	logger   *slog.Logger
	metrics  *metrics.Registry
}

func NewPaymentHandler(s store.Store, r *routing.Router, l *KeyLocker, logger *slog.Logger, m *metrics.Registry) *PaymentHandler {
	return &PaymentHandler{store: s, router: r, locker: l, logger: logger, metrics: m}
}

func (h *PaymentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqID := RequestIDFrom(ctx)

	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
		return
	}

	merchantID := "merchant_1"
	scopedKey := merchantID + ":" + idemKey

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	var req createPaymentRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	reqHash, err := domain.HashRequest(req)
	if err != nil {
		http.Error(w, "failed to hash request", http.StatusInternalServerError)
		return
	}

	// 3. Fast path: Check if record already exists in DB
	rec, err := h.store.GetIdempotencyKey(ctx, scopedKey)
	if err == nil {
		if rec.RequestHash != reqHash {
			h.logger.Warn("idempotency conflict", "request_id", reqID, "key", scopedKey)
			http.Error(w, "idempotency key already used with different payload", http.StatusConflict)
			return
		}
		// CACHE HIT!
		h.metrics.Inc("idempotency_cache_hits_total") // <-- METRIC
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Idempotent-Replayed", "true")
		w.WriteHeader(rec.ResponseCode)
		w.Write(rec.ResponseBody)
		return
	}

	// 4. Lock the key
	unlock := h.locker.Lock(scopedKey)
	defer unlock()

	// 5. Double-check
	rec, err = h.store.GetIdempotencyKey(ctx, scopedKey)
	if err == nil {
		if rec.RequestHash != reqHash {
			http.Error(w, "idempotency key already used with different payload", http.StatusConflict)
			return
		}
		h.metrics.Inc("idempotency_cache_hits_total") // <-- METRIC
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Idempotent-Replayed", "true")
		w.WriteHeader(rec.ResponseCode)
		w.Write(rec.ResponseBody)
		return
	}

	// 6. Execute the payment.
	paymentID := domain.NewID()
	payment := &domain.Payment{
		ID:          paymentID,
		MerchantID:  merchantID,
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
		State:       domain.StateCreated,
		Version:     1,
	}

	provResp, err := h.router.Charge(ctx, provider.ChargeRequest{
		PaymentID:   paymentID,
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
	})

	var responseCode int
	var responseBody []byte

	if err != nil {
		payment.State = domain.StateFailed
		responseCode = http.StatusPaymentRequired // 402
		responseBody, _ = json.Marshal(map[string]string{"error": err.Error()})
		h.metrics.Inc("payments_failed_total") // <-- METRIC
	} else {
		payment.State = domain.StateAuthorized
		payment.Provider = "routed_provider"
		responseCode = http.StatusCreated // 201
		resp := createPaymentResponse{
			PaymentID: paymentID,
			Status:    "authorized",
			Reference: provResp.ProviderReference,
		}
		responseBody, _ = json.Marshal(resp)
		h.metrics.Inc("payments_created_total") // <-- METRIC
	}

	if err := h.store.Create(ctx, payment); err != nil {
		http.Error(w, "failed to save payment", http.StatusInternalServerError)
		return
	}

	rec = &domain.IdempotencyRecord{
		Key:          scopedKey,
		MerchantID:   merchantID,
		RequestHash:  reqHash,
		ResponseCode: responseCode,
		ResponseBody: responseBody,
		CreatedAt:    time.Now().UTC(),
	}

	_ = h.store.SaveIdempotencyKey(ctx, rec)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(responseCode)
	w.Write(responseBody)
}