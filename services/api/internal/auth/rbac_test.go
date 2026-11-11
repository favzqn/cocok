package auth

import "testing"

func TestRolePermissionMatrix(t *testing.T) {
	cases := []struct {
		role string
		perm string
		want bool
	}{
		{"owner", PermOrgManage, true},
		{"owner", PermMemberManage, true},
		{"admin", PermOrgManage, true},
		{"admin", PermIntegrationManage, true},
		{"finance_manager", PermMatchApprove, true},
		{"finance_manager", PermOrgManage, false},
		{"finance_manager", PermMemberManage, false},
		{"finance_staff", PermImportRun, true},
		{"finance_staff", PermMatchApprove, false},
		{"viewer", PermDataView, true},
		{"viewer", PermImportRun, false},
		{"viewer", PermMatchApprove, false},
		{"unknown_role", PermDataView, false},
	}
	for _, c := range cases {
		if got := RoleHasPermission(c.role, c.perm); got != c.want {
			t.Errorf("RoleHasPermission(%q, %q) = %v, want %v", c.role, c.perm, got, c.want)
		}
	}
}

func TestValidRole(t *testing.T) {
	for _, r := range []string{"owner", "admin", "finance_manager", "finance_staff", "viewer"} {
		if !ValidRole(r) {
			t.Errorf("expected %q valid", r)
		}
	}
	if ValidRole("superuser") {
		t.Error("expected superuser invalid")
	}
}
