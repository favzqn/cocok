package tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"
)

const csvBody = `Tanggal,Keterangan,Nominal,Jenis,Ref
2024-05-01,Transfer masuk Budi,1.500.000,masuk,TRX-1
2024-05-02,Payment invoice,-250000,keluar,TRX-2
`

func createAccount(t *testing.T, c *client, orgID string) string {
	t.Helper()
	res, body := c.do("POST", "/api/v1/organizations/"+orgID+"/accounts", map[string]any{
		"name": "BCA Operasional", "kind": "bank", "account_number": "1234567890",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create account: %d %v", res.StatusCode, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("no account id: %v", body)
	}
	return id
}

func uploadCSV(t *testing.T, c *client, orgID, accountID, csv string) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("account_id", accountID)
	_ = mw.WriteField("mapping", `{"date":"Tanggal","amount":"Nominal","description":"Keterangan","direction":"Jenis","reference":"Ref"}`)
	fw, err := mw.CreateFormFile("file", "statement.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(csv)); err != nil {
		t.Fatal(err)
	}
	_ = mw.Close()

	req, err := http.NewRequest("POST", c.base+"/api/v1/organizations/"+orgID+"/imports", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if c.cookie != "" {
		req.Header.Add("Cookie", c.cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func TestImportFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "imp-owner@test.id")
	orgID := createOrg(t, owner, "Import Co")
	accID := createAccount(t, owner, orgID)

	// Preview (upload) — nothing committed yet
	res, prev := uploadCSV(t, owner, orgID, accID, csvBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d %v", res.StatusCode, prev)
	}
	if prev["valid_count"].(float64) != 2 || prev["invalid_count"].(float64) != 0 {
		t.Errorf("preview counts: %v", prev)
	}
	batchID, _ := prev["batch_id"].(string)
	if batchID == "" {
		t.Fatal("no batch_id")
	}

	// Transactions not visible before commit
	res, body := owner.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list tx: %d", res.StatusCode)
	}
	if len(body["data"].([]any)) != 0 {
		t.Errorf("transactions before commit: %v", body["data"])
	}

	// Commit
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+batchID+"/commit", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("commit: %d %v", res.StatusCode, body)
	}
	if body["inserted"].(float64) != 2 {
		t.Errorf("inserted = %v, want 2", body["inserted"])
	}

	// Double commit → 409
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+batchID+"/commit", nil)
	if res.StatusCode != http.StatusConflict {
		t.Errorf("double commit: got %d, want 409", res.StatusCode)
	}

	// Re-import same file → all duplicates
	res, prev2 := uploadCSV(t, owner, orgID, accID, csvBody)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("re-upload: %d", res.StatusCode)
	}
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev2["batch_id"].(string)+"/commit", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("re-commit: %d %v", res.StatusCode, body)
	}
	if body["duplicates"].(float64) != 2 || body["inserted"].(float64) != 0 {
		t.Errorf("re-import: %v, want 0 inserted / 2 duplicates", body)
	}

	// Canonical rows still 2
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	if n := len(body["data"].([]any)); n != 2 {
		t.Errorf("tx count = %d, want 2", n)
	}

	// Bad date rows reported in preview, not fatal
	badCSV := "Tanggal,Keterangan,Nominal\nnot-a-date,x,1000\n2024-06-01,y,2000\n"
	res, prev3 := uploadCSV(t, owner, orgID, accID, badCSV)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("bad-date upload should still preview: %d %v", res.StatusCode, prev3)
	}
	if prev3["valid_count"].(float64) != 1 || prev3["invalid_count"].(float64) != 1 {
		t.Errorf("bad-date preview: %v", prev3)
	}
}

func TestImportTenantIsolationAndRBAC(t *testing.T) {
	srv, _ := newTestServer(t)

	owner := register(t, srv, "iso-owner@test.id")
	orgA := createOrg(t, owner, "Iso A")
	accA := createAccount(t, owner, orgA)

	evil := register(t, srv, "iso-evil@test.id")
	orgB := createOrg(t, evil, "Iso B")

	// Cross-org upload → 404 (account belongs to org A)
	res, _ := uploadCSV(t, evil, orgB, accA, csvBody)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-org upload with foreign account: got %d, want 404", res.StatusCode)
	}
	// Cross-org reads → 404
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgA+"/transactions", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-org tx list: got %d, want 404", res.StatusCode)
	}
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgA+"/imports", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-org import list: got %d, want 404", res.StatusCode)
	}

	// viewer: data:view yes, account:manage / import:run no
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgA+"/invitations", map[string]string{
		"email": "viewer-iso@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite viewer: %d %v", res.StatusCode, inv)
	}
	viewer := register(t, srv, "viewer-iso@test.id")
	res, body := viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("accept: %d %v", res.StatusCode, body)
	}

	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgA+"/accounts", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer list accounts: got %d, want 200", res.StatusCode)
	}
	res, _ = viewer.do("POST", "/api/v1/organizations/"+orgA+"/accounts", map[string]any{"name": "X"})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer create account: got %d, want 403", res.StatusCode)
	}
	res, _ = uploadCSV(t, viewer, orgA, accA, csvBody)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer upload: got %d, want 403", res.StatusCode)
	}

	// finance_staff: import:run yes, account:manage no
	res, inv = owner.do("POST", "/api/v1/organizations/"+orgA+"/invitations", map[string]string{
		"email": "staff-iso@test.id", "role": "finance_staff",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite staff: %d", res.StatusCode)
	}
	staff := register(t, srv, "staff-iso@test.id")
	staff.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})

	res, _ = staff.do("POST", "/api/v1/organizations/"+orgA+"/accounts", map[string]any{"name": "X"})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("staff create account: got %d, want 403", res.StatusCode)
	}
	res, _ = uploadCSV(t, staff, orgA, accA, csvBody)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("staff upload: got %d, want 201", res.StatusCode)
	}
}
