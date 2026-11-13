package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/jackc/pgx/v5/pgtype"
)

const CookieName = "cocok_session"

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailTaken         = errors.New("email already registered")
	ErrDisabled           = errors.New("account disabled")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidToken       = errors.New("invalid or expired token")
)

type Service struct {
	queries   *db.Queries
	ttl       time.Duration
	mailer    Mailer
	webOrigin string
}

func NewService(q *db.Queries, ttl time.Duration, m Mailer, webOrigin string) *Service {
	return &Service{queries: q, ttl: ttl, mailer: m, webOrigin: strings.TrimRight(webOrigin, "/")}
}

type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (*db.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, fmt.Errorf("%w: invalid email", platform.ErrValidation)
	}
	if len(in.Password) < 8 {
		return nil, fmt.Errorf("%w: password must be at least 8 characters", platform.ErrValidation)
	}
	if strings.TrimSpace(in.FullName) == "" {
		return nil, fmt.Errorf("%w: full name is required", platform.ErrValidation)
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u, err := s.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(in.FullName),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	s.SendVerification(ctx, &u)
	return &u, nil
}

type LoginResult struct {
	User      *db.User
	RawToken  string
	ExpiresAt time.Time
}

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (*LoginResult, error) {
	u, err := s.queries.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		// constant-ish work to reduce user enumeration timing signal
		_, _ = HashPassword("dummy-password-for-timing")
		return nil, ErrInvalidCredentials
	}
	if !VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if u.Status != "active" {
		return nil, ErrDisabled
	}
	return s.createSession(ctx, u, userAgent, ip)
}

func (s *Service) createSession(ctx context.Context, u db.User, userAgent, ip string) (*LoginResult, error) {
	raw, err := platform.NewToken()
	if err != nil {
		return nil, err
	}
	exp := time.Now().UTC().Add(s.ttl)
	sess, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    u.ID,
		TokenHash: HashToken(raw),
		UserAgent: pgtype.Text{String: truncate(userAgent, 255), Valid: userAgent != ""},
		Ip:        pgtype.Text{String: ip, Valid: ip != ""},
		ExpiresAt: exp,
	})
	if err != nil {
		return nil, err
	}
	_ = sess
	return &LoginResult{User: &u, RawToken: raw, ExpiresAt: exp}, nil
}

// Refresh rotates the session token: revokes old, issues new.
func (s *Service) Refresh(ctx context.Context, rawToken, userAgent, ip string) (*LoginResult, error) {
	old, err := s.queries.GetSessionByTokenHash(ctx, HashToken(rawToken))
	if err != nil {
		return nil, ErrInvalidSession
	}
	u, err := s.queries.GetUserByID(ctx, old.UserID)
	if err != nil || u.Status != "active" {
		return nil, ErrInvalidSession
	}
	if err := s.queries.RevokeSession(ctx, old.ID); err != nil {
		return nil, err
	}
	return s.createSession(ctx, u, userAgent, ip)
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	return s.queries.RevokeSessionByTokenHash(ctx, HashToken(rawToken))
}

// Authenticate resolves a raw cookie token to a user, or ErrInvalidSession.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (*db.User, error) {
	if rawToken == "" {
		return nil, ErrInvalidSession
	}
	sess, err := s.queries.GetSessionByTokenHash(ctx, HashToken(rawToken))
	if err != nil {
		return nil, ErrInvalidSession
	}
	u, err := s.queries.GetUserByID(ctx, sess.UserID)
	if err != nil || u.Status != "active" {
		return nil, ErrInvalidSession
	}
	return &u, nil
}

// HashToken stores only sha256 of the raw session token.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func ptr(s string) *string { return &s }
