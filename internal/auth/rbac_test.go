package auth

import "testing"

func TestRBAC_AdminHasAllPermissions(t *testing.T) {
	enforcer := NewRBACEnforcer(DefaultRoles())

	perms := []Permission{
		PermAgentsRead, PermAgentsWrite,
		PermConfigRead, PermConfigWrite,
		PermEventsRead, PermTenantsAdmin,
	}

	for _, p := range perms {
		if !enforcer.HasPermission([]string{"admin"}, p) {
			t.Errorf("admin should have permission %s", p)
		}
	}
}

func TestRBAC_ViewerLimited(t *testing.T) {
	enforcer := NewRBACEnforcer(DefaultRoles())

	if !enforcer.HasPermission([]string{"viewer"}, PermAgentsRead) {
		t.Error("viewer should have agents:read")
	}
	if enforcer.HasPermission([]string{"viewer"}, PermAgentsWrite) {
		t.Error("viewer should not have agents:write")
	}
	if enforcer.HasPermission([]string{"viewer"}, PermTenantsAdmin) {
		t.Error("viewer should not have tenants:admin")
	}
}

func TestRBAC_UnknownRole(t *testing.T) {
	enforcer := NewRBACEnforcer(DefaultRoles())

	if enforcer.HasPermission([]string{"nonexistent"}, PermAgentsRead) {
		t.Error("unknown role should have no permissions")
	}
}

func TestRBAC_MultipleRoles(t *testing.T) {
	enforcer := NewRBACEnforcer(DefaultRoles())

	// viewer + operator combined should have write access
	if !enforcer.HasPermission([]string{"viewer", "operator"}, PermAgentsWrite) {
		t.Error("viewer+operator should have agents:write via operator role")
	}
}
