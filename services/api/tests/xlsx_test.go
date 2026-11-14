package tests

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/xuri/excelize/v2"
)

func uploadXLSX(t *testing.T, c *client, orgID, accountID string, data []byte) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("account_id", accountID)
	_ = mw.WriteField("mapping", `{"date":"Tanggal","amount":"Nominal","description":"Keterangan","direction":"Jenis","reference":"Ref"}`)
	fw, err := mw.CreateFormFile("file", "statement.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
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

func buildSampleXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetName("Sheet1", "Statement")
	rows := [][]interface{}{
		{"Tanggal", "Keterangan", "Nominal", "Jenis", "Ref"},
		{"2024-06-01", "Marketplace payout", 999000, "masuk", "XLS-1"},
		{"2024-06-02", "Fee platform", -1000, "keluar", "XLS-2"},
	}
	for r, row := range rows {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue("Statement", cell, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestXLSXImportFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "xlsx-owner@test.id")
	orgID := createOrg(t, owner, "XLSX Co")
	accID := createAccount(t, owner, orgID)

	res, prev := uploadXLSX(t, owner, orgID, accID, buildSampleXLSX(t))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("xlsx upload: %d %v", res.StatusCode, prev)
	}
	if prev["valid_count"].(float64) != 2 || prev["invalid_count"].(float64) != 0 {
		t.Errorf("preview: %v", prev)
	}
	batchID := prev["batch_id"].(string)

	res, body := owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+batchID+"/commit", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("commit: %d %v", res.StatusCode, body)
	}
	if body["inserted"].(float64) != 2 {
		t.Errorf("inserted = %v, want 2", body["inserted"])
	}

	res, txBody := owner.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	txs := txBody["data"].([]any)
	if len(txs) != 2 {
		t.Fatalf("tx count = %d, want 2", len(txs))
	}
	first := txs[0].(map[string]any)
	if first["amount_minor"].(float64) != 1000 && first["amount_minor"].(float64) != 999000 {
		t.Errorf("unexpected tx: %v", first)
	}

	// Re-import same xlsx → all duplicates (raw original stored, re-parsed)
	res, prev2 := uploadXLSX(t, owner, orgID, accID, buildSampleXLSX(t))
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("re-upload: %d", res.StatusCode)
	}
	res, body = owner.do("POST", "/api/v1/organizations/"+orgID+"/imports/"+prev2["batch_id"].(string)+"/commit", nil)
	if body["duplicates"].(float64) != 2 || body["inserted"].(float64) != 0 {
		t.Errorf("re-import: %v, want 0 inserted / 2 dup", body)
	}
}

func TestUploadWithoutMappingGuessesColumns(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "guess-owner@test.id")
	orgID := createOrg(t, owner, "Guess Co")
	accID := createAccount(t, owner, orgID)

	// Empty mapping — server must guess columns from the header row.
	csvData := "Tanggal,Keterangan,Nominal,Jenis,Ref\n2024-07-01,Auto guess,5000,masuk,GS-1\n"
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("account_id", accID)
	_ = mw.WriteField("mapping", "{}")
	fw, _ := mw.CreateFormFile("file", "guess.csv")
	_, _ = fw.Write([]byte(csvData))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", owner.base+"/api/v1/organizations/"+orgID+"/imports", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Add("Cookie", owner.cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("guess upload: %d %v", res.StatusCode, out)
	}
	if out["valid_count"].(float64) != 1 {
		t.Errorf("valid_count = %v, want 1 (guess worked)", out["valid_count"])
	}
	m := out["mapping"].(map[string]any)
	if m["date"] != "Tanggal" || m["amount"] != "Nominal" {
		t.Errorf("echoed mapping = %v", m)
	}
}
