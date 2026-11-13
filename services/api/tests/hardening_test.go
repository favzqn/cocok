package tests

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cocok/cocok/services/api/internal/platform"
	"github.com/cocok/cocok/services/api/migrations"
)

func TestSecurityHeadersAndMetrics(t *testing.T) {
	srv, _ := newTestServer(t)

	// Security headers present on every response
	res, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for k, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	} {
		if got := res.Header.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}

	// Generate some traffic, then scrape metrics
	http.Get(srv.URL + "/healthz")
	mres, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer mres.Body.Close()
	buf := make([]byte, 4096)
	n, _ := mres.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "cocok_http_responses_total") {
		t.Errorf("metrics missing counter:\n%s", body)
	}
	if !strings.Contains(body, "cocok_process_uptime_seconds") {
		t.Errorf("metrics missing uptime:\n%s", body)
	}
}

func TestAuditLogAppendOnly(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := platform.OpenPool(ctx, url)
	if err != nil {
		t.Skipf("test db not available: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := platform.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, actor_id, action, entity_type)
		VALUES (NULL, NULL, 'test.event', 'test')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_logs SET action = 'tampered'`); err == nil {
		t.Error("UPDATE on audit_logs must fail (append-only trigger)")
	} else if !strings.Contains(err.Error(), "append-only") {
		t.Errorf("unexpected update error: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_logs`); err == nil {
		t.Error("DELETE on audit_logs must fail (append-only trigger)")
	}
}
