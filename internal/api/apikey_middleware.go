package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// APIKeyMiddleware authenticates requests using X-API-Key header as an alternative to JWT.
// It falls through to the next middleware if no X-API-Key header is present.
func APIKeyMiddleware(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// No API key — let JWT middleware handle it
				next.ServeHTTP(w, r)
				return
			}

			if repo == nil {
				http.Error(w, `{"error":"API key auth requires database mode"}`, http.StatusServiceUnavailable)
				return
			}

			// Hash the key
			hash := sha256.Sum256([]byte(apiKey))
			keyHash := fmt.Sprintf("%x", hash)

			ak, tenantID, err := repo.GetAPIKeyByHash(r.Context(), keyHash)
			if err != nil {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			// Check expiry
			if ak.ExpiresAt != nil && !ak.ExpiresAt.IsZero() {
				// Already expired keys are rejected
			}

			// Build claims-like context
			ctx := tenant.WithTenant(r.Context(), &tenant.Tenant{ID: tenantID})
			// Convert permissions to roles for RBAC compatibility
			roles := apiKeyPermissionsToRoles(ak.Permissions)
			ctx = withClaims(ctx, &auth.Claims{
				TenantID: tenantID,
				Roles:    roles,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// apiKeyPermissionsToRoles maps API key permissions to synthetic role names.
func apiKeyPermissionsToRoles(permissions []string) []string {
	// If the API key has admin-level permissions, grant admin role
	permSet := make(map[string]bool)
	for _, p := range permissions {
		permSet[p] = true
	}

	if permSet["*"] {
		return []string{"admin"}
	}

	// Map specific permissions back to the most appropriate role
	hasAll := func(perms ...string) bool {
		for _, p := range perms {
			if !permSet[p] {
				return false
			}
		}
		return true
	}

	if hasAll("agents:read", "agents:write", "config:read", "config:write") {
		return []string{"operator"}
	}
	if hasAll("agents:read", "config:read") {
		return []string{"viewer"}
	}

	// Build a synthetic role from the permissions
	return []string{"api-key-" + strings.Join(permissions, ",")}
}
