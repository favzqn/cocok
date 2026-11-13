package tests

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
)

var tokenRe = regexp.MustCompile(`token=([A-Za-z0-9_\-]+)`)

func latestToken(t *testing.T, queries *db.Queries, to, kind string) string {
	t.Helper()
	rows, err := queries.ListEmailOutbox(context.Background(), to)
	if err != nil || len(rows) == 0 {
		t.Fatalf("outbox empty for %s: err=%v rows=%d", to, err, len(rows))
	}
	for _, r := range rows {
		if r.Kind != kind {
			continue
		}
		m := tokenRe.FindStringSubmatch(r.Body)
		if len(m) == 2 {
			return m[1]
		}
	}
	t.Fatalf("no %s token in outbox for %s", kind, to)
	return ""
}

func TestEmailVerificationFlow(t *testing.T) {
	srv, queries := newTestServer(t)
	c := register(t, srv, "verify@test.id")

	// Registration sent a verification email to the sandbox outbox
	token := latestToken(t, queries, "verify@test.id", "verify_email")

	res, body := c.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("verify: %d %v", res.StatusCode, body)
	}

	// Token single-use
	res, _ = c.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": token})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("reuse token: got %d, want 400", res.StatusCode)
	}
	// Garbage token
	res, _ = c.do("POST", "/api/v1/auth/verify-email", map[string]string{"token": "nope"})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad token: got %d, want 400", res.StatusCode)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	srv, queries := newTestServer(t)
	c := register(t, srv, "reset@test.id")
	oldCookie := c.cookie

	// Forgot password → outbox (unknown email must also 200, no enumeration)
	res, _ := c.do("POST", "/api/v1/auth/forgot-password", map[string]string{"email": "nobody@test.id"})
	if res.StatusCode != http.StatusOK {
		t.Errorf("forgot unknown: got %d, want 200", res.StatusCode)
	}
	res, _ = c.do("POST", "/api/v1/auth/forgot-password", map[string]string{"email": "reset@test.id"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("forgot: %d", res.StatusCode)
	}
	token := latestToken(t, queries, "reset@test.id", "reset_password")

	// Weak password rejected
	res, _ = c.do("POST", "/api/v1/auth/reset-password", map[string]string{
		"token": token, "password": "short",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("weak password: got %d, want 400", res.StatusCode)
	}

	// Reset with valid token
	res, body := c.do("POST", "/api/v1/auth/reset-password", map[string]string{
		"token": token, "password": "password-baru1",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reset: %d %v", res.StatusCode, body)
	}

	// Old password fails, new works
	bad := &client{t: t, base: srv.URL}
	res, _ = bad.do("POST", "/api/v1/auth/login", map[string]string{
		"email": "reset@test.id", "password": "rahasia123",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("old password after reset: got %d, want 401", res.StatusCode)
	}
	res, body = bad.do("POST", "/api/v1/auth/login", map[string]string{
		"email": "reset@test.id", "password": "password-baru1",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("new password login: %d %v", res.StatusCode, body)
	}

	// All pre-reset sessions revoked
	res, _ = c.do("GET", "/api/v1/auth/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("old session after reset: got %d, want 401", res.StatusCode)
	}
	_ = oldCookie

	// Reset token single-use
	res, _ = c.do("POST", "/api/v1/auth/reset-password", map[string]string{
		"token": token, "password": "password-baru2",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("reuse reset token: got %d, want 400", res.StatusCode)
	}
}
