package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// ValidateIntent validates an intent document without persisting, returning warnings/errors.
func (h *Handlers) ValidateIntent(w http.ResponseWriter, r *http.Request) {
	var intent config.Intent
	if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var warnings []string
	var errors []string

	// Run validation
	if err := intent.Validate(); err != nil {
		errors = append(errors, err.Error())
	}

	// Additional validation rules
	for i, p := range intent.Pipelines {
		if len(p.Processors) == 0 {
			warnings = append(warnings, fmt.Sprintf("pipeline[%d] %q: no processors defined, data will pass through unprocessed", i, p.Name))
		}
		for j, r := range p.Receivers {
			if r.Endpoint == "" && r.Protocol == "" {
				warnings = append(warnings, fmt.Sprintf("pipeline[%d].receivers[%d]: no endpoint or protocol specified", i, j))
			}
		}
		for j, e := range p.Exporters {
			if e.Endpoint == "" && e.Type != "debug" && e.Type != "logging" {
				warnings = append(warnings, fmt.Sprintf("pipeline[%d].exporters[%d]: no endpoint specified for type %q", i, j, e.Type))
			}
		}
	}

	// Try compilation if no errors
	var compiledYAML string
	if len(errors) == 0 {
		yaml, err := h.compiler.Compile(&intent)
		if err != nil {
			errors = append(errors, fmt.Sprintf("compilation failed: %s", err.Error()))
		} else {
			compiledYAML = yaml
		}
	}

	valid := len(errors) == 0

	w.Header().Set("Content-Type", "application/json")
	if !valid {
		w.WriteHeader(http.StatusBadRequest)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"valid":    valid,
		"errors":   errors,
		"warnings": warnings,
		"yaml":     compiledYAML,
	})
}

// UpdateAgentLabels updates an agent's labels and re-evaluates fleet membership.
func (h *Handlers) UpdateAgentLabels(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, `{"error":"agent id is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Labels == nil {
		http.Error(w, `{"error":"labels are required"}`, http.StatusBadRequest)
		return
	}

	if h.repo == nil {
		h.publishAudit(r.Context(), t.ID, "agent.labels_updated", map[string]any{
			"agent_id": agentID,
			"labels":   req.Labels,
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"agent_id": agentID,
			"labels":   req.Labels,
		})
		return
	}

	agent, err := h.repo.UpdateAgentLabels(r.Context(), t.ID, agentID, req.Labels)
	if err != nil {
		http.Error(w, `{"error":"failed to update agent labels"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "agent.labels_updated", map[string]any{
		"agent_id": agent.ID,
		"labels":   req.Labels,
	})

	// Re-evaluate fleet membership
	fleets, _ := h.repo.ListFleets(r.Context(), t.ID)
	for _, fleet := range fleets {
		var selector map[string]string
		json.Unmarshal([]byte(fleet.Selector), &selector)

		matched := true
		for k, v := range selector {
			if req.Labels[k] != v {
				matched = false
				break
			}
		}

		if matched {
			h.publishAudit(r.Context(), t.ID, "fleet.membership_changed", map[string]any{
				"fleet_id":   fleet.ID,
				"fleet_name": fleet.Name,
				"agent_id":   agent.ID,
				"action":     "matched",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent)
}

// ListRollouts returns rollouts with optional filtering by fleet_id and status.
func (h *Handlers) ListRollouts(w http.ResponseWriter, r *http.Request) {
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

	fleetID := r.URL.Query().Get("fleet_id")
	status := r.URL.Query().Get("status")

	rollouts, err := h.repo.ListRollouts(r.Context(), t.ID, fleetID, status)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if rollouts == nil {
		rollouts = []db.RolloutRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollouts)
}

// GetRolloutHistory returns timestamped status transitions for a rollout.
func (h *Handlers) GetRolloutHistory(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"rollout history requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	rolloutID := r.PathValue("id")
	if rolloutID == "" {
		http.Error(w, `{"error":"rollout id is required"}`, http.StatusBadRequest)
		return
	}

	history, err := h.repo.ListRolloutHistory(r.Context(), t.ID, rolloutID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if history == nil {
		history = []db.RolloutHistoryRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// PromoteConfigIntent promotes the latest version of a config intent.
func (h *Handlers) PromoteConfigIntent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"promotion requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	intentName := r.PathValue("name")
	if intentName == "" {
		http.Error(w, `{"error":"intent name is required"}`, http.StatusBadRequest)
		return
	}

	ci, err := h.repo.PromoteConfigIntent(r.Context(), t.ID, intentName)
	if err != nil {
		http.Error(w, `{"error":"failed to promote intent"}`, http.StatusNotFound)
		return
	}

	h.publishAudit(r.Context(), t.ID, "config_intent.promoted", map[string]any{
		"intent_id": ci.ID,
		"name":      ci.Name,
		"version":   ci.Version,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ci)
}

// CreateTenant creates a new tenant (requires tenants:admin permission).
func (h *Handlers) CreateTenant(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"tenant management requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	tenant, err := h.repo.CreateTenant(r.Context(), req.Name)
	if err != nil {
		http.Error(w, `{"error":"failed to create tenant"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tenant)
}

// GetTenant returns a tenant by ID (requires tenants:admin permission).
func (h *Handlers) GetTenant(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"tenant management requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	tenantID := r.PathValue("id")
	if tenantID == "" {
		http.Error(w, `{"error":"tenant id is required"}`, http.StatusBadRequest)
		return
	}

	t, err := h.repo.GetTenantByID(r.Context(), tenantID)
	if err != nil {
		http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// configHash computes a SHA256 hash of a config string.
func configHash(config string) string {
	h := sha256.Sum256([]byte(config))
	return fmt.Sprintf("%x", h)
}
