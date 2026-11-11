package tests

import (
	"net/http"
	"testing"
)

func TestTenantIsolation(t *testing.T) {
	srv, _ := newTestServer(t)

	alice := register(t, srv, "alice@test.id")
	orgA := createOrg(t, alice, "Company A")

	bob := register(t, srv, "bob@test.id")
	orgB := createOrg(t, bob, "Company B")

	// Bob must not read org A (404, no ID probing)
	res, _ := bob.do("GET", "/api/v1/organizations/"+orgA, nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant GET: got %d, want 404", res.StatusCode)
	}
	res, _ = bob.do("GET", "/api/v1/organizations/"+orgA+"/members", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant members: got %d, want 404", res.StatusCode)
	}
	res, _ = bob.do("POST", "/api/v1/organizations/"+orgA+"/invitations", map[string]string{
		"email": "x@evil.id", "role": "admin",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant invite: got %d, want 404", res.StatusCode)
	}

	// Alice reads her own org fine
	res, body := alice.do("GET", "/api/v1/organizations/"+orgA, nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("owner GET own org: got %d body %v", res.StatusCode, body)
	}
	if body["timezone"] != "Asia/Jakarta" || body["default_currency"] != "IDR" {
		t.Errorf("default org config wrong: tz=%v cur=%v", body["timezone"], body["default_currency"])
	}

	// Unauthenticated access rejected
	anon := &client{t: t, base: srv.URL}
	res, _ = anon.do("GET", "/api/v1/organizations", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("anon list orgs: got %d, want 401", res.StatusCode)
	}
	_ = orgB
}

func TestAuthFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	c := register(t, srv, "login@test.id")

	bad := &client{t: t, base: srv.URL}
	res, _ := bad.do("POST", "/api/v1/auth/login", map[string]string{
		"email": "login@test.id", "password": "salah",
	})
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: got %d, want 401", res.StatusCode)
	}

	res, body := bad.do("POST", "/api/v1/auth/login", map[string]string{
		"email": "login@test.id", "password": "rahasia123",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login: got %d body %v", res.StatusCode, body)
	}

	res, _ = c.do("POST", "/api/v1/auth/register", map[string]string{
		"email": "login@test.id", "password": "rahasia123", "full_name": "Dup",
	})
	if res.StatusCode != http.StatusConflict {
		t.Errorf("duplicate register: got %d, want 409", res.StatusCode)
	}

	// refresh rotates the token
	oldCookie := c.cookie
	res, _ = c.do("POST", "/api/v1/auth/refresh", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh: got %d", res.StatusCode)
	}
	if c.cookie == oldCookie {
		t.Error("refresh did not rotate token")
	}
	old := &client{t: t, base: srv.URL, cookie: oldCookie}
	res, _ = old.do("GET", "/api/v1/auth/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("revoked token reuse: got %d, want 401", res.StatusCode)
	}

	res, _ = c.do("POST", "/api/v1/auth/logout", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("logout: got %d", res.StatusCode)
	}
	res, _ = c.do("GET", "/api/v1/auth/me", nil)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("me after logout: got %d, want 401", res.StatusCode)
	}
}
