package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cocok/cocok/services/api/internal/accounts"
	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/auth"
	"github.com/cocok/cocok/services/api/internal/httpx"
	"github.com/cocok/cocok/services/api/internal/imports"
	"github.com/cocok/cocok/services/api/internal/organizations"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/cocok/cocok/services/api/internal/reconciliation"
	"github.com/cocok/cocok/services/api/migrations"
)

// Integration tests run against a real PostgreSQL.
// Set TEST_DATABASE_URL (e.g. postgres://user@localhost:5432/cocok_test?sslmode=disable).
// The test database schema is wiped at the start of each run.

func newTestServer(t *testing.T) (*httptest.Server, *db.Queries) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := platform.OpenPool(ctx, url)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	queries := db.New(pool)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	auditWriter := audit.NewWriter(queries, log)
	authSvc := auth.NewService(queries, 24*time.Hour)
	orgSvc := organizations.NewService(queries, pool, audit.OrgAdapter{W: auditWriter})
	acctSvc := accounts.NewService(queries, pool, auditWriter)
	impSvc := imports.NewService(queries, pool, auditWriter)
	reconSvc := reconciliation.NewService(queries, pool, auditWriter)
	srv := httptest.NewServer(httpx.NewRouter(httpx.Deps{
		Auth:     httpx.NewAuthHandlers(authSvc, log, false),
		Orgs:     httpx.NewOrgHandlers(orgSvc, log),
		Accounts: httpx.NewAccountHandlers(acctSvc, log),
		Imports:  httpx.NewImportHandlers(impSvc, log),
		Recon:    httpx.NewReconHandlers(reconSvc, log),
		OrgsSvc:  orgSvc,
		Log:      log,
	}))
	t.Cleanup(srv.Close)
	return srv, queries
}

type client struct {
	t      *testing.T
	base   string
	cookie string
}

func (c *client) do(method, path string, body any) (*http.Response, map[string]any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rd)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cookie != "" {
		req.Header.Add("Cookie", c.cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	for _, ck := range res.Cookies() {
		if ck.Name == auth.CookieName && ck.Value != "" {
			c.cookie = auth.CookieName + "=" + ck.Value
		}
	}
	return res, out
}

func register(t *testing.T, srv *httptest.Server, email string) *client {
	t.Helper()
	c := &client{t: t, base: srv.URL}
	res, body := c.do("POST", "/api/v1/auth/register", map[string]string{
		"email": email, "password": "rahasia123", "full_name": "Test User",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("register %s: status %d body %v", email, res.StatusCode, body)
	}
	return c
}

func createOrg(t *testing.T, c *client, name string) string {
	t.Helper()
	res, body := c.do("POST", "/api/v1/organizations", map[string]string{"name": name})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create org: status %d body %v", res.StatusCode, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no org id in %v", body)
	}
	return id
}
