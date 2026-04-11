package template

import "time"

// PolicyPackTemplateRef references a template with parameter overrides.
type PolicyPackTemplateRef struct {
	Name       string         `json:"name"`
	Version    string         `json:"version,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// PolicyPack combines multiple templates into a reusable policy pack.
type PolicyPack struct {
	Name        string                  `json:"name"`
	Version     string                  `json:"version"`
	Description string                  `json:"description"`
	Templates   []PolicyPackTemplateRef `json:"templates"`
}

// PolicyPackRow represents a persisted policy pack in the database.
type PolicyPackRow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	PackJSON    string    `json:"pack_json"` // JSON of PolicyPack
	CreatedAt   time.Time `json:"created_at"`
}

// Resolve resolves template references against a registry, returning full templates.
func (pp *PolicyPack) Resolve(registry *TemplateRegistry) ([]PipelineTemplate, error) {
	var resolved []PipelineTemplate
	for _, ref := range pp.Templates {
		var t *PipelineTemplate
		var ok bool
		if ref.Version != "" {
			t, ok = registry.GetVersion(ref.Name, ref.Version)
		} else {
			t, ok = registry.Get(ref.Name)
		}
		if !ok {
			continue
		}
		resolved = append(resolved, *t)
	}
	return resolved, nil
}
