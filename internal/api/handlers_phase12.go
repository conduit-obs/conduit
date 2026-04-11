package api

import (
	"encoding/json"
	"net/http"

	"github.com/conduit-obs/conduit/internal/db"
)

// ListFeatureFlags returns all feature flags.
func (h *Handlers) ListFeatureFlags(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	flags, err := h.repo.ListFeatureFlags(r.Context())
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if flags == nil {
		flags = []db.FeatureFlagRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flags)
}

// CreateFeatureFlag creates a new feature flag.
func (h *Handlers) CreateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"feature flags require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name            string          `json:"name"`
		Description     string          `json:"description"`
		Enabled         bool            `json:"enabled"`
		TenantOverrides map[string]bool `json:"tenant_overrides,omitempty"`
		PercentRollout  int             `json:"percent_rollout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	overridesJSON := "{}"
	if req.TenantOverrides != nil {
		data, _ := json.Marshal(req.TenantOverrides)
		overridesJSON = string(data)
	}

	flag, err := h.repo.CreateFeatureFlag(r.Context(), req.Name, req.Description, req.Enabled, overridesJSON, req.PercentRollout)
	if err != nil {
		http.Error(w, `{"error":"failed to create feature flag"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(flag)
}

// UpdateFeatureFlag updates a feature flag.
func (h *Handlers) UpdateFeatureFlag(w http.ResponseWriter, r *http.Request) {
	if h.repo == nil {
		http.Error(w, `{"error":"feature flags require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"flag name is required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		Enabled         *bool            `json:"enabled,omitempty"`
		PercentRollout  *int             `json:"percent_rollout,omitempty"`
		TenantOverrides *map[string]bool `json:"tenant_overrides,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	var overridesStr *string
	if req.TenantOverrides != nil {
		data, _ := json.Marshal(*req.TenantOverrides)
		s := string(data)
		overridesStr = &s
	}

	flag, err := h.repo.UpdateFeatureFlag(r.Context(), name, req.Enabled, req.PercentRollout, overridesStr)
	if err != nil {
		http.Error(w, `{"error":"feature flag not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(flag)
}
