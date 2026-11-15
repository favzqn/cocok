package tests

import (
	"net/http"
	"testing"
)

func TestAuditLogEndpoint(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "audit-owner@test.id")
	orgID := createOrg(t, owner, "Audit Co")

	// org.created event must be in the trail
	res, body := owner.do("GET", "/api/v1/organizations/"+orgID+"/audit-logs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list audit: %d %v", res.StatusCode, body)
	}
	logs := body["data"].([]any)
	if len(logs) == 0 {
		t.Fatal("empty audit trail after org creation")
	}
	found := false
	for _, row := range logs {
		if row.(map[string]any)["action"] == "org.created" {
			found = true
		}
	}
	if !found {
		t.Errorf("org.created missing from trail: %v", logs)
	}

	// viewer: audit:view NOT in viewer role → 403
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "audit-viewer@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite")
	}
	viewer := register(t, srv, "audit-viewer@test.id")
	viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgID+"/audit-logs", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer audit: got %d, want 403", res.StatusCode)
	}

	// cross-tenant → 404
	evil := register(t, srv, "audit-evil@test.id")
	createOrg(t, evil, "Audit Evil")
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/audit-logs", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant audit: got %d, want 404", res.StatusCode)
	}
}

func TestOrgSettingsUpdate(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "settings-owner@test.id")
	orgID := createOrg(t, owner, "Settings Co")

	// Update name + timezone
	res, body := owner.do("PATCH", "/api/v1/organizations/"+orgID, map[string]any{
		"name": "Settings Renamed", "timezone": "Asia/Makassar",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("patch: %d %v", res.StatusCode, body)
	}
	if body["name"] != "Settings Renamed" || body["timezone"] != "Asia/Makassar" {
		t.Errorf("patched org = %v", body)
	}

	// Bad timezone → 400
	res, _ = owner.do("PATCH", "/api/v1/organizations/"+orgID, map[string]any{
		"timezone": "Mars/Olympus",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad tz: got %d, want 400", res.StatusCode)
	}

	// audit trail records org.updated
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/audit-logs", nil)
	found := false
	for _, row := range body["data"].([]any) {
		if row.(map[string]any)["action"] == "org.updated" {
			found = true
		}
	}
	if !found {
		t.Error("org.updated missing from audit trail")
	}

	// finance_staff lacks org:manage → 403
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "settings-staff@test.id", "role": "finance_staff",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite")
	}
	staff := register(t, srv, "settings-staff@test.id")
	staff.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	res, _ = staff.do("PATCH", "/api/v1/organizations/"+orgID, map[string]any{"name": "X"})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("staff patch: got %d, want 403", res.StatusCode)
	}
}
