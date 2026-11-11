package auth

// RBAC: permissions are strings; roles map to permission sets.
// Handlers declare required permission; middleware enforces.

const (
	PermOrgManage         = "org:manage"
	PermMemberManage      = "member:manage"
	PermAccountManage     = "account:manage"
	PermImportRun         = "import:run"
	PermReconcileRun      = "reconcile:run"
	PermMatchApprove      = "match:approve"
	PermExceptionResolve  = "exception:resolve"
	PermReportExport      = "report:export"
	PermIntegrationManage = "integration:manage"
	PermAuditView         = "audit:view"
	PermDataView          = "data:view"
)

var rolePerms = map[string]map[string]bool{
	"owner": permSet(
		PermOrgManage, PermMemberManage, PermAccountManage, PermImportRun,
		PermReconcileRun, PermMatchApprove, PermExceptionResolve, PermReportExport,
		PermIntegrationManage, PermAuditView, PermDataView,
	),
	"admin": permSet(
		PermOrgManage, PermMemberManage, PermAccountManage, PermImportRun,
		PermReconcileRun, PermMatchApprove, PermExceptionResolve, PermReportExport,
		PermIntegrationManage, PermAuditView, PermDataView,
	),
	"finance_manager": permSet(
		PermDataView, PermReconcileRun, PermMatchApprove, PermExceptionResolve,
		PermReportExport, PermImportRun, PermAuditView,
	),
	"finance_staff": permSet(
		PermDataView, PermImportRun, PermReconcileRun, PermExceptionResolve, PermReportExport,
	),
	"viewer": permSet(PermDataView, PermReportExport),
}

func permSet(perms ...string) map[string]bool {
	m := make(map[string]bool, len(perms))
	for _, p := range perms {
		m[p] = true
	}
	return m
}

// RoleHasPermission reports whether role includes perm.
func RoleHasPermission(role, perm string) bool {
	set, ok := rolePerms[role]
	return ok && set[perm]
}

// ValidRole reports whether role is a known role name.
func ValidRole(role string) bool {
	_, ok := rolePerms[role]
	return ok
}

// ValidRoles lists role names (for validation messages).
func ValidRoles() []string {
	out := make([]string, 0, len(rolePerms))
	for r := range rolePerms {
		out = append(out, r)
	}
	return out
}
