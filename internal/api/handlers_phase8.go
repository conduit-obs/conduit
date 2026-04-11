package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// UpdateConfigIntentTags updates tags for a config intent (PATCH).
func (h *Handlers) UpdateConfigIntentTags(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"tags require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	intentID := r.PathValue("id")
	if intentID == "" {
		http.Error(w, `{"error":"intent id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	ci, err := h.repo.UpdateConfigIntentTags(r.Context(), t.ID, intentID, req.Tags)
	if err != nil {
		http.Error(w, `{"error":"failed to update tags"}`, http.StatusNotFound)
		return
	}

	h.publishAudit(r.Context(), t.ID, "config_intent.tags_updated", map[string]any{
		"intent_id": ci.ID,
		"name":      ci.Name,
		"tags":      ci.Tags,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ci)
}

// ExportConfigIntent exports a config intent with all versions as a portable JSON bundle.
func (h *Handlers) ExportConfigIntent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"export requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	intentName := r.PathValue("name")
	if intentName == "" {
		http.Error(w, `{"error":"intent name is required"}`, http.StatusBadRequest)
		return
	}

	versions, err := h.repo.GetAllConfigIntentVersions(r.Context(), t.ID, intentName)
	if err != nil || len(versions) == 0 {
		http.Error(w, `{"error":"intent not found"}`, http.StatusNotFound)
		return
	}

	type exportVersion struct {
		Version      int      `json:"version"`
		IntentJSON   string   `json:"intent_json"`
		CompiledYAML *string  `json:"compiled_yaml,omitempty"`
		Promoted     bool     `json:"promoted"`
		Tags         []string `json:"tags"`
		CreatedAt    string   `json:"created_at"`
	}

	var exportVersions []exportVersion
	for _, v := range versions {
		exportVersions = append(exportVersions, exportVersion{
			Version:      v.Version,
			IntentJSON:   v.IntentJSON,
			CompiledYAML: v.CompiledYAML,
			Promoted:     v.Promoted,
			Tags:         v.Tags,
			CreatedAt:    v.CreatedAt.Format(time.RFC3339),
		})
	}

	bundle := map[string]any{
		"name":       intentName,
		"versions":   exportVersions,
		"exported_at": time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bundle)
}

// ImportConfigIntent imports a config intent bundle, creating or merging idempotently.
func (h *Handlers) ImportConfigIntent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"import requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var bundle struct {
		Name     string `json:"name"`
		Versions []struct {
			IntentJSON   string   `json:"intent_json"`
			CompiledYAML *string  `json:"compiled_yaml,omitempty"`
			Tags         []string `json:"tags"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&bundle); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if bundle.Name == "" || len(bundle.Versions) == 0 {
		http.Error(w, `{"error":"name and at least one version are required"}`, http.StatusBadRequest)
		return
	}

	var imported []db.ConfigIntentRow
	for _, v := range bundle.Versions {
		tags := v.Tags
		if tags == nil {
			tags = []string{}
		}
		ci, err := h.repo.CreateConfigIntentWithTags(r.Context(), t.ID, bundle.Name, v.IntentJSON, v.CompiledYAML, tags)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to import version: %s"}`, err.Error()), http.StatusInternalServerError)
			return
		}
		imported = append(imported, *ci)
	}

	h.publishAudit(r.Context(), t.ID, "config_intent.imported", map[string]any{
		"name":             bundle.Name,
		"versions_imported": len(imported),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"name":     bundle.Name,
		"imported": len(imported),
		"versions": imported,
	})
}

// GetTopology returns a hierarchical tree of region>zone>cluster>agents.
func (h *Handlers) GetTopology(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"regions": []any{}})
		return
	}

	agents, err := h.repo.ListAgentsWithTopology(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}

	// Build hierarchical tree: region > zone > cluster > agents
	type agentInfo struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	type clusterNode struct {
		Name   string      `json:"name"`
		Agents []agentInfo `json:"agents"`
	}
	type zoneNode struct {
		Name     string        `json:"name"`
		Clusters []clusterNode `json:"clusters"`
	}
	type regionNode struct {
		Name  string     `json:"name"`
		Zones []zoneNode `json:"zones"`
	}

	regionMap := make(map[string]map[string]map[string][]agentInfo) // region -> zone -> cluster -> agents

	for _, a := range agents {
		var topo map[string]string
		json.Unmarshal([]byte(a.Topology), &topo)

		region := topo["region"]
		if region == "" {
			region = "unknown"
		}
		zone := topo["zone"]
		if zone == "" {
			zone = "unknown"
		}
		cluster := topo["cluster"]
		if cluster == "" {
			cluster = "unknown"
		}

		if regionMap[region] == nil {
			regionMap[region] = make(map[string]map[string][]agentInfo)
		}
		if regionMap[region][zone] == nil {
			regionMap[region][zone] = make(map[string][]agentInfo)
		}
		regionMap[region][zone][cluster] = append(regionMap[region][zone][cluster], agentInfo{
			ID:     a.ID,
			Name:   a.Name,
			Status: a.Status,
		})
	}

	var regions []regionNode
	for rName, zones := range regionMap {
		var zoneNodes []zoneNode
		for zName, clusters := range zones {
			var clusterNodes []clusterNode
			for cName, agts := range clusters {
				clusterNodes = append(clusterNodes, clusterNode{Name: cName, Agents: agts})
			}
			zoneNodes = append(zoneNodes, zoneNode{Name: zName, Clusters: clusterNodes})
		}
		regions = append(regions, regionNode{Name: rName, Zones: zoneNodes})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"regions": regions})
}

// GetTenantUsage returns rate limiting usage stats for a tenant.
func (h *Handlers) GetTenantUsage(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	tenantID := r.PathValue("id")
	if tenantID == "" {
		http.Error(w, `{"error":"tenant id is required"}`, http.StatusBadRequest)
		return
	}

	// Only allow tenants to view their own usage, or admins to view any
	claims, _ := claimsFromContext(r.Context())
	if claims != nil && t.ID != tenantID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	rateLimit := h.GetTenantRateLimit(tenantID)
	requests, limited := h.rateLimiter.Usage(tenantID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"tenant_id":          tenantID,
		"rate_limit":         rateLimit,
		"total_requests":     requests,
		"requests_limited":   limited,
	})
}

// CreateScheduledRollout creates a rollout, optionally scheduled for a future time.
// If scheduled_at is not provided, delegates to CreateRolloutWithStrategy.
func (h *Handlers) CreateScheduledRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	// Read body once so we can re-use it if delegating
	var rawBody bytes.Buffer
	rawBody.ReadFrom(r.Body)
	bodyBytes := rawBody.Bytes()

	var req struct {
		FleetID      string  `json:"fleet_id"`
		IntentID     string  `json:"intent_id"`
		StrategyType string  `json:"strategy"`
		CanaryPct    int     `json:"canary_percent"`
		ScheduledAt  *string `json:"scheduled_at,omitempty"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.FleetID == "" || req.IntentID == "" {
		http.Error(w, `{"error":"fleet_id and intent_id are required"}`, http.StatusBadRequest)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	// Parse scheduled_at if provided — if present, create a scheduled rollout
	var scheduledAt *time.Time
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		t2, err := time.Parse(time.RFC3339, *req.ScheduledAt)
		if err != nil {
			http.Error(w, `{"error":"scheduled_at must be RFC3339 format"}`, http.StatusBadRequest)
			return
		}
		scheduledAt = &t2
	}

	if scheduledAt == nil {
		// No schedule — delegate to normal rollout creation with reconstructed body
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		h.CreateRolloutWithStrategy(w, r)
		return
	}

	fleet, err := h.repo.GetFleet(r.Context(), t.ID, req.FleetID)
	if err != nil {
		http.Error(w, `{"error":"fleet not found"}`, http.StatusNotFound)
		return
	}

	intent, err := h.repo.GetConfigIntentByID(r.Context(), t.ID, req.IntentID)
	if err != nil {
		http.Error(w, `{"error":"intent not found"}`, http.StatusNotFound)
		return
	}

	if !intent.Promoted {
		http.Error(w, `{"error":"only promoted intents can be used in rollouts"}`, http.StatusBadRequest)
		return
	}

	var selector map[string]string
	json.Unmarshal([]byte(fleet.Selector), &selector)

	agents, err := h.repo.MatchAgentsBySelector(r.Context(), t.ID, selector)
	if err != nil {
		http.Error(w, `{"error":"failed to match agents"}`, http.StatusInternalServerError)
		return
	}

	if req.StrategyType == "" {
		req.StrategyType = "all-at-once"
	}

	strategyJSON, _ := json.Marshal(map[string]any{
		"type":           req.StrategyType,
		"canary_percent": req.CanaryPct,
	})

	rollout, err := h.repo.CreateRolloutWithSchedule(r.Context(), t.ID, req.FleetID, req.IntentID, len(agents), string(strategyJSON), scheduledAt)
	if err != nil {
		http.Error(w, `{"error":"failed to create scheduled rollout"}`, http.StatusInternalServerError)
		return
	}

	h.repo.CreateRolloutHistory(r.Context(), t.ID, rollout.ID, "scheduled",
		fmt.Sprintf("Rollout scheduled for %s, targeting %d agents", scheduledAt.Format(time.RFC3339), len(agents)))

	h.publishAudit(r.Context(), t.ID, "rollout.scheduled", map[string]any{
		"rollout_id":   rollout.ID,
		"fleet_id":     req.FleetID,
		"intent_id":    req.IntentID,
		"scheduled_at": scheduledAt.Format(time.RFC3339),
		"target_count": len(agents),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rollout)
}
