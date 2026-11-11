package organizations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotMember = errors.New("not a member")
	ErrSlugTaken = errors.New("slug already taken")
	ErrLastOwner = errors.New("cannot remove or demote the last owner")
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   AuditWriter
}

// AuditWriter is the consumer-side interface (avoids import cycle with audit pkg).
type AuditWriter interface {
	Write(ctx context.Context, e AuditEvent)
}

type AuditEvent struct {
	OrganizationID *uuid.UUID
	ActorID        uuid.UUID
	Action         string
	EntityType     string
	EntityID       *uuid.UUID
	PreviousState  map[string]any
	NewState       map[string]any
	Reason         string
	Metadata       map[string]any
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a AuditWriter) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

type CreateOrgInput struct {
	Name         string `json:"name"`
	BusinessType string `json:"business_type"`
	Slug         string `json:"slug,omitempty"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// CreateWithOwner creates the org and the owner membership in one tx.
func (s *Service) CreateWithOwner(ctx context.Context, actorID uuid.UUID, in CreateOrgInput) (*db.Organization, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", platform.ErrValidation)
	}
	slug := in.Slug
	if slug == "" {
		slug = slugify(name)
	} else {
		slug = strings.ToLower(strings.TrimSpace(slug))
	}
	if !slugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: invalid slug", platform.ErrValidation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.queries.WithTx(tx)

	org, err := qtx.CreateOrganization(ctx, db.CreateOrganizationParams{
		Name:            name,
		Slug:            slug,
		BusinessType:    textVal(in.BusinessType),
		DefaultCurrency: "IDR",
		Timezone:        "Asia/Jakarta",
	})
	if err != nil {
		if isUnique(err) {
			return nil, ErrSlugTaken
		}
		return nil, err
	}
	if _, err := qtx.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: org.ID,
		UserID:         actorID,
		Role:           "owner",
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	orgID := org.ID
	s.audit.Write(ctx, AuditEvent{
		OrganizationID: &orgID,
		ActorID:        actorID,
		Action:         "org.created",
		EntityType:     "organization",
		EntityID:       &orgID,
		NewState:       map[string]any{"name": org.Name, "slug": org.Slug},
	})
	return &org, nil
}

// Membership resolves actor's active membership or ErrNotMember (callers map to 404).
func (s *Service) Membership(ctx context.Context, orgID, userID uuid.UUID) (*db.OrganizationMember, error) {
	m, err := s.queries.GetMembership(ctx, db.GetMembershipParams{
		OrganizationID: orgID,
		UserID:         userID,
	})
	if err != nil {
		return nil, ErrNotMember
	}
	return &m, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]db.ListOrganizationsForUserRow, error) {
	return s.queries.ListOrganizationsForUser(ctx, userID)
}

func (s *Service) Get(ctx context.Context, orgID uuid.UUID) (*db.Organization, error) {
	o, err := s.queries.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, platform.ErrNotFound
	}
	return &o, nil
}

func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]db.ListMembersRow, error) {
	return s.queries.ListMembers(ctx, orgID)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case !prevDash && b.Len() > 0:
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "org-" + uuid.NewString()[:8]
	}
	return out
}

func nullIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// textVal converts nullable-text db params; kept for future columns.
func textVal(s string) pgtype.Text {
	t := strings.TrimSpace(s)
	return pgtype.Text{String: t, Valid: t != ""}
}
