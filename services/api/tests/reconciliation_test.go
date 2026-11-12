package tests

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// seedOppositePair imports two statements yielding one credit + one debit with
// shared ref PAY-1 (near-certain auto-match), returns (org, acc, txIDs).
func seedOppositePair(t *testing.T, c *client, orgID string) []string {
	t.Helper()
	accID := createAccount(t, c, orgID)
	creditCSV := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-05-01,Payout Shopee,1500000,masuk,PAY-1\n"
	debitCSV := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-05-02,Settlement ke BCA,-1500000,keluar,PAY-1\n"

	commitCSV := func(csv string) {
		res, prev := uploadCSV(t, c, orgID, accID, csv)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("seed upload: %d %v", res.StatusCode, prev)
		}
		res, body := c.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev["batch_id"].(string)+"/commit", nil)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("seed commit: %d %v", res.StatusCode, body)
		}
	}
	commitCSV(creditCSV)
	commitCSV(debitCSV)

	res, body := c.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list tx: %d", res.StatusCode)
	}
	data, _ := body["data"].([]any)
	var ids []string
	for _, row := range data {
		ids = append(ids, fmt.Sprint(row.(map[string]any)["id"]))
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(ids))
	}
	return ids
}

func TestReconciliationRunApproveFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "recon-owner@test.id")
	orgID := createOrg(t, owner, "Recon Co")
	seedOppositePair(t, owner, orgID)

	// Run → at least one proposed group
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("run: %d %v", res.StatusCode, body)
	}
	if body["proposed_groups"].(float64) < 1 {
		t.Fatalf("proposed_groups = %v, want >= 1", body["proposed_groups"])
	}

	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups?status=proposed", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list groups: %d", res.StatusCode)
	}
	groups := body["data"].([]any)
	if len(groups) < 1 {
		t.Fatal("no proposed groups")
	}
	g := groups[0].(map[string]any)
	groupID := g["id"].(string)
	if g["kind"] != "1:1" {
		t.Errorf("kind = %v, want 1:1", g["kind"])
	}
	if len(g["credit_tx_ids"].([]any)) != 1 || len(g["debit_tx_ids"].([]any)) != 1 {
		t.Errorf("group members: %v", g)
	}

	// Re-run replaces proposed groups (no duplicates)
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rerun: %d", res.StatusCode)
	}
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups?status=proposed", nil)
	groups = body["data"].([]any)
	if len(groups) != 1 {
		t.Errorf("rerun proposed groups = %d, want 1", len(groups))
	}
	groupID = groups[0].(map[string]any)["id"].(string)

	// Approve
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/approve", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d %v", res.StatusCode, body)
	}
	// Approve again → 409 (not proposed anymore)
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/approve", nil)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("double approve: got %d, want 409", res.StatusCode)
	}

	// Matched txs excluded from next run
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("run after approve: %d", res.StatusCode)
	}
	if body["proposed_groups"].(float64) != 0 {
		t.Errorf("matched txs re-proposed: %v", body)
	}

	// Unmatch frees them
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/unmatch", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("unmatch: got %d", res.StatusCode)
	}
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if body["proposed_groups"].(float64) < 1 {
		t.Errorf("unmatched txs not re-proposed: %v", body)
	}
}

func TestManualMatchValidation(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "manual-owner@test.id")
	orgID := createOrg(t, owner, "Manual Co")
	ids := seedOppositePair(t, owner, orgID)

	// Balanced manual match (no run needed) → 201
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups", map[string]any{
		"transaction_ids": ids,
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("manual match: %d %v", res.StatusCode, body)
	}
	if body["status"] != "approved" {
		t.Errorf("manual group status = %v, want approved", body["status"])
	}

	// Same txs again → 409 (already matched)
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups", map[string]any{
		"transaction_ids": ids,
	})
	if res.StatusCode != http.StatusConflict {
		t.Errorf("duplicate manual match: got %d, want 409", res.StatusCode)
	}

	// Run cannot touch them either
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if body["proposed_groups"].(float64) != 0 {
		t.Errorf("manually matched txs re-proposed: %v", body)
	}

	// Cross-org tx id in manual match → 404
	evil := register(t, srv, "manual-evil@test.id")
	orgB := createOrg(t, evil, "Manual B")
	idsB := seedOppositePair(t, evil, orgB)
	res, _ = evil.do("POST", "/api/v1/organizations/"+orgB+"/reconciliation/groups", map[string]any{
		"transaction_ids": append([]string{ids[0]}, idsB...),
	})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-org tx manual match: got %d, want 404", res.StatusCode)
	}
}

func TestManualMatchUnbalanced(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "unbal-owner@test.id")
	orgID := createOrg(t, owner, "Unbal Co")
	ids := seedOppositePair(t, owner, orgID)

	// Single side only → 400
	res, _ := owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups", map[string]any{
		"transaction_ids": ids[:1],
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("one-sided match: got %d, want 400", res.StatusCode)
	}
}

func TestReconciliationRBACAndTenant(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "rrb-owner@test.id")
	orgID := createOrg(t, owner, "RRB Co")
	seedOppositePair(t, owner, orgID)

	// viewer: run forbidden, groups viewable
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "rrb-viewer@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite: %d", res.StatusCode)
	}
	viewer := register(t, srv, "rrb-viewer@test.id")
	viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})

	res, _ = viewer.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer run: got %d, want 403", res.StatusCode)
	}
	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer list groups: got %d, want 200", res.StatusCode)
	}

	// finance_staff: reconcile:run yes, match:approve no
	res, inv = owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "rrb-staff@test.id", "role": "finance_staff",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite staff: %d", res.StatusCode)
	}
	staff := register(t, srv, "rrb-staff@test.id")
	staff.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})

	res, body := staff.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("staff run: %d %v", res.StatusCode, body)
	}
	res, body = staff.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups?status=proposed", nil)
	groups := body["data"].([]any)
	if len(groups) == 0 {
		t.Fatal("no proposed groups for staff approve test")
	}
	groupID := groups[0].(map[string]any)["id"].(string)
	res, _ = staff.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/approve", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("staff approve: got %d, want 403", res.StatusCode)
	}

	// Cross-tenant → 404
	evil := register(t, srv, "rrb-evil@test.id")
	orgB := createOrg(t, evil, "RRB B")
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant groups: got %d, want 404", res.StatusCode)
	}
	res, _ = evil.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant run: got %d, want 404", res.StatusCode)
	}
	_ = orgB
}

// Two concurrent manual matches over the same transactions: exactly one wins.
func TestManualMatchConcurrency(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "conc-owner@test.id")
	orgID := createOrg(t, owner, "Conc Co")
	ids := seedOppositePair(t, owner, orgID)

	const n = 4
	statuses := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Fresh client per goroutine (own session copy).
			c := &client{t: t, base: srv.URL, cookie: owner.cookie}
			res, _ := c.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups", map[string]any{
				"transaction_ids": ids,
			})
			statuses[i] = res.StatusCode
		}(i)
	}
	wg.Wait()

	created := 0
	for _, s := range statuses {
		switch s {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			// expected for the losers
		default:
			t.Errorf("unexpected status %d", s)
		}
	}
	if created != 1 {
		t.Errorf("created = %d, want exactly 1 (statuses %v)", created, statuses)
	}
}
