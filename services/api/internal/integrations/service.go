package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDuplicate = errors.New("provider integration already exists")
	providers    = map[string]bool{"xendit": true, "midtrans": true}
)

type Service struct {
	queries    *db.Queries
	pool       *pgxpool.Pool
	audit      *audit.Writer
	webBaseURL string
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer, webBaseURL string) *Service {
	return &Service{queries: q, pool: pool, audit: a, webBaseURL: webBaseURL}
}

type CreateInput struct {
	Provider         string  `json:"provider"`
	WebhookSecret    string  `json:"webhook_secret"`
	DefaultAccountID *string `json:"default_account_id,omitempty"`
}

type Integration struct {
	ID               string  `json:"id"`
	Provider         string  `json:"provider"`
	Mode             string  `json:"mode"`
	DefaultAccountID *string `json:"default_account_id,omitempty"`
	Status           string  `json:"status"`
	WebhookURL       string  `json:"webhook_url"`
	CreatedAt        any     `json:"created_at"`
}

func (s *Service) Create(ctx context.Context, orgID, actorID uuid.UUID, in CreateInput) (*Integration, error) {
	prov := strings.ToLower(strings.TrimSpace(in.Provider))
	if !providers[prov] {
		return nil, fmt.Errorf("%w: provider must be xendit or midtrans", platform.ErrValidation)
	}
	secret := strings.TrimSpace(in.WebhookSecret)
	if len(secret) < 8 {
		return nil, fmt.Errorf("%w: webhook_secret must be at least 8 characters", platform.ErrValidation)
	}
	accID, err := optUUID(in.DefaultAccountID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid default_account_id", platform.ErrValidation)
	}
	row, err := s.queries.CreateIntegration(ctx, db.CreateIntegrationParams{
		OrganizationID:   orgID,
		Provider:         prov,
		WebhookSecret:    secret,
		DefaultAccountID: accID,
		CreatedBy:        &actorID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "integration.created", EntityType: "integration", EntityID: &row.ID,
		NewState: map[string]any{"provider": prov, "mode": row.Mode},
	})
	out := s.toIntegration(row)
	return &out, nil
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID) ([]Integration, error) {
	rows, err := s.queries.ListIntegrations(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Integration, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.toIntegration(r))
	}
	return out, nil
}

func (s *Service) Disable(ctx context.Context, orgID, actorID, id uuid.UUID) error {
	prev, err := s.queries.GetIntegration(ctx, db.GetIntegrationParams{OrganizationID: orgID, ID: id})
	if err != nil {
		return platform.ErrNotFound
	}
	if err := s.queries.DisableIntegration(ctx, db.DisableIntegrationParams{
		OrganizationID: orgID, ID: id,
	}); err != nil {
		return err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "integration.disabled", EntityType: "integration", EntityID: &id,
		PreviousState: map[string]any{"status": prev.Status},
	})
	return nil
}

func (s *Service) toIntegration(r db.Integration) Integration {
	out := Integration{
		ID:         r.ID.String(),
		Provider:   r.Provider,
		Mode:       r.Mode,
		Status:     r.Status,
		WebhookURL: fmt.Sprintf("%s/api/v1/webhooks/%s/%s", s.webBaseURL, r.Provider, r.ID),
		CreatedAt:  r.CreatedAt,
	}
	if r.DefaultAccountID != nil {
		v := r.DefaultAccountID.String()
		out.DefaultAccountID = &v
	}
	return out
}

func optUUID(s *string) (*uuid.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key")
}
