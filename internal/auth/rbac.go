package auth

// Permission represents an API permission.
type Permission string

const (
	PermAgentsRead    Permission = "agents:read"
	PermAgentsWrite   Permission = "agents:write"
	PermConfigRead    Permission = "config:read"
	PermConfigWrite   Permission = "config:write"
	PermEventsRead     Permission = "events:read"
	PermFleetsRead     Permission = "fleets:read"
	PermFleetsWrite    Permission = "fleets:write"
	PermRolloutsRead   Permission = "rollouts:read"
	PermRolloutsWrite  Permission = "rollouts:write"
	PermTenantsAdmin   Permission = "tenants:admin"
)

// Role defines a named set of permissions.
type Role struct {
	Name        string
	Permissions []Permission
}

// DefaultRoles returns the built-in role definitions.
func DefaultRoles() map[string]Role {
	return map[string]Role{
		"admin": {
			Name: "admin",
			Permissions: []Permission{
				PermAgentsRead, PermAgentsWrite,
				PermConfigRead, PermConfigWrite,
				PermEventsRead,
				PermFleetsRead, PermFleetsWrite,
				PermRolloutsRead, PermRolloutsWrite,
				PermTenantsAdmin,
			},
		},
		"operator": {
			Name: "operator",
			Permissions: []Permission{
				PermAgentsRead, PermAgentsWrite,
				PermConfigRead, PermConfigWrite,
				PermEventsRead,
				PermFleetsRead, PermFleetsWrite,
				PermRolloutsRead, PermRolloutsWrite,
			},
		},
		"viewer": {
			Name: "viewer",
			Permissions: []Permission{
				PermAgentsRead,
				PermConfigRead,
				PermEventsRead,
				PermFleetsRead,
				PermRolloutsRead,
			},
		},
	}
}

// RBACEnforcer checks permissions against roles.
type RBACEnforcer struct {
	roles map[string]Role
}

// NewRBACEnforcer creates an enforcer with the given role definitions.
func NewRBACEnforcer(roles map[string]Role) *RBACEnforcer {
	return &RBACEnforcer{roles: roles}
}

// HasPermission checks if any of the given role names grants the required permission.
func (e *RBACEnforcer) HasPermission(roleNames []string, required Permission) bool {
	for _, name := range roleNames {
		role, ok := e.roles[name]
		if !ok {
			continue
		}
		for _, p := range role.Permissions {
			if p == required {
				return true
			}
		}
	}
	return false
}
