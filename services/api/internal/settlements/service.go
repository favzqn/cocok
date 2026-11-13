package settlements

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

type CreateInput struct {
	AccountID           *string `json:"account_id,omitempty"`
	Counterparty        string  `json:"counterparty,omitempty"`
	Reference           string  `json:"reference,omitempty"`
	ExpectedAmountMinor int64   `json:"expected_amount_minor"`
	Currency            string  `json:"currency,omitempty"`
	ExpectedDate        string  `json:"expected_date"` // YYYY-MM-DD
	Notes               string  `json:"notes,omitempty"`
}

type UpdateInput struct {
	AccountID           *string `json:"account_id,omitempty"`
	Counterparty        *string `json:"counterparty,omitempty"`
	Reference           *string `json:"reference,omitempty"`
	ExpectedAmountMinor *int64  `json:"expected_amount_minor,omitempty"`
	ExpectedDate        *string `json:"expected_date,omitempty"`
	Status              *string `json:"status,omitempty"`
	SettledAmountMinor  *int64  `json:"settled_amount_minor,omitempty"`
	Notes               *string `json:"notes,omitempty"`
}

var statuses = map[string]bool{"pending": true, "partial": true, "settled": true, "failed": true}

// Settlement is the API view.
type Settlement struct {
	ID                  string      `json:"id"`
	AccountID           *string     `json:"account_id,omitempty"`
	Counterparty        *string     `json:"counterparty,omitempty"`
	Reference           *string     `json:"reference,omitempty"`
	ExpectedAmountMinor int64       `json:"expected_amount_minor"`
	Currency            string      `json:"currency"`
	ExpectedDate        interface{} `json:"expected_date"`
	Status              string      `json:"status"`
	SettledAmountMinor  int64       `json:"settled_amount_minor"`
	SettledAt           interface{} `json:"settled_at,omitempty"`
	Notes               *string     `json:"notes,omitempty"`
	LinkedGroupID       *string     `json:"linked_group_id,omitempty"`
	CreatedAt           interface{} `json:"created_at"`
}

func toSettlement(s db.Settlement) Settlement {
	out := Settlement{
		ID:                  s.ID.String(),
		ExpectedAmountMinor: s.ExpectedAmountMinor,
		Currency:            s.Currency,
		ExpectedDate:        s.ExpectedDate,
		Status:              s.Status,
		SettledAmountMinor:  s.SettledAmountMinor,
		CreatedAt:           s.CreatedAt,
	}
	if s.AccountID != nil {
		v := s.AccountID.String()
		out.AccountID = &v
	}
	if s.Counterparty.Valid {
		v := s.Counterparty.String
		out.Counterparty = &v
	}
	if s.Reference.Valid {
		v := s.Reference.String
		out.Reference = &v
	}
	if s.Notes.Valid {
		v := s.Notes.String
		out.Notes = &v
	}
	if s.SettledAt != nil {
		out.SettledAt = *s.SettledAt
	}
	if s.LinkedGroupID != nil {
		v := s.LinkedGroupID.String()
		out.LinkedGroupID = &v
	}
	return out
}

func (s *Service) Create(ctx context.Context, orgID, actorID uuid.UUID, in CreateInput) (*Settlement, error) {
	if in.ExpectedAmountMinor <= 0 {
		return nil, fmt.Errorf("%w: expected_amount_minor must be > 0", platform.ErrValidation)
	}
	date, err := time.Parse("2006-01-02", strings.TrimSpace(in.ExpectedDate))
	if err != nil {
		return nil, fmt.Errorf("%w: expected_date must be YYYY-MM-DD", platform.ErrValidation)
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if cur == "" {
		cur = "IDR"
	}
	if len(cur) != 3 {
		return nil, fmt.Errorf("%w: currency must be a 3-letter code", platform.ErrValidation)
	}
	accID, err := optUUID(in.AccountID)
	if err != nil {
		return nil, err
	}
	row, err := s.queries.CreateSettlement(ctx, db.CreateSettlementParams{
		OrganizationID:      orgID,
		AccountID:           accID,
		Counterparty:        optText(in.Counterparty),
		Reference:           optText(in.Reference),
		ExpectedAmountMinor: in.ExpectedAmountMinor,
		Currency:            cur,
		ExpectedDate:        date,
		Notes:               optText(in.Notes),
		CreatedBy:           &actorID,
	})
	if err != nil {
		return nil, err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "settlement.created", EntityType: "settlement", EntityID: &row.ID,
		NewState: map[string]any{"amount": row.ExpectedAmountMinor, "date": row.ExpectedDate},
	})
	out := toSettlement(row)
	return &out, nil
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID, status string) ([]Settlement, error) {
	rows, err := s.queries.ListSettlements(ctx, db.ListSettlementsParams{
		OrganizationID: orgID, Status: optText(status),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Settlement, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSettlement(r))
	}
	return out, nil
}

// Update patches a settlement (manual status changes included).
func (s *Service) Update(ctx context.Context, orgID, actorID, id uuid.UUID, in UpdateInput) (*Settlement, error) {
	prev, err := s.queries.GetSettlement(ctx, db.GetSettlementParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return nil, platform.ErrNotFound
	}
	next := prev
	if in.AccountID != nil {
		accID, err := optUUID(in.AccountID)
		if err != nil {
			return nil, err
		}
		next.AccountID = accID
	}
	if in.Counterparty != nil {
		next.Counterparty = optText(*in.Counterparty)
	}
	if in.Reference != nil {
		next.Reference = optText(*in.Reference)
	}
	if in.ExpectedAmountMinor != nil {
		if *in.ExpectedAmountMinor <= 0 {
			return nil, fmt.Errorf("%w: expected_amount_minor must be > 0", platform.ErrValidation)
		}
		next.ExpectedAmountMinor = *in.ExpectedAmountMinor
	}
	if in.ExpectedDate != nil {
		d, err := time.Parse("2006-01-02", strings.TrimSpace(*in.ExpectedDate))
		if err != nil {
			return nil, fmt.Errorf("%w: expected_date must be YYYY-MM-DD", platform.ErrValidation)
		}
		next.ExpectedDate = d
	}
	if in.Status != nil {
		if !statuses[*in.Status] {
			return nil, fmt.Errorf("%w: unknown status", platform.ErrValidation)
		}
		next.Status = *in.Status
	}
	if in.SettledAmountMinor != nil {
		if *in.SettledAmountMinor < 0 {
			return nil, fmt.Errorf("%w: settled_amount_minor must be >= 0", platform.ErrValidation)
		}
		next.SettledAmountMinor = *in.SettledAmountMinor
	}
	if next.Status == "settled" && next.SettledAmountMinor == 0 {
		next.SettledAmountMinor = next.ExpectedAmountMinor
	}
	if next.Status == "settled" && next.SettledAt == nil {
		t := time.Now()
		next.SettledAt = &t
	}
	if next.Status != "settled" {
		next.SettledAt = nil
	}
	if in.Notes != nil {
		next.Notes = optText(*in.Notes)
	}
	if err := s.queries.UpdateSettlement(ctx, db.UpdateSettlementParams{
		OrganizationID:      orgID,
		ID:                  id,
		AccountID:           next.AccountID,
		Counterparty:        next.Counterparty,
		Reference:           next.Reference,
		ExpectedAmountMinor: next.ExpectedAmountMinor,
		Currency:            next.Currency,
		ExpectedDate:        next.ExpectedDate,
		Status:              next.Status,
		SettledAmountMinor:  next.SettledAmountMinor,
		SettledAt:           next.SettledAt,
		Notes:               next.Notes,
		LinkedGroupID:       next.LinkedGroupID,
	}); err != nil {
		return nil, err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "settlement.updated", EntityType: "settlement", EntityID: &id,
		PreviousState: map[string]any{"status": prev.Status, "settled_amount": prev.SettledAmountMinor},
		NewState:      map[string]any{"status": next.Status, "settled_amount": next.SettledAmountMinor},
	})
	out := toSettlement(next)
	return &out, nil
}

func (s *Service) Delete(ctx context.Context, orgID, actorID, id uuid.UUID) error {
	prev, err := s.queries.GetSettlement(ctx, db.GetSettlementParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return platform.ErrNotFound
	}
	if err := s.queries.DeleteSettlement(ctx, db.DeleteSettlementParams{OrganizationID: orgID, ID: id}); err != nil {
		return err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "settlement.deleted", EntityType: "settlement", EntityID: &id,
		PreviousState: map[string]any{"status": prev.Status, "amount": prev.ExpectedAmountMinor},
	})
	return nil
}
