package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
)

// Mailer is the consumer-side email interface; sandbox impl writes to the
// email_outbox table (notifications package), real SMTP/SES lands Phase 7.
type Mailer interface {
	Send(ctx context.Context, to, subject, body, kind string) error
}

const verifyTokenTTL = 24 * time.Hour
const resetTokenTTL = time.Hour

// SendVerification emails a one-time verify link (best-effort; never fails registration).
func (s *Service) SendVerification(ctx context.Context, u *db.User) {
	if s.mailer == nil {
		return
	}
	raw, err := platform.NewToken()
	if err != nil {
		return
	}
	if _, err := s.queries.CreateAuthToken(ctx, db.CreateAuthTokenParams{
		UserID:    u.ID,
		Kind:      "verify_email",
		TokenHash: HashToken(raw),
		ExpiresAt: time.Now().UTC().Add(verifyTokenTTL),
	}); err != nil {
		return
	}
	link := s.webOrigin + "/verify-email?token=" + raw
	body := "Hi " + u.FullName + ",\n\nVerify your COCOK account:\n" + link +
		"\n\nThis link expires in 24 hours.\n"
	_ = s.mailer.Send(ctx, string(u.Email), "Verify your COCOK account", body, "verify_email")
}

// VerifyEmail consumes a verify token (idempotent-ish: used tokens rejected).
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidToken
	}
	tok, err := s.queries.GetAuthTokenByHash(ctx, HashToken(token))
	if err != nil || tok.Kind != "verify_email" {
		return ErrInvalidToken
	}
	if err := s.queries.MarkEmailVerified(ctx, tok.UserID); err != nil {
		return err
	}
	return s.queries.MarkAuthTokenUsed(ctx, tok.ID)
}

// ForgotPassword always returns nil for unknown emails (no enumeration).
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	if s.mailer == nil {
		return nil
	}
	u, err := s.queries.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || u.Status != "active" {
		return nil
	}
	raw, err := platform.NewToken()
	if err != nil {
		return err
	}
	if _, err := s.queries.CreateAuthToken(ctx, db.CreateAuthTokenParams{
		UserID:    u.ID,
		Kind:      "reset_password",
		TokenHash: HashToken(raw),
		ExpiresAt: time.Now().UTC().Add(resetTokenTTL),
	}); err != nil {
		return err
	}
	link := s.webOrigin + "/reset-password?token=" + raw
	body := "Hi " + u.FullName + ",\n\nReset your COCOK password:\n" + link +
		"\n\nThis link expires in 1 hour. If you didn't request this, ignore this email.\n"
	return s.mailer.Send(ctx, string(u.Email), "Reset your COCOK password", body, "reset_password")
}

// ResetPassword sets a new password from a reset token and revokes ALL sessions.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if strings.TrimSpace(token) == "" {
		return ErrInvalidToken
	}
	if len(newPassword) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", platform.ErrValidation)
	}
	tok, err := s.queries.GetAuthTokenByHash(ctx, HashToken(token))
	if err != nil || tok.Kind != "reset_password" {
		return ErrInvalidToken
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.queries.SetUserPasswordHash(ctx, db.SetUserPasswordHashParams{
		ID: tok.UserID, PasswordHash: hash,
	}); err != nil {
		return err
	}
	if err := s.queries.RevokeAllUserSessions(ctx, tok.UserID); err != nil {
		return err
	}
	return s.queries.MarkAuthTokenUsed(ctx, tok.ID)
}
