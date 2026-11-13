package tests

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const xenditSecret = "xendit-callback-token-123"

func createIntegration(t *testing.T, c *client, orgID, provider, secret, defaultAcc string) map[string]any {
	t.Helper()
	payload := map[string]any{
		"provider": provider, "webhook_secret": secret,
	}
	if defaultAcc != "" {
		payload["default_account_id"] = defaultAcc
	}
	res, body := c.do("POST", "/api/v1/organizations/"+orgID+"/integrations", payload)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create integration: %d %v", res.StatusCode, body)
	}
	return body
}

func postWebhook(t *testing.T, c *client, path string, headers map[string]string, payload string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest("POST", c.base+path, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
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

func TestXenditWebhookFlow(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "wh-owner@test.id")
	orgID := createOrg(t, owner, "WH Co")
	accID := createAccount(t, owner, orgID)

	intg := createIntegration(t, owner, orgID, "xendit", xenditSecret, accID)
	whPath := "/api/v1/webhooks/xendit/" + intg["id"].(string)

	payload := `{
		"event": "payment.succeeded",
		"data": {
			"id": "evt-123",
			"amount": 1500000,
			"currency": "IDR",
			"description": "Shopee payout",
			"status": "SUCCEEDED",
			"reference_id": "PAY-99",
			"created": "2024-05-01T10:00:00+07:00"
		}
	}`

	// Bad signature → 401
	res, _ := postWebhook(t, owner, whPath, map[string]string{"X-Callback-Token": "wrong"}, payload)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad token: got %d, want 401", res.StatusCode)
	}

	// Good signature → processed + credit tx minted
	res, body := postWebhook(t, owner, whPath, map[string]string{"X-Callback-Token": xenditSecret}, payload)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("good webhook: %d %v", res.StatusCode, body)
	}
	if body["message"] != "processed" {
		t.Errorf("message = %v, want processed", body["message"])
	}
	res, txBody := owner.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	txs := txBody["data"].([]any)
	if len(txs) != 1 {
		t.Fatalf("tx count = %d, want 1 (%v)", len(txs), body)
	}
	tx0 := txs[0].(map[string]any)
	if tx0["direction"] != "credit" || tx0["amount_minor"].(float64) != 1500000 {
		t.Errorf("tx = %v", tx0)
	}
	if tx0["external_ref"] != "PAY-99" {
		t.Errorf("external_ref = %v", tx0["external_ref"])
	}

	// Duplicate delivery → 200 duplicate, no new tx (event_key dedup)
	res, body = postWebhook(t, owner, whPath, map[string]string{"X-Callback-Token": xenditSecret}, payload)
	if res.StatusCode != http.StatusOK || body["duplicate"] != true {
		t.Errorf("duplicate: %d %v", res.StatusCode, body)
	}
	_, txBody = owner.do("GET", "/api/v1/organizations/"+orgID+"/transactions", nil)
	if n := len(txBody["data"].([]any)); n != 1 {
		t.Errorf("tx count after dup = %d, want 1", n)
	}

	// Unknown integration → 404
	res, _ = postWebhook(t, owner, "/api/v1/webhooks/xendit/00000000-0000-0000-0000-000000000000",
		map[string]string{"X-Callback-Token": xenditSecret}, payload)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown integration: got %d, want 404", res.StatusCode)
	}
	_ = accID
}

func TestMidtransWebhookSignature(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "wh-mt-owner@test.id")
	orgID := createOrg(t, owner, "MT Co")
	accID := createAccount(t, owner, orgID)

	secret := "midtrans-server-key-xyz"
	intg := createIntegration(t, owner, orgID, "midtrans", secret, accID)
	whPath := "/api/v1/webhooks/midtrans/" + intg["id"].(string)

	payload := `{
		"order_id": "ORD-77",
		"status_code": "200",
		"gross_amount": "875000",
		"transaction_status": "settlement",
		"fraud_status": "accept",
		"transaction_time": "2024-05-02 11:00:00",
		"customer_name": "Budi"
	}`

	sum := sha512.Sum512([]byte("ORD-77" + "200" + "875000" + secret))
	sig := hex.EncodeToString(sum[:])

	res, body := postWebhook(t, owner, whPath, map[string]string{"X-Signature": sig}, payload)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("midtrans webhook: %d %v", res.StatusCode, body)
	}
	if body["message"] != "processed" {
		t.Errorf("message = %v", body["message"])
	}

	res, _ = postWebhook(t, owner, whPath, map[string]string{"X-Signature": "deadbeef"}, payload)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad midtrans sig: got %d, want 401", res.StatusCode)
	}
}

func TestIntegrationRBACAndTenant(t *testing.T) {
	srv, _ := newTestServer(t)
	owner := register(t, srv, "intg-owner@test.id")
	orgID := createOrg(t, owner, "Intg Co")

	// Duplicate provider → 409
	createIntegration(t, owner, orgID, "xendit", xenditSecret, "")
	res, _ := owner.do("POST", "/api/v1/organizations/"+orgID+"/integrations", map[string]any{
		"provider": "xendit", "webhook_secret": "another-secret-99",
	})
	if res.StatusCode != http.StatusConflict {
		t.Errorf("dup integration: got %d, want 409", res.StatusCode)
	}

	// viewer: list ok, create forbidden
	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "intg-viewer@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatal("invite")
	}
	viewer := register(t, srv, "intg-viewer@test.id")
	viewer.do("POST", "/api/v1/invitations/accept", map[string]string{"token": inv["invite_token"].(string)})
	res, _ = viewer.do("GET", "/api/v1/organizations/"+orgID+"/integrations", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("viewer list: got %d, want 200", res.StatusCode)
	}
	res, _ = viewer.do("POST", "/api/v1/organizations/"+orgID+"/integrations", map[string]any{
		"provider": "midtrans", "webhook_secret": "viewer-secret-1",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("viewer create: got %d, want 403", res.StatusCode)
	}

	// Cross-tenant → 404
	evil := register(t, srv, "intg-evil@test.id")
	createOrg(t, evil, "Intg Evil")
	res, _ = evil.do("GET", "/api/v1/organizations/"+orgID+"/integrations", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("cross-tenant list: got %d, want 404", res.StatusCode)
	}
}
