package tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestInvitationAndRBAC(t *testing.T) {
	srv, _ := newTestServer(t)

	owner := register(t, srv, "owner@test.id")
	orgID := createOrg(t, owner, "Invite Co")

	res, inv := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "staff@test.id", "role": "finance_staff",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("invite: got %d body %v", res.StatusCode, inv)
	}
	token, _ := inv["invite_token"].(string)
	if token == "" {
		t.Fatal("no invite_token returned")
	}

	staff := register(t, srv, "staff@test.id")
	res, body := staff.do("POST", "/api/v1/invitations/accept", map[string]string{"token": token})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("accept: got %d body %v", res.StatusCode, body)
	}

	res, _ = staff.do("GET", "/api/v1/organizations/"+orgID+"/members", nil)
	if res.StatusCode != http.StatusOK {
		t.Errorf("staff members: got %d, want 200", res.StatusCode)
	}
	// finance_staff lacks member:manage
	res, _ = staff.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "x@test.id", "role": "viewer",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("staff invite: got %d, want 403", res.StatusCode)
	}

	// invite with different email cannot be accepted by someone else
	inv2res, inv2 := owner.do("POST", "/api/v1/organizations/"+orgID+"/invitations", map[string]string{
		"email": "manager@test.id", "role": "finance_manager",
	})
	if inv2res.StatusCode != http.StatusCreated {
		t.Fatalf("invite2: %d", inv2res.StatusCode)
	}
	stranger := register(t, srv, "stranger@test.id")
	res, _ = stranger.do("POST", "/api/v1/invitations/accept", map[string]string{
		"token": fmt.Sprint(inv2["invite_token"]),
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("wrong-email accept: got %d, want 400", res.StatusCode)
	}

	// last owner cannot be demoted
	res, _ = owner.do("PATCH", "/api/v1/organizations/"+orgID+"/members/role", map[string]string{
		"user_id": userIDFromMembers(t, owner, orgID, "owner@test.id"), "role": "viewer",
	})
	if res.StatusCode != http.StatusConflict {
		t.Errorf("demote last owner: got %d, want 409", res.StatusCode)
	}
}

func userIDFromMembers(t *testing.T, c *client, orgID, email string) string {
	t.Helper()
	res, body := c.do("GET", "/api/v1/organizations/"+orgID+"/members", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("members: %d", res.StatusCode)
	}
	data, _ := body["data"].([]any)
	for _, m := range data {
		mm := m.(map[string]any)
		if strings.EqualFold(fmt.Sprint(mm["email"]), email) {
			return fmt.Sprint(mm["user_id"])
		}
	}
	t.Fatalf("member %s not found", email)
	return ""
}
