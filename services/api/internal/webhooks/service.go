package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/connectors"
	"github.com/cocok/cocok/services/api/internal/imports"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound    = errors.New("integration not found")
	ErrDuplicateEv = errors.New("duplicate event")
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

// Result tells the handler what HTTP status/body to return. Providers must
// receive 200 for duplicates too (stops retries).
type Result struct {
	Status    int    `json:"-"`
	Message   string `json:"message"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// Handle verifies signature, dedupes by (provider, event_key), and mints a
// credit transaction for succeeded payments on the integration's default account.
func (s *Service) Handle(ctx context.Context, provider, integrationIDStr string, header http.Header, body []byte) (*Result, error) {
	integrationID, err := uuid.Parse(integrationIDStr)
	if err != nil {
		return nil, ErrNotFound
	}
	adapter, err := connectors.For(provider)
	if err != nil {
		return nil, ErrNotFound
	}
	intg, err := s.queries.GetIntegrationByID(ctx, integrationID)
	if err != nil || intg.Provider != provider {
		return nil, ErrNotFound
	}
	if err := adapter.Verify(intg.WebhookSecret, header, body); err != nil {
		return nil, err // ErrBadSignature → 401
	}
	event, err := adapter.Parse(body)
	if err != nil {
		return nil, err // ErrUnknownEvent → 400
	}

	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, err
	}
	booked := event.OccurredAt.In(loc)
	if booked.IsZero() {
		booked = time.Now().In(loc)
	}

	var txID *uuid.UUID
	status := "ignored"
	reason := ""
	if event.Succeeded {
		if intg.DefaultAccountID == nil {
			reason = "no default account configured"
		} else {
			acc, aerr := s.queries.GetFinancialAccount(ctx, db.GetFinancialAccountParams{
				OrganizationID: intg.OrganizationID, ID: *intg.DefaultAccountID,
			})
			if aerr != nil {
				reason = "default account missing"
			} else {
				amount, perr := platform.ParseAmount(event.AmountRaw, acc.Currency)
				if perr != nil || amount == 0 {
					reason = "invalid amount"
				} else {
					id, ierr := s.mintTransaction(ctx, intg, acc, event, booked, amount)
					if ierr != nil {
						return nil, ierr
					}
					if id != nil {
						txID = id
						status = "processed"
					} else {
						status = "processed"
						reason = "already exists"
					}
				}
			}
		}
	} else {
		reason = "event not a success"
	}

	payload, _ := json.Marshal(json.RawMessage(body))
	n, err := s.queries.InsertWebhookEvent(ctx, db.InsertWebhookEventParams{
		IntegrationID:  intg.ID,
		OrganizationID: intg.OrganizationID,
		Provider:       provider,
		EventKey:       event.EventKey,
		EventType:      event.EventType,
		Payload:        payload,
		Status:         status,
		Reason:         pgtype.Text{String: reason, Valid: reason != ""},
		TransactionID:  txID,
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		// Dedup: event already seen — mint already guarded by fingerprint too.
		return &Result{Status: http.StatusOK, Message: "duplicate", Duplicate: true}, nil
	}

	orgID := intg.OrganizationID
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: uuid.Nil,
		Action: "webhook." + status, EntityType: "webhook_event",
		NewState: map[string]any{
			"provider": provider, "event_key": event.EventKey, "status": status, "reason": reason,
		},
	})
	return &Result{Status: http.StatusOK, Message: status}, nil
}

// mintTransaction inserts a credit canonical tx; returns nil id when the
// fingerprint already exists (import dedup double-guard).
func (s *Service) mintTransaction(ctx context.Context, intg db.Integration, acc db.FinancialAccount, event connectors.PaymentEvent, booked time.Time, amountMinor int64) (*uuid.UUID, error) {
	row := imports.ParsedRow{
		BookedAt:    booked,
		Direction:   "credit",
		AmountMinor: amountMinor,
		Description: event.Description,
		ExternalRef: event.Reference,
	}
	if row.ExternalRef == "" {
		row.ExternalRef = event.EventKey
	}
	if row.Description == "" {
		row.Description = fmt.Sprintf("%s payment", intg.Provider)
	}
	fp := imports.Fingerprint(row)
	raw, _ := json.Marshal(map[string]any{
		"provider": intg.Provider, "event_key": event.EventKey, "event_type": event.EventType,
	})
	bID := intg.ID
	_ = bID
	n, err := s.queries.InsertTransaction(ctx, db.InsertTransactionParams{
		OrganizationID: intg.OrganizationID,
		AccountID:      acc.ID,
		ImportBatchID:  nil,
		BookedAt:       booked,
		Direction:      "credit",
		AmountMinor:    amountMinor,
		Currency:       acc.Currency,
		Description:    row.Description,
		Counterparty:   pgtype.Text{},
		ExternalRef:    pgtype.Text{String: row.ExternalRef, Valid: true},
		Fingerprint:    fp,
		SourceRow:      0, // not from a CSV row
		RawRow:         raw,
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	// Re-select to get the id (InsertTransaction is execrows).
	return s.txIDByFingerprint(ctx, intg.OrganizationID, acc.ID, fp)
}

func (s *Service) txIDByFingerprint(ctx context.Context, orgID, accID uuid.UUID, fp string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM transactions
		WHERE organization_id = $1 AND account_id = $2 AND fingerprint = $3`,
		orgID, accID, fp).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &id, nil
}
