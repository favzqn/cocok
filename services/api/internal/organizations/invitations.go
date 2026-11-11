package organizations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/auth"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
)

type InviteInput struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Invite creates an invitation; returns raw token (shown once, only hash stored).
func (s *Service) Invite(ctx context.Context, orgID, actorID uuid.UUID, in InviteInput) (string, *db.Invitation, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return "", nil, fmt.Errorf("%w: invalid email", platform.ErrValidation)
	}
	if !auth.ValidRole(in.Role) {
		return "", nil, fmt.Errorf("%w: invalid role", platform.ErrValidation)
	}
	raw, err := platform.NewToken()
	if err != nil {
		return "", nil, err
	}
	inv, err := s.queries.CreateInvitation(ctx, db.CreateInvitationParams{
		OrganizationID: orgID,
		Email:          email,
		Role:           in.Role,
		TokenHash:      auth.HashToken(raw),
		ExpiresAt:      time.Now().UTC().Add(7 * 24 * time.Hour),
		CreatedBy:      actorID,
	})
	if err != nil {
		return "", nil, err
	}
	s.audit.Write(ctx, AuditEvent{
		OrganizationID: &orgID,
		ActorID:        actorID,
		Action:         "org.invitation.created",
		EntityType:     "invitation",
		EntityID:       &inv.ID,
		NewState:       map[string]any{"email": email, "role": in.Role},
	})
	return raw, &inv, nil
}

// AcceptInvite joins the caller via raw token; invite email must match user email.
func (s *Service) AcceptInvite(ctx context.Context, user *db.User, rawToken string) (*db.Organization, string, error) {
	inv, err := s.queries.GetInvitationByTokenHash(ctx, auth.HashToken(rawToken))
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid or expired invitation", platform.ErrValidation)
	}
	if inv.AcceptedAt != nil || time.Now().UTC().After(inv.ExpiresAt) {
		return nil, "", fmt.Errorf("%w: invitation no longer valid", platform.ErrValidation)
	}
	if !strings.EqualFold(string(inv.Email), user.Email) {
		return nil, "", fmt.Errorf("%w: invitation was issued to a different email", platform.ErrValidation)
	}
	if _, err := s.queries.CreateMembership(ctx, db.CreateMembershipParams{
		OrganizationID: inv.OrganizationID,
		UserID:         user.ID,
		Role:           inv.Role,
	}); err != nil {
		if isUnique(err) {
			_ = s.queries.MarkInvitationAccepted(ctx, inv.ID)
			org, e := s.queries.GetOrganizationByID(ctx, inv.OrganizationID)
			return &org, inv.Role, e
		}
		return nil, "", err
	}
	if err := s.queries.MarkInvitationAccepted(ctx, inv.ID); err != nil {
		return nil, "", err
	}
	org, err := s.queries.GetOrganizationByID(ctx, inv.OrganizationID)
	if err != nil {
		return nil, "", err
	}
	s.audit.Write(ctx, AuditEvent{
		OrganizationID: &inv.OrganizationID,
		ActorID:        user.ID,
		Action:         "org.invitation.accepted",
		EntityType:     "invitation",
		EntityID:       &inv.ID,
		NewState:       map[string]any{"role": inv.Role},
	})
	return &org, inv.Role, nil
}

type ChangeRoleInput struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
}

func (s *Service) ChangeRole(ctx context.Context, orgID, actorID, targetUserID uuid.UUID, newRole string) error {
	if !auth.ValidRole(newRole) {
		return fmt.Errorf("%w: invalid role", platform.ErrValidation)
	}
	cur, err := s.queries.GetMembership(ctx, db.GetMembershipParams{OrganizationID: orgID, UserID: targetUserID})
	if err != nil {
		return platform.ErrNotFound
	}
	if cur.Role == "owner" && newRole != "owner" {
		members, err := s.queries.ListMembers(ctx, orgID)
		if err != nil {
			return err
		}
		owners := 0
		for _, m := range members {
			if m.Role == "owner" && m.Status == "active" {
				owners++
			}
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if err := s.queries.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{
		OrganizationID: orgID,
		UserID:         targetUserID,
		Role:           newRole,
	}); err != nil {
		return err
	}
	s.audit.Write(ctx, AuditEvent{
		OrganizationID: &orgID,
		ActorID:        actorID,
		Action:         "org.member.role_changed",
		EntityType:     "organization_member",
		EntityID:       &targetUserID,
		PreviousState:  map[string]any{"role": cur.Role},
		NewState:       map[string]any{"role": newRole},
	})
	return nil
}
