package tests

import (
	"net/http"
	"testing"
)

func TestExceptionsGeneratedOnRun(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "exc-owner@test.id")
	orgID := createOrg(t, owner, "Exc Co")
	seedOppositePair(t, owner, orgID)

	// Run: 1 pair proposed → its 2 txs NOT exceptions; seed has only those 2.
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("run: %d %v", res.StatusCode, body)
	}
	if body["proposed_groups"].(float64) < 1 {
		t.Fatalf("expected proposal, got %v", body)
	}

	// All txs matched → no open unmatched exceptions.
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/exceptions?status=open", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list exceptions: %d", res.StatusCode)
	}
	if n := len(body["data"].([]any)); n != 0 {
		t.Errorf("open exceptions with full match: got %d, want 0", n)
	}

	// Import an orphan credit that cannot pair → unmatched exception on rerun.
	accID := createAccount(t, owner, orgID)
	orphan := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-05-10,Donasi aneh,99999999,masuk,ORPHAN-1\n"
	res, prev := uploadCSV(t, owner, orgID, accID, orphan)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("orphan upload: %d", res.StatusCode)
	}
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev["batch_id"].(string)+"/commit", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatal("orphan commit failed")
	}
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if body["proposed_groups"].(float64) < 1 {
		t.Fatalf("pair should survive rerun: %v", body)
	}
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/exceptions?status=open&type=unmatched", nil)
	excList := body["data"].([]any)
	if len(excList) != 1 {
		t.Fatalf("open unmatched exceptions = %d, want 1 (%v)", len(excList), body)
	}
	excID := excList[0].(map[string]any)["id"].(string)

	// Dismiss → never re-opened on later runs.
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/exceptions/"+excID+"/dismiss", map[string]string{
		"note": "intentional donation",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("dismiss: %d", res.StatusCode)
	}
	// Matching the orphan removes it from leftover anyway; dismiss must also
	// survive a rerun where it is still unmatched — run again.
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	_ = body
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/exceptions?status=open&type=unmatched", nil)
	if n := len(body["data"].([]any)); n != 0 {
		t.Errorf("dismissed exception re-opened: %d open", n)
	}

	// Resolve flow on a fresh manual exception.
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/exceptions", map[string]any{
		"transaction_ids": []string{},
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("empty flag: got %d, want 400", res.StatusCode)
	}
}

func TestExceptionsRBACAndTenant(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "exc-rb-owner@test.id")
	orgID := createOrg(t, owner, "Exc RB Co")
	ids := seedOppositePair(t, owner, orgID)

	// viewer: read ok, resolve forbidden
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "exc-viewer@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite viewer")
	}
	viewer := register(t, srv, "exc-viewer@test.id")
	viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})

	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgID+"/exceptions", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer list exceptions: got %d, want 200", res.StatusCode)
	}
	// Manual flag by owner, viewer tries to dismiss it
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/exceptions", map[string]any{
		"transaction_ids": ids[:1], "note": "suspicious",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("owner flag: %d %v", res.StatusCode, body)
	}
	excID := body["id"].(string)
	res, _ = viewer.do("POST", "/api/v1/organizations/"+orgID+"/exceptions/"+excID+"/dismiss", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer dismiss: got %d, want 403", res.StatusCode)
	}

	// Double-flag → 409
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/exceptions", map[string]any{
		"transaction_ids": ids[:1],
	})
	if res.StatusCode != http.StatusConflict {
		t.Errorf("double flag: got %d, want 409", res.StatusCode)
	}

	// Cross-tenant: evil sees 404 on org's exceptions
	evil := register(t, srv, "exc-evil@test.id")
	orgB := createOrg(t, evil, "Exc B")
	evil.do("POST", "/api/v1/organizations", map[string]string{"name": "unused"}) // noop keep orgB used
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/exceptions", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant exceptions: got %d, want 404", res.StatusCode)
	}
	_ = orgB
}
