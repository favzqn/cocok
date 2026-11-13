package connectors

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Midtrans sandbox adapter. Signature = hex(sha512(order_id + status_code +
// gross_amount + server_key)) in the x-signature header (Midtrans docs).
type Midtrans struct{}

func (Midtrans) Provider() string { return "midtrans" }

func (Midtrans) Verify(secret string, header http.Header, body []byte) error {
	var payload struct {
		OrderID     string      `json:"order_id"`
		StatusCode  string      `json:"status_code"`
		GrossAmount json.Number `json:"gross_amount"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ErrBadSignature
	}
	want := payload.OrderID + payload.StatusCode + payload.GrossAmount.String() + secret
	sum := sha512.Sum512([]byte(want))
	expected := hex.EncodeToString(sum[:])
	got := strings.ToLower(strings.TrimSpace(header.Get("X-Signature")))
	if secret == "" || got == "" || !constantTimeEqual(got, expected) {
		return ErrBadSignature
	}
	return nil
}

func (Midtrans) Parse(body []byte) (PaymentEvent, error) {
	var payload struct {
		OrderID           string      `json:"order_id"`
		TransactionStatus string      `json:"transaction_status"`
		GrossAmount       json.Number `json:"gross_amount"`
		FraudStatus       string      `json:"fraud_status"`
		TransactionTime   string      `json:"transaction_time"`
		CustomerName      string      `json:"customer_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return PaymentEvent{}, fmt.Errorf("%w: malformed json", ErrUnknownEvent)
	}
	if payload.OrderID == "" {
		return PaymentEvent{}, fmt.Errorf("%w: missing order_id", ErrUnknownEvent)
	}
	succeeded := payload.TransactionStatus == "settlement" ||
		(payload.TransactionStatus == "capture" && payload.FraudStatus != "deny")
	ev := PaymentEvent{
		EventKey:    payload.OrderID,
		EventType:   payload.TransactionStatus,
		Succeeded:   succeeded,
		AmountRaw:   payload.GrossAmount.String(),
		Description: payload.CustomerName,
		Reference:   payload.OrderID,
	}
	if t, err := time.Parse("2006-01-02 15:04:05", payload.TransactionTime); err == nil {
		ev.OccurredAt = t
	}
	return ev, nil
}
