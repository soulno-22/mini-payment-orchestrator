package httpapi

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status    string `json:"status"`
	RequestID string `json:"request_id"`
}

func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:    "ok",
			RequestID: RequestIDFrom(r.Context()),
		})
	}
}
