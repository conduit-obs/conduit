package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// --- Organizations ---

func (h *Handlers) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok { http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError); return }
	if h.repo == nil { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode([]any{}); return }
	orgs, err := h.repo.ListOrganizations(r.Context(), t.ID)
	if err != nil { http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError); return }
	if orgs == nil { orgs = []db.OrganizationRow{} }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orgs)
}

func (h *Handlers) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok { http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError); return }
	if h.repo == nil { http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable); return }
	var req struct { Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest); return
	}
	org, err := h.repo.CreateOrganization(r.Context(), t.ID, req.Name)
	if err != nil { http.Error(w, `{"error":"failed to create organization"}`, http.StatusInternalServerError); return }
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(org)
}

// --- Projects ---

func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok { http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError); return }
	if h.repo == nil { w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode([]any{}); return }
	orgID := r.PathValue("id")
	projects, err := h.repo.ListProjects(r.Context(), t.ID, orgID)
	if err != nil { http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError); return }
	if projects == nil { projects = []db.ProjectRow{} }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}

func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok { http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError); return }
	if h.repo == nil { http.Error(w, `{"error":"requires database mode"}`, http.StatusServiceUnavailable); return }
	orgID := r.PathValue("id")
	var req struct { Name string `json:"name"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest); return
	}
	project, err := h.repo.CreateProject(r.Context(), t.ID, orgID, req.Name)
	if err != nil { http.Error(w, `{"error":"failed to create project"}`, http.StatusInternalServerError); return }
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(project)
}

// --- Frontend Config ---

func (h *Handlers) FrontendConfig(w http.ResponseWriter, r *http.Request) {
	cfg := map[string]any{
		"apiUrl":      envOrDefault("CONDUIT_API_URL", ""),
		"oidcEnabled": os.Getenv("CONDUIT_OIDC_ENABLED") == "true",
		"appTitle":    envOrDefault("CONDUIT_APP_TITLE", "Conduit"),
		"supportUrl":  envOrDefault("CONDUIT_SUPPORT_URL", "https://github.com/conduit-obs/conduit"),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cfg)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
