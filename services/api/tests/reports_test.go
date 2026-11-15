package tests

import (
	"encoding/csv"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReportsSummaryAndExport(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "rep-owner@test.id")
	orgID := createOrg(t, owner, "Rep Co")
	seedOppositePair(t, owner, orgID)

	// Run + approve so matched stats exist
	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK || body["proposed_groups"].(float64) < 1 {
		t.Fatalf("run: %d %v", res.StatusCode, body)
	}
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reconciliation/groups?status=proposed", nil)
	groupID := body["data"].([]any)[0].(map[string]any)["id"].(string)
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/groups/"+groupID+"/approve", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d", res.StatusCode)
	}

	// Add an orphan → exception after another run
	accID := createAccount(t, owner, orgID)
	orphan := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-05-11,Sisa,42000,masuk,LEFT-1\n"
	res, prev := uploadCSV(t, owner, orgID, accID, orphan)
	if res.StatusCode != http.StatusCreated {
		t.Fatal("orphan upload")
	}
	owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev["batch_id"].(string)+"/commit", nil)
	res, _ = owner.do("POST", "/api/v1/organizations/"+orgID+"/reconciliation/runs", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatal("rerun")
	}

	// Summary assertions
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("summary: %d %v", res.StatusCode, body)
	}
	tx := body["transactions"].(map[string]any)
	if tx["total_count"].(float64) != 3 {
		t.Errorf("total_count = %v, want 3", tx["total_count"])
	}
	matched := body["matched"].(map[string]any)
	if matched["count"].(float64) != 2 {
		t.Errorf("matched count = %v, want 2", matched["count"])
	}
	if rate := matched["match_rate"].(float64); rate < 0.6 || rate > 0.7 {
		t.Errorf("match_rate = %v, want ~0.667", rate)
	}
	exc := body["exceptions_by_status"].(map[string]any)
	if exc["open"].(float64) != 1 {
		t.Errorf("open exceptions = %v, want 1", exc["open"])
	}
	accs := body["accounts"].([]any)
	if len(accs) == 0 {
		t.Error("accounts rollup empty")
	}

	// Export transactions → valid CSV attachment
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/organizations/"+orgID+"/reports/export?kind=transactions", nil)
	req.Header.Add("Cookie", owner.cookie)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("export: %d", res2.StatusCode)
	}
	if ct := res2.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("content-type = %q", ct)
	}
	if cd := res2.Header.Get("Content-Disposition"); !strings.Contains(cd, "transactions_") {
		t.Errorf("content-disposition = %q", cd)
	}
	raw, _ := io.ReadAll(res2.Body)
	records, err := csv.NewReader(strings.NewReader(string(raw))).ReadAll()
	if err != nil || len(records) < 4 { // header + 3 rows
		t.Fatalf("csv parse: err=%v rows=%d", err, len(records))
	}
	if records[0][0] != "id" || records[0][8] != "matched" {
		t.Errorf("header = %v", records[0])
	}
	matchedCol := 0
	for _, row := range records[1:] {
		if row[8] == "true" {
			matchedCol++
		}
	}
	if matchedCol != 2 {
		t.Errorf("matched rows in csv = %d, want 2", matchedCol)
	}

	// Bad kind → 400
	res, _ = owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/export?kind=bogus", nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad kind: got %d, want 400", res.StatusCode)
	}

	// Cross-tenant → 404
	evil := register(t, srv, "rep-evil@test.id")
	evilOrg := createOrg(t, evil, "Rep Evil")
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant summary: got %d, want 404", res.StatusCode)
	}
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/reports/export?kind=transactions", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant export: got %d, want 404", res.StatusCode)
	}
	_ = evilOrg
}

func TestReportsDateRange(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "rep-range@test.id")
	orgID := createOrg(t, owner, "Range Co")
	// statement.csv dates: 05-01 (2 tx), 05-02 (1), 05-03 (1), 05-04..07 (4)
	seedOppositePair(t, owner, orgID) // 05-01 credit + 05-02 debit
	accID := createAccount(t, owner, orgID)
	extra := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-05-05,Late,7000,masuk,LATE-1\n2024-04-28,Early,8000,masuk,EARLY-1\n"
	res, prev := uploadCSV(t, owner, orgID, accID, extra)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("upload: %d", res.StatusCode)
	}
	owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev["batch_id"].(string)+"/commit", nil)

	// Full range (no filter): 4 tx
	res, body := owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary", nil)
	if got := body["transactions"].(map[string]any)["total_count"].(float64); got != 4 {
		t.Errorf("unfiltered total = %v, want 4", got)
	}
	// May only: 05-01..05-05 → 3 (excludes 04-28)
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary?from=2024-05-01&to=2024-05-06", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("range summary: %d", res.StatusCode)
	}
	if got := body["transactions"].(map[string]any)["total_count"].(float64); got != 3 {
		t.Errorf("may range total = %v, want 3", got)
	}
	// Single day 05-01: 1 tx (the seeded credit; debit is 05-02)
	res, body = owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary?from=2024-05-01&to=2024-05-01", nil)
	if got := body["transactions"].(map[string]any)["total_count"].(float64); got != 1 {
		t.Errorf("day total = %v, want 1", got)
	}
	// Bad date → 400
	res, _ = owner.do("GET", "/api/v1/organizations/"+orgID+"/reports/summary?from=not-a-date", nil)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad from: got %d, want 400", res.StatusCode)
	}
	// Export respects range
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/organizations/"+orgID+"/reports/export?kind=transactions&from=2024-05-01&to=2024-05-06", nil)
	req.Header.Add("Cookie", owner.cookie)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Errorf("range export: %d", res2.StatusCode)
	}
}
