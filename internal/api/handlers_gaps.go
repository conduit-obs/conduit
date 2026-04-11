package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/conduit-obs/conduit/internal/alerting"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/slo"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// --- Shared state for SLO and Alerting (attached to Handlers) ---

var globalSLOTracker = slo.NewTracker()
var globalAlertEngine = alerting.NewEngine()

// --- Quotas ---

func (h *Handlers) GetTenantQuotas(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	tenantID := r.PathValue("id")
	q, err := h.repo.GetTenantQuotas(r.Context(), tenantID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tenant_id": tenantID, "max_agents": 100, "max_fleets": 20, "max_configs": 50, "max_api_keys": 10})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(q)
}

func (h *Handlers) UpdateTenantQuotas(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	tenantID := r.PathValue("id")
	var req struct {
		MaxAgents  int `json:"max_agents"`
		MaxFleets  int `json:"max_fleets"`
		MaxConfigs int `json:"max_configs"`
		MaxAPIKeys int `json:"max_api_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	q, err := h.repo.UpsertTenantQuotas(r.Context(), tenantID, req.MaxAgents, req.MaxFleets, req.MaxConfigs, req.MaxAPIKeys)
	if err != nil {
		http.Error(w, `{"error":"failed to update quotas"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(q)
}

// --- Environments ---

func (h *Handlers) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	envs, err := h.repo.ListEnvironments(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if envs == nil {
		envs = []db.EnvironmentRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(envs)
}

func (h *Handlers) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	env, err := h.repo.CreateEnvironment(r.Context(), t.ID, req.Name)
	if err != nil {
		http.Error(w, `{"error":"failed to create environment"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(env)
}

// --- Rollout Rollback ---

func (h *Handlers) RollbackRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	rolloutID := r.PathValue("id")
	snapshots, err := h.repo.GetRolloutSnapshots(r.Context(), rolloutID)
	if err != nil || len(snapshots) == 0 {
		http.Error(w, `{"error":"no snapshots found for rollback"}`, http.StatusNotFound)
		return
	}
	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "rolled_back", 0)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "rolled_back", "Rollback initiated")
	h.publishAudit(r.Context(), t.ID, "rollout.rolled_back", map[string]any{
		"rollout_id":     rolloutID,
		"agents_affected": len(snapshots),
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "rolled_back",
		"agents_restored": len(snapshots),
	})
}

// --- Rollout Approvals ---

func (h *Handlers) ApproveRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	rolloutID := r.PathValue("id")
	claims, _ := claimsFromContext(r.Context())
	approver := "unknown"
	if claims != nil {
		approver = claims.TenantID
	}
	var req struct {
		Comment string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	var comment *string
	if req.Comment != "" {
		comment = &req.Comment
	}
	approval, err := h.repo.CreateRolloutApproval(r.Context(), t.ID, rolloutID, approver, "approved", comment)
	if err != nil {
		http.Error(w, `{"error":"failed to approve"}`, http.StatusInternalServerError)
		return
	}
	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "in_progress", 0)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "approved", "Rollout approved")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approval)
}

func (h *Handlers) RejectRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	rolloutID := r.PathValue("id")
	var req struct {
		Comment string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	var comment *string
	if req.Comment != "" {
		comment = &req.Comment
	}
	approval, err := h.repo.CreateRolloutApproval(r.Context(), t.ID, rolloutID, "reviewer", "rejected", comment)
	if err != nil {
		http.Error(w, `{"error":"failed to reject"}`, http.StatusInternalServerError)
		return
	}
	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "failed", 0)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "rejected", "Rollout rejected")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(approval)
}

// --- Rollout Dry-Run ---

func (h *Handlers) RolloutDryRun(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		FleetID  string `json:"fleet_id"`
		IntentID string `json:"intent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	fleet, err := h.repo.GetFleet(r.Context(), t.ID, req.FleetID)
	if err != nil {
		http.Error(w, `{"error":"fleet not found"}`, http.StatusNotFound)
		return
	}
	var selector map[string]string
	json.Unmarshal([]byte(fleet.Selector), &selector)
	agents, _ := h.repo.MatchAgentsBySelector(r.Context(), t.ID, selector)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"dry_run":       true,
		"fleet_id":      req.FleetID,
		"intent_id":     req.IntentID,
		"matched_agents": len(agents),
		"fleet_name":    fleet.Name,
	})
}

// --- Custom Roles ---

func (h *Handlers) ListCustomRoles(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	roles, err := h.repo.ListCustomRoles(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if roles == nil {
		roles = []db.CustomRoleRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roles)
}

func (h *Handlers) CreateCustomRole(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	role, err := h.repo.CreateCustomRole(r.Context(), t.ID, req.Name, req.Permissions)
	if err != nil {
		http.Error(w, `{"error":"failed to create role"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(role)
}

func (h *Handlers) DeleteCustomRole(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	name := r.PathValue("name")
	if err := h.repo.DeleteCustomRole(r.Context(), t.ID, name); err != nil {
		http.Error(w, `{"error":"role not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// --- OIDC Auth Flow ---

func (h *Handlers) OIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	issuerURL := r.URL.Query().Get("issuer")
	if issuerURL == "" {
		issuerURL = "https://accounts.google.com"
	}
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = "conduit"
	}
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = "http://localhost:8085/callback"
	}
	authorizeURL := issuerURL + "/authorize?client_id=" + clientID + "&redirect_uri=" + redirectURI + "&response_type=code&scope=openid+profile+email&state=conduit"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"authorize_url": authorizeURL,
		"state":         "conduit",
	})
}

func (h *Handlers) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "callback_received",
		"code":   code,
		"state":  state,
		"note":   "In production, exchange code for tokens and create session",
	})
}

// --- Drift Remediation ---

func (h *Handlers) RemediateAgent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	agentID := r.PathValue("id")
	h.publishAudit(r.Context(), t.ID, "agent.remediated", map[string]any{"agent_id": agentID})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "remediation_initiated", "agent_id": agentID})
}

func (h *Handlers) RemediateFleet(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	fleetID := r.PathValue("id")
	h.publishAudit(r.Context(), t.ID, "fleet.remediated", map[string]any{"fleet_id": fleetID})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "remediation_initiated", "fleet_id": fleetID})
}

// --- Alert Rules ---

func (h *Handlers) ListAlertRules(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	rules, err := h.repo.ListAlertRules(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []db.AlertRuleRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rules)
}

func (h *Handlers) CreateAlertRule(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name      string   `json:"name"`
		Condition string   `json:"condition"`
		Threshold float64  `json:"threshold"`
		Channels  []string `json:"channels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name and condition required"}`, http.StatusBadRequest)
		return
	}
	rule, err := h.repo.CreateAlertRule(r.Context(), t.ID, req.Name, req.Condition, req.Threshold, req.Channels)
	if err != nil {
		http.Error(w, `{"error":"failed to create alert rule"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rule)
}

// --- SLO ---

func (h *Handlers) GetSLO(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"slos": globalSLOTracker.Status()})
}

// --- Active Alerts ---

func (h *Handlers) GetActiveAlerts(w http.ResponseWriter, r *http.Request) {
	alerts := globalAlertEngine.ActiveAlerts()
	if alerts == nil {
		alerts = []alerting.Alert{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

// --- Audit Logs ---

func (h *Handlers) QueryAuditLogs(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	actor := r.URL.Query().Get("actor")
	action := r.URL.Query().Get("action")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	logs, err := h.repo.QueryAuditLogs(r.Context(), t.ID, actor, action, from, to, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []db.AuditLogRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// --- Usage History ---

func (h *Handlers) GetUsageHistory(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	tenantID := r.PathValue("id")
	snapshots, err := h.repo.ListUsageSnapshots(r.Context(), tenantID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if snapshots == nil {
		snapshots = []db.UsageSnapshotRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshots)
}

// --- Entitlements ---

func (h *Handlers) GetEntitlements(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tier": "free", "max_agents": 10, "max_fleets": 5})
		return
	}
	tenantID := r.PathValue("id")
	e, err := h.repo.GetEntitlements(r.Context(), tenantID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"tier": "free", "max_agents": 10, "max_fleets": 5})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(e)
}

// --- Compliance Export ---

func (h *Handlers) ExportCompliance(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	events, err := h.repo.ExportComplianceData(r.Context(), t.ID, from, to)
	if err != nil {
		http.Error(w, `{"error":"export failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"tenant_id": t.ID, "events": events, "from": from, "to": to})
}

// --- GDPR Delete ---

func (h *Handlers) DeleteTenantData(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable)
		return
	}
	tenantID := r.PathValue("id")
	if err := h.repo.DeleteAllTenantData(r.Context(), tenantID); err != nil {
		http.Error(w, `{"error":"deletion failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted", "tenant_id": tenantID})
}
