package connectors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Xendit sandbox adapter. Signature = x-callback-token header equals the
// integration's webhook secret (Xendit's documented callback token scheme).
type Xendit struct{}

func (Xendit) Provider() string { return "xendit" }

func (Xendit) Verify(secret string, header http.Header, body []byte) error {
	got := header.Get("X-Callback-Token")
	if secret == "" || !constantTimeEqual(got, secret) {
		return ErrBadSignature
	}
	return nil
}

var xenditSucceeded = map[string]bool{
	"payment.succeeded": true,
	"payment.completed": true,
	"invoice.paid":      true,
	"payment.capture":   true,
}

func (Xendit) Parse(body []byte) (PaymentEvent, error) {
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			ID          string      `json:"id"`
			Amount      json.Number `json:"amount"`
			Currency    string      `json:"currency"`
			Description string      `json:"description"`
			Status      string      `json:"status"`
			ReferenceID string      `json:"reference_id"`
			Created     string      `json:"created"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return PaymentEvent{}, fmt.Errorf("%w: malformed json", ErrUnknownEvent)
	}
	if payload.Data.ID == "" {
		return PaymentEvent{}, fmt.Errorf("%w: missing data.id", ErrUnknownEvent)
	}
	ev := PaymentEvent{
		EventKey:    payload.Data.ID,
		EventType:   payload.Event,
		Succeeded:   xenditSucceeded[payload.Event] || payload.Data.Status == "SUCCEEDED",
		AmountRaw:   payload.Data.Amount.String(),
		Description: payload.Data.Description,
		Reference:   payload.Data.ReferenceID,
	}
	if t, err := time.Parse(time.RFC3339, payload.Data.Created); err == nil {
		ev.OccurredAt = t
	}
	return ev, nil
}
