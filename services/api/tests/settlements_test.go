package tests

import (
	"net/http"
	"testing"
)

func TestSettlementCRUDAndAutoSettle(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "setl-owner@test.id")
	orgID := createOrg(t, owner, "Setl Co")
	ids := seedOppositePair(t, owner, orgID) // credit 1.5jt + debit 1.5jt, ref TRX-001

	// Create a pending settlement matching the credit (1.500.000, window ok)
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"counterparty":          "Budi Santoso",
		"reference":             "TRX-001",
		"expected_amount_minor": 1500000,
		"expected_date":         "2024-05-02",
		"notes":                 "expected payout",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create settlement: %d %v", res.StatusCode, body)
	}
	setlID := body["id"].(string)
	if body["status"] != "pending" {
		t.Errorf("status = %v, want pending", body["status"])
	}

	// Validation: amount must be > 0
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"expected_amount_minor": 0, "expected_date": "2024-05-02",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("zero amount: got %d, want 400", res.StatusCode)
	}

	// Run + approve the pair → auto-settle must flip the settlement.
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK || body["proposed_groups"].(float64) < 1 {
		t.Fatalf("run: %d %v", res.StatusCode, body)
	}
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups?status=proposed", nil)
	groups := body["data"].([]any)
	groupID := groups[0].(map[string]any)["id"].(string)
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/approve", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d %v", res.StatusCode, body)
	}

	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/settlements?status=settled", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list settled: %d", res.StatusCode)
	}
	settled := body["data"].([]any)
	if len(settled) != 1 {
		t.Fatalf("settled count = %d, want 1 (%v)", len(settled), body)
	}
	row := settled[0].(map[string]any)
	if row["id"] != setlID {
		t.Errorf("wrong settlement settled")
	}
	if row["settled_amount_minor"].(float64) != 1500000 {
		t.Errorf("settled amount = %v", row["settled_amount_minor"])
	}
	if row["linked_group_id"] != groupID {
		t.Errorf("linked_group_id = %v, want %s", row["linked_group_id"], groupID)
	}

	// Manual status patch (e.g. mark failed on a new one) + delete
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"expected_amount_minor": 50000, "expected_date": "2024-06-01",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create 2: %d %v", res.StatusCode, body)
	}
	id2 := body["id"].(string)
	res, _ = owner.do("PATCH", "/api/v1/organizations/"+orgID+"/settlements/"+id2, map[string]any{
		"status": "failed", "notes": "gateway rejected",
	})
	if res.StatusCode != http.StatusOK {
		t.Errorf("patch failed status: got %d", res.StatusCode)
	}
	res, _ = owner.do("DELETE", "/api/v1/organizations/"+orgID+"/settlements/"+id2, nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("delete: got %d", res.StatusCode)
	}

	_ = ids
}

func TestSettlementRBACAndTenant(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "setl-rb-owner@test.id")
	orgID := createOrg(t, owner, "Setl RB Co")

	// finance_staff: reconcile:run → create ok
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "setl-staff@test.id", "role": "finance_staff",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite staff")
	}
	staff := register(t, srv, "setl-staff@test.id")
	staff.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	res, body := staff.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"expected_amount_minor": 1000, "expected_date": "2024-05-01",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("staff create settlement: %d %v", res.StatusCode, body)
	}

	// viewer: list ok, create forbidden
	res, inv = owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "setl-viewer@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite viewer")
	}
	viewer := register(t, srv, "setl-viewer@test.id")
	viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgID+"/settlements", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer list: got %d, want 200", res.StatusCode)
	}
	res, _ = viewer.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"expected_amount_minor": 1000, "expected_date": "2024-05-01",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer create: got %d, want 403", res.StatusCode)
	}

	// Cross-tenant
	evil := register(t, srv, "setl-evil@test.id")
	evilOrg := createOrg(t, evil, "Setl Evil")
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/settlements", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant list: got %d, want 404", res.StatusCode)
	}
	res, _ = evil.do("POST", "/api/v1/organizations/"+orgID+"/settlements", map[string]any{
		"expected_amount_minor": 1, "expected_date": "2024-05-01",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant create: got %d, want 404", res.StatusCode)
	}
	_ = evilOrg
}
