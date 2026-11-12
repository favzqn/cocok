package accounts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var kinds = map[string]bool{
	"bank": true, "payment_gateway": true, "e_wallet": true, "marketplace": true, "other": true,
}

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

type CreateInput struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	AccountNumber string `json:"account_number,omitempty"`
	Currency      string `json:"currency,omitempty"`
}

type UpdateInput struct {
	Name   *string `json:"name,omitempty"`
	Kind   *string `json:"kind,omitempty"`
	Status *string `json:"status,omitempty"`
}

func (s *Service) Create(ctx context.Context, orgID, actorID uuid.UUID, in CreateInput) (*db.FinancialAccount, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", platform.ErrValidation)
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = "bank"
	}
	if !kinds[kind] {
		return nil, fmt.Errorf("%w: kind must be one of bank, payment_gateway, e_wallet, marketplace, other", platform.ErrValidation)
	}
	cur := strings.ToUpper(strings.TrimSpace(in.Currency))
	if cur == "" {
		cur = "IDR"
	}
	if len(cur) != 3 {
		return nil, fmt.Errorf("%w: currency must be a 3-letter code", platform.ErrValidation)
	}
	acc, err := s.queries.CreateFinancialAccount(ctx, db.CreateFinancialAccountParams{
		OrganizationID: orgID,
		Name:           name,
		Kind:           kind,
		AccountNumber:  text(in.AccountNumber),
		Currency:       cur,
		CreatedBy:      &actorID,
	})
	if err != nil {
		return nil, err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "account.created", EntityType: "financial_account", EntityID: &acc.ID,
		NewState: map[string]any{"name": acc.Name, "kind": acc.Kind, "currency": acc.Currency},
	})
	return &acc, nil
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]db.FinancialAccount, error) {
	return s.queries.ListFinancialAccounts(ctx, orgID)
}

// Get returns the account or platform.ErrNotFound (cross-org → 404 upstream).
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*db.FinancialAccount, error) {
	acc, err := s.queries.GetFinancialAccount(ctx, db.GetFinancialAccountParams{
		OrganizationID: orgID, ID: id,
	})
	if err != nil {
		return nil, platform.ErrNotFound
	}
	return &acc, nil
}

func (s *Service) Update(ctx context.Context, orgID, actorID, id uuid.UUID, in UpdateInput) (*db.FinancialAccount, error) {
	prev, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	name := prev.Name
	if in.Name != nil {
		name = strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name cannot be empty", platform.ErrValidation)
		}
	}
	kind := prev.Kind
	if in.Kind != nil {
		kind = strings.TrimSpace(*in.Kind)
		if !kinds[kind] {
			return nil, fmt.Errorf("%w: unknown kind", platform.ErrValidation)
		}
	}
	status := prev.Status
	if in.Status != nil {
		status = strings.TrimSpace(*in.Status)
		if status != "active" && status != "archived" {
			return nil, fmt.Errorf("%w: status must be active or archived", platform.ErrValidation)
		}
	}
	if err := s.queries.UpdateFinancialAccount(ctx, db.UpdateFinancialAccountParams{
		OrganizationID: orgID, ID: id, Name: name, Kind: kind, Status: status,
	}); err != nil {
		return nil, err
	}
	next, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "account.updated", EntityType: "financial_account", EntityID: &id,
		PreviousState: map[string]any{"name": prev.Name, "kind": prev.Kind, "status": prev.Status},
		NewState:      map[string]any{"name": next.Name, "kind": next.Kind, "status": next.Status},
	})
	return next, nil
}

// ErrNotFound re-export for handlers.
var ErrNotFound = errors.New("not found")

func text(s string) pgtype.Text {
	t := strings.TrimSpace(s)
	return pgtype.Text{String: t, Valid: t != ""}
}
