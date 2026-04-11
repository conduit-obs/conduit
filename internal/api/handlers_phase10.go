package api

import (
	"encoding/json"
	"net/http"

	"github.com/conduit-obs/conduit/internal/db"
	tmpl "github.com/conduit-obs/conduit/internal/template"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// ListTemplates returns all pipeline templates. Supports ?category= filter.
// Also includes built-in templates.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	category := r.URL.Query().Get("category")

	// Start with built-in templates
	builtins := tmpl.BuiltinTemplates()
	var result []any
	for _, b := range builtins {
		if category != "" && b.Metadata.Category != category {
			continue
		}
		result = append(result, map[string]any{
			"name":        b.Metadata.Name,
			"version":     b.Metadata.Version,
			"description": b.Metadata.Description,
			"category":    b.Metadata.Category,
			"labels":      b.Metadata.Labels,
			"builtin":     true,
			"parameters":  b.Parameters,
		})
	}

	// Add DB templates if available
	if h.repo != nil {
		var rows []db.TemplateRow
		var err error
		if category != "" {
			rows, err = h.repo.ListTemplatesByCategory(r.Context(), t.ID, category)
		} else {
			rows, err = h.repo.ListTemplates(r.Context(), t.ID)
		}
		if err == nil {
			for _, row := range rows {
				result = append(result, row)
			}
		}
	}

	if result == nil {
		result = []any{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GetTemplate returns the latest version of a template by name.
func (h *Handlers) GetTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"template name is required"}`, http.StatusBadRequest)
		return
	}

	// Check built-in templates first
	for _, b := range tmpl.BuiltinTemplates() {
		if b.Metadata.Name == name {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(b)
			return
		}
	}

	// Check DB
	if h.repo != nil {
		row, err := h.repo.GetTemplate(r.Context(), t.ID, name)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(row)
			return
		}
	}

	http.Error(w, `{"error":"template not found"}`, http.StatusNotFound)
}

// GetTemplateVersions returns all versions of a template.
func (h *Handlers) GetTemplateVersions(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"template name is required"}`, http.StatusBadRequest)
		return
	}

	// Built-in templates only have one version
	for _, b := range tmpl.BuiltinTemplates() {
		if b.Metadata.Name == name {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]any{b})
			return
		}
	}

	if h.repo != nil {
		rows, err := h.repo.ListTemplateVersions(r.Context(), t.ID, name)
		if err == nil && len(rows) > 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rows)
			return
		}
	}

	http.Error(w, `{"error":"template not found"}`, http.StatusNotFound)
}

// CreateTemplate creates a new pipeline template.
func (h *Handlers) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	var req tmpl.PipelineTemplate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Metadata.Name == "" {
		http.Error(w, `{"error":"metadata.name is required"}`, http.StatusBadRequest)
		return
	}
	if req.Metadata.Version == "" {
		req.Metadata.Version = "1.0.0"
	}

	if h.repo == nil {
		// In-memory mode
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(req)
		return
	}

	paramsJSON, _ := json.Marshal(req.Parameters)
	intentJSON, _ := json.Marshal(req.Intent)

	row, err := h.repo.CreateTemplate(r.Context(), t.ID,
		req.Metadata.Name, req.Metadata.Version, req.Metadata.Description,
		req.Metadata.Category, string(paramsJSON), string(intentJSON))
	if err != nil {
		http.Error(w, `{"error":"failed to create template"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "template.created", map[string]any{
		"template_id": row.ID,
		"name":        row.Name,
		"version":     row.Version,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(row)
}

// DeprecateTemplate marks a template as deprecated.
func (h *Handlers) DeprecateTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"deprecation requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"template name is required"}`, http.StatusBadRequest)
		return
	}

	row, err := h.repo.DeprecateTemplate(r.Context(), t.ID, name)
	if err != nil {
		http.Error(w, `{"error":"template not found"}`, http.StatusNotFound)
		return
	}

	h.publishAudit(r.Context(), t.ID, "template.deprecated", map[string]any{
		"template_id": row.ID,
		"name":        row.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(row)
}

// ListPolicyPacks returns all policy packs.
func (h *Handlers) ListPolicyPacks(w http.ResponseWriter, r *http.Request) {
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

	packs, err := h.repo.ListPolicyPacks(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if packs == nil {
		packs = []db.PolicyPackRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(packs)
}

// GetPolicyPack returns a policy pack by name with resolved templates.
func (h *Handlers) GetPolicyPack(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"policy packs require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"policy pack name is required"}`, http.StatusBadRequest)
		return
	}

	pp, err := h.repo.GetPolicyPack(r.Context(), t.ID, name)
	if err != nil {
		http.Error(w, `{"error":"policy pack not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pp)
}

// CreatePolicyPack creates a new policy pack.
func (h *Handlers) CreatePolicyPack(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"policy packs require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req tmpl.PolicyPack
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}

	packJSON, _ := json.Marshal(req)

	pp, err := h.repo.CreatePolicyPack(r.Context(), t.ID, req.Name, req.Version, req.Description, string(packJSON))
	if err != nil {
		http.Error(w, `{"error":"failed to create policy pack"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "policy_pack.created", map[string]any{
		"pack_id": pp.ID,
		"name":    pp.Name,
		"version": pp.Version,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pp)
}
