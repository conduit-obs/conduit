package api

import (
	"net/http"

	"github.com/conduit-obs/conduit/internal/auth"
)

// Gateway is the API gateway that routes requests.
type Gateway struct {
	mux      *http.ServeMux
	handlers *Handlers
}

// NewGateway creates a new API gateway with all routes configured.
func NewGateway(handlers *Handlers, validator *auth.JWTValidator, enforcer *auth.RBACEnforcer) *Gateway {
	gw := &Gateway{
		mux:      http.NewServeMux(),
		handlers: handlers,
	}

	apiKeyMW := APIKeyMiddleware(handlers.repo)
	authMW := CombinedAuthMiddleware(validator, apiKeyMW)

	// Rate limiting middleware (applied after auth so tenant is set)
	rateLimitMW := RateLimitMiddleware(handlers.GetRateLimiter(), handlers.GetTenantRateLimit)

	// Public routes
	gw.mux.HandleFunc("GET /healthz", handlers.HealthCheck)

	// Auth (public — no auth required)
	gw.mux.HandleFunc("GET /api/v1/auth/status", handlers.AuthStatus)
	gw.mux.HandleFunc("POST /api/v1/auth/setup", handlers.AuthSetup)
	gw.mux.HandleFunc("POST /api/v1/auth/login", handlers.AuthLogin)
	gw.mux.HandleFunc("POST /api/v1/auth/refresh", handlers.AuthRefresh)
	gw.mux.HandleFunc("POST /api/v1/auth/accept-invite", handlers.AcceptInvite)
	gw.mux.HandleFunc("GET /api/v1/auth/config", handlers.Auth0Config)
	gw.mux.HandleFunc("POST /api/v1/auth/callback", handlers.Auth0Callback)

	// Users (authenticated)
	gw.mux.Handle("GET /api/v1/users", chain(http.HandlerFunc(handlers.ListUsers), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("POST /api/v1/users/invite", chain(http.HandlerFunc(handlers.InviteUser), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("PATCH /api/v1/users/{id}", chain(http.HandlerFunc(handlers.UpdateUserHandler), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Organizations & Projects
	gw.mux.Handle("GET /api/v1/organizations", chain(http.HandlerFunc(handlers.ListOrganizations), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("POST /api/v1/organizations", chain(http.HandlerFunc(handlers.CreateOrganization), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("GET /api/v1/organizations/{id}/projects", chain(http.HandlerFunc(handlers.ListProjects), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("POST /api/v1/organizations/{id}/projects", chain(http.HandlerFunc(handlers.CreateProject), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Frontend config (public)
	gw.mux.HandleFunc("GET /api/v1/config/frontend", handlers.FrontendConfig)

	// Protected routes — Agents (supports ?capability= and ?min_health= filters)
	gw.mux.Handle("GET /api/v1/agents", chain(
		http.HandlerFunc(handlers.ListAgentsWithHealthFilter),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/agents", chain(
		http.HandlerFunc(handlers.RegisterAgent),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsWrite),
		rateLimitMW,
	))
	gw.mux.Handle("PATCH /api/v1/agents/{id}", chain(
		http.HandlerFunc(handlers.UpdateAgentLabels),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsWrite),
		rateLimitMW,
	))

	gw.mux.Handle("DELETE /api/v1/agents/{id}", chain(
		http.HandlerFunc(handlers.DeregisterAgent),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsWrite),
		rateLimitMW,
	))

	// Config (compile supports inheritance)
	gw.mux.Handle("POST /api/v1/config/compile", chain(
		http.HandlerFunc(handlers.CompileIntentWithInheritance),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/config/intents", chain(
		http.HandlerFunc(handlers.CreateConfigIntent),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/config/intents", chain(
		http.HandlerFunc(handlers.ListConfigIntents),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/config/validate", chain(
		http.HandlerFunc(handlers.ValidateIntent),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/config/intents/{name}/promote", chain(
		http.HandlerFunc(handlers.PromoteConfigIntent),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))

	// Config intent tags (PATCH)
	gw.mux.Handle("PATCH /api/v1/config/intents/{id}/tags", chain(
		http.HandlerFunc(handlers.UpdateConfigIntentTags),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))

	// Config intent export/import
	gw.mux.Handle("GET /api/v1/config/intents/{name}/export", chain(
		http.HandlerFunc(handlers.ExportConfigIntent),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/config/intents/import", chain(
		http.HandlerFunc(handlers.ImportConfigIntent),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))

	// Fleets
	gw.mux.Handle("POST /api/v1/fleets", chain(
		http.HandlerFunc(handlers.CreateFleet),
		authMW,
		RequirePermission(enforcer, auth.PermFleetsWrite),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/fleets", chain(
		http.HandlerFunc(handlers.ListFleets),
		authMW,
		RequirePermission(enforcer, auth.PermFleetsRead),
		rateLimitMW,
	))

	// Fleets — membership
	gw.mux.Handle("GET /api/v1/fleets/{id}/agents", chain(
		http.HandlerFunc(handlers.GetFleetAgents),
		authMW,
		RequirePermission(enforcer, auth.PermFleetsRead),
		rateLimitMW,
	))

	// Rollouts (supports scheduled_at for scheduled rollouts)
	gw.mux.Handle("POST /api/v1/rollouts", chain(
		http.HandlerFunc(handlers.CreateScheduledRollout),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsWrite),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/rollouts", chain(
		http.HandlerFunc(handlers.ListRollouts),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsRead),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/rollouts/{id}", chain(
		http.HandlerFunc(handlers.GetRollout),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsRead),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/rollouts/{id}/history", chain(
		http.HandlerFunc(handlers.GetRolloutHistory),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsRead),
		rateLimitMW,
	))

	// Rollout lifecycle
	gw.mux.Handle("POST /api/v1/rollouts/{id}/pause", chain(
		http.HandlerFunc(handlers.PauseRollout),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsWrite),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/rollouts/{id}/resume", chain(
		http.HandlerFunc(handlers.ResumeRollout),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsWrite),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/rollouts/{id}/cancel", chain(
		http.HandlerFunc(handlers.CancelRollout),
		authMW,
		RequirePermission(enforcer, auth.PermRolloutsWrite),
		rateLimitMW,
	))

	// Config diff
	gw.mux.Handle("GET /api/v1/config/intents/{name}/diff", chain(
		http.HandlerFunc(handlers.DiffConfigIntents),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))

	// Tenants (admin only)
	gw.mux.Handle("POST /api/v1/tenants", chain(
		http.HandlerFunc(handlers.CreateTenant),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/tenants/{id}", chain(
		http.HandlerFunc(handlers.GetTenant),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))

	// Tenant usage stats (rate limiting)
	gw.mux.Handle("GET /api/v1/tenants/{id}/usage", chain(
		http.HandlerFunc(handlers.GetTenantUsage),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
	))

	// Agent config history
	gw.mux.Handle("GET /api/v1/agents/{id}/config-history", chain(
		http.HandlerFunc(handlers.GetAgentConfigHistory),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsRead),
		rateLimitMW,
	))

	// API Keys
	gw.mux.Handle("POST /api/v1/api-keys", chain(
		http.HandlerFunc(handlers.CreateAPIKey),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/api-keys", chain(
		http.HandlerFunc(handlers.ListAPIKeys),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))

	// Batch operations
	gw.mux.Handle("POST /api/v1/batch", chain(
		http.HandlerFunc(handlers.ExecuteBatch),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsWrite),
		rateLimitMW,
	))

	// Webhooks
	gw.mux.Handle("POST /api/v1/webhooks", chain(
		http.HandlerFunc(handlers.CreateWebhook),
		authMW,
		RequirePermission(enforcer, auth.PermEventsRead),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/webhooks", chain(
		http.HandlerFunc(handlers.ListWebhooks),
		authMW,
		RequirePermission(enforcer, auth.PermEventsRead),
		rateLimitMW,
	))
	gw.mux.Handle("DELETE /api/v1/webhooks/{id}", chain(
		http.HandlerFunc(handlers.DeleteWebhook),
		authMW,
		RequirePermission(enforcer, auth.PermEventsRead),
		rateLimitMW,
	))

	// Topology
	gw.mux.Handle("GET /api/v1/topology", chain(
		http.HandlerFunc(handlers.GetTopology),
		authMW,
		RequirePermission(enforcer, auth.PermAgentsRead),
		rateLimitMW,
	))

	// Pipeline Templates
	gw.mux.Handle("GET /api/v1/templates", chain(
		http.HandlerFunc(handlers.ListTemplates),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/templates", chain(
		http.HandlerFunc(handlers.CreateTemplate),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/templates/{name}", chain(
		http.HandlerFunc(handlers.GetTemplate),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/templates/{name}/versions", chain(
		http.HandlerFunc(handlers.GetTemplateVersions),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("PATCH /api/v1/templates/{name}/deprecate", chain(
		http.HandlerFunc(handlers.DeprecateTemplate),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))

	// Policy Packs
	gw.mux.Handle("GET /api/v1/policy-packs", chain(
		http.HandlerFunc(handlers.ListPolicyPacks),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/policy-packs", chain(
		http.HandlerFunc(handlers.CreatePolicyPack),
		authMW,
		RequirePermission(enforcer, auth.PermConfigWrite),
		rateLimitMW,
	))
	gw.mux.Handle("GET /api/v1/policy-packs/{name}", chain(
		http.HandlerFunc(handlers.GetPolicyPack),
		authMW,
		RequirePermission(enforcer, auth.PermConfigRead),
		rateLimitMW,
	))

	// Feature Flags (admin)
	gw.mux.Handle("GET /api/v1/feature-flags", chain(
		http.HandlerFunc(handlers.ListFeatureFlags),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))
	gw.mux.Handle("POST /api/v1/feature-flags", chain(
		http.HandlerFunc(handlers.CreateFeatureFlag),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))
	gw.mux.Handle("PATCH /api/v1/feature-flags/{name}", chain(
		http.HandlerFunc(handlers.UpdateFeatureFlag),
		authMW,
		RequirePermission(enforcer, auth.PermTenantsAdmin),
		rateLimitMW,
	))

	// Quotas
	gw.mux.Handle("GET /api/v1/tenants/{id}/quotas", chain(http.HandlerFunc(handlers.GetTenantQuotas), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))
	gw.mux.Handle("PATCH /api/v1/tenants/{id}/quotas", chain(http.HandlerFunc(handlers.UpdateTenantQuotas), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Environments
	gw.mux.Handle("GET /api/v1/environments", chain(http.HandlerFunc(handlers.ListEnvironments), authMW, RequirePermission(enforcer, auth.PermConfigRead), rateLimitMW))
	gw.mux.Handle("POST /api/v1/environments", chain(http.HandlerFunc(handlers.CreateEnvironment), authMW, RequirePermission(enforcer, auth.PermConfigWrite), rateLimitMW))

	// Rollout extensions
	gw.mux.Handle("POST /api/v1/rollouts/{id}/rollback", chain(http.HandlerFunc(handlers.RollbackRollout), authMW, RequirePermission(enforcer, auth.PermRolloutsWrite), rateLimitMW))
	gw.mux.Handle("POST /api/v1/rollouts/{id}/approve", chain(http.HandlerFunc(handlers.ApproveRollout), authMW, RequirePermission(enforcer, auth.PermRolloutsWrite), rateLimitMW))
	gw.mux.Handle("POST /api/v1/rollouts/{id}/reject", chain(http.HandlerFunc(handlers.RejectRollout), authMW, RequirePermission(enforcer, auth.PermRolloutsWrite), rateLimitMW))
	gw.mux.Handle("POST /api/v1/rollouts/dry-run", chain(http.HandlerFunc(handlers.RolloutDryRun), authMW, RequirePermission(enforcer, auth.PermRolloutsRead), rateLimitMW))

	// Custom Roles
	gw.mux.Handle("GET /api/v1/roles", chain(http.HandlerFunc(handlers.ListCustomRoles), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin), rateLimitMW))
	gw.mux.Handle("POST /api/v1/roles", chain(http.HandlerFunc(handlers.CreateCustomRole), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin), rateLimitMW))
	gw.mux.Handle("DELETE /api/v1/roles/{name}", chain(http.HandlerFunc(handlers.DeleteCustomRole), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin), rateLimitMW))

	// OIDC Auth Flow
	gw.mux.HandleFunc("GET /api/v1/auth/oidc/authorize", handlers.OIDCAuthorize)
	gw.mux.HandleFunc("GET /api/v1/auth/oidc/callback", handlers.OIDCCallback)

	// Drift Remediation
	gw.mux.Handle("POST /api/v1/agents/{id}/remediate", chain(http.HandlerFunc(handlers.RemediateAgent), authMW, RequirePermission(enforcer, auth.PermAgentsWrite), rateLimitMW))
	gw.mux.Handle("POST /api/v1/fleets/{id}/remediate", chain(http.HandlerFunc(handlers.RemediateFleet), authMW, RequirePermission(enforcer, auth.PermFleetsWrite), rateLimitMW))

	// Alert Rules
	gw.mux.Handle("GET /api/v1/alert-rules", chain(http.HandlerFunc(handlers.ListAlertRules), authMW, RequirePermission(enforcer, auth.PermEventsRead), rateLimitMW))
	gw.mux.Handle("POST /api/v1/alert-rules", chain(http.HandlerFunc(handlers.CreateAlertRule), authMW, RequirePermission(enforcer, auth.PermEventsRead), rateLimitMW))

	// SLO & Alerts
	gw.mux.HandleFunc("GET /api/v1/slo", handlers.GetSLO)
	gw.mux.HandleFunc("GET /api/v1/alerts", handlers.GetActiveAlerts)

	// Audit Logs
	gw.mux.Handle("GET /api/v1/audit-logs", chain(http.HandlerFunc(handlers.QueryAuditLogs), authMW, RequirePermission(enforcer, auth.PermEventsRead), rateLimitMW))

	// Usage History
	gw.mux.Handle("GET /api/v1/tenants/{id}/usage/history", chain(http.HandlerFunc(handlers.GetUsageHistory), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Entitlements
	gw.mux.Handle("GET /api/v1/tenants/{id}/entitlements", chain(http.HandlerFunc(handlers.GetEntitlements), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Compliance Export
	gw.mux.Handle("GET /api/v1/compliance/export", chain(http.HandlerFunc(handlers.ExportCompliance), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin), rateLimitMW))

	// GDPR Data Deletion
	gw.mux.Handle("DELETE /api/v1/tenants/{id}/data", chain(http.HandlerFunc(handlers.DeleteTenantData), authMW, RequirePermission(enforcer, auth.PermTenantsAdmin)))

	// Metrics (public)
	gw.mux.HandleFunc("GET /api/v1/metrics", handlers.Metrics)

	// Version (public)
	gw.mux.HandleFunc("GET /api/v1/version", handlers.Version)

	// API Docs (public)
	gw.mux.HandleFunc("GET /api/v1/docs", handlers.DocsHandler)
	gw.mux.HandleFunc("GET /api/v1/docs/openapi.yaml", handlers.OpenAPISpec)

	// Events — WebSocket stream
	gw.mux.Handle("GET /api/v1/events/stream", chain(
		http.HandlerFunc(handlers.EventStream),
		authMW,
		RequirePermission(enforcer, auth.PermEventsRead),
	))

	return gw
}

// ServeHTTP implements the http.Handler interface.
// Wraps all requests with X-Request-ID correlation middleware.
func (gw *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	RequestIDMiddleware(gw.mux).ServeHTTP(w, r)
}

// chain applies middleware in order (outermost first).
func chain(handler http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}
