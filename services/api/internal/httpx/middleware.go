package httpx

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/cocok/cocok/services/api/internal/auth"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxUser
	ctxOrgID
	ctxRole
)

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func UserFrom(ctx context.Context) *db.User {
	if v, ok := ctx.Value(ctxUser).(*db.User); ok {
		return v
	}
	return nil
}

func OrgFrom(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(ctxOrgID).(uuid.UUID)
	return v, ok
}

func RoleFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRole).(string); ok {
		return v
	}
	return ""
}

// WithRequestID injects/propagates X-Request-ID.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b, err := uuid.NewRandom()
			if err == nil {
				id = b.String()
			}
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

// RequireAuth resolves the session cookie into ctxUser or 401.
func RequireAuth(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(auth.CookieName)
			if err != nil || c.Value == "" {
				Err(w, r, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
				return
			}
			user, err := svc.Authenticate(r.Context(), c.Value)
			if err != nil {
				Err(w, r, http.StatusUnauthorized, "unauthorized", "invalid or expired session", nil)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxUser, user)))
		})
	}
}

// WithOrg loads :orgID, verifies active membership, injects org+role.
// Non-members get 404 (no ID probing).
func WithOrg(orgs OrgMembership) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFrom(r.Context())
			if user == nil {
				Err(w, r, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
				return
			}
			idStr := chi.URLParam(r, "orgID")
			orgID, err := uuid.Parse(idStr)
			if err != nil {
				NotFound(w, r)
				return
			}
			m, err := orgs.Membership(r.Context(), orgID, user.ID)
			if err != nil {
				NotFound(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), ctxOrgID, orgID)
			ctx = context.WithValue(ctx, ctxRole, m.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OrgMembership is the consumer interface for tenant checks.
type OrgMembership interface {
	Membership(ctx context.Context, orgID, userID uuid.UUID) (*db.OrganizationMember, error)
}

// RequirePermission enforces a permission for the role in ctx.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFrom(r.Context())
			if !auth.RoleHasPermission(role, perm) {
				Err(w, r, http.StatusForbidden, "forbidden", "permission denied", map[string]any{"required": perm})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit is a simple per-IP limiter for auth endpoints.
func RateLimit(rps float64, burst int) func(http.Handler) http.Handler {
	var mu sync.Mutex
	lims := make(map[string]*rate.Limiter)
	get := func(ip string) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()
		l, ok := lims[ip]
		if !ok {
			l = rate.NewLimiter(rate.Limit(rps), burst)
			lims[ip] = l
		}
		return l
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := strings.Split(r.RemoteAddr, ":")[0]
			if !get(ip).Allow() {
				Err(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
