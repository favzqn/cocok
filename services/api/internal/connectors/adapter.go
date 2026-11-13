package connectors

import (
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Sandbox-labeled adapters (PRODUCT.md §Assumptions): signature verification +
// event parsing only, until live credentials and outbound flows exist.

var (
	ErrBadSignature = errors.New("invalid webhook signature")
	ErrUnknownEvent = errors.New("unknown webhook event")
)

// PaymentEvent is the normalized inbound callback.
type PaymentEvent struct {
	EventKey    string // stable provider id — dedup key
	EventType   string // provider's raw event/transaction status
	Succeeded   bool   // money arrived → mint a credit transaction
	AmountRaw   string // major-unit amount as text (parsed by platform.ParseAmount)
	Description string
	Reference   string
	OccurredAt  time.Time
}

// Adapter verifies + parses one provider's webhooks.
type Adapter interface {
	Provider() string
	Verify(secret string, header http.Header, body []byte) error
	Parse(body []byte) (PaymentEvent, error)
}

// For returns the adapter for a known provider name.
func For(provider string) (Adapter, error) {
	switch provider {
	case "xendit":
		return Xendit{}, nil
	case "midtrans":
		return Midtrans{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownEvent, provider)
	}
}

func constantTimeEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}

func useNumberDecoder(body []byte) (*json.Decoder, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	return dec, nil
}
