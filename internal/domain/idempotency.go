package domain

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

type IdempotencyRecord struct {
	Key          string
	MerchantID   string
	RequestHash  string
	ResponseCode int
	ResponseBody []byte
	CreatedAt    time.Time
}

func HashRequest(req any) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h), nil
}
