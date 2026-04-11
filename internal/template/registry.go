package template

import (
	"fmt"
	"strings"
	"sync"
)

// TemplateRegistry provides in-memory storage and lookup for pipeline templates.
type TemplateRegistry struct {
	mu        sync.RWMutex
	templates map[string][]PipelineTemplate // name -> versions (newest last)
}

// NewRegistry creates a new template registry.
func NewRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string][]PipelineTemplate),
	}
}

// Register adds a template to the registry.
func (r *TemplateRegistry) Register(t PipelineTemplate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[t.Metadata.Name] = append(r.templates[t.Metadata.Name], t)
}

// Get returns the latest version of a template by name.
func (r *TemplateRegistry) Get(name string) (*PipelineTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.templates[name]
	if !ok || len(versions) == 0 {
		return nil, false
	}
	t := versions[len(versions)-1]
	return &t, true
}

// GetVersion returns a specific version of a template.
func (r *TemplateRegistry) GetVersion(name, version string) (*PipelineTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.templates[name]
	if !ok {
		return nil, false
	}
	for _, t := range versions {
		if t.Metadata.Version == version {
			return &t, true
		}
	}
	return nil, false
}

// GetVersions returns all versions of a template.
func (r *TemplateRegistry) GetVersions(name string) []PipelineTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions, ok := r.templates[name]
	if !ok {
		return nil
	}
	result := make([]PipelineTemplate, len(versions))
	copy(result, versions)
	return result
}

// List returns all templates (latest version of each).
func (r *TemplateRegistry) List() []PipelineTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []PipelineTemplate
	for _, versions := range r.templates {
		if len(versions) > 0 {
			result = append(result, versions[len(versions)-1])
		}
	}
	return result
}

// Search returns templates matching the query in name, description, or category.
func (r *TemplateRegistry) Search(query string) []PipelineTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	query = strings.ToLower(query)
	var result []PipelineTemplate
	for _, versions := range r.templates {
		if len(versions) == 0 {
			continue
		}
		t := versions[len(versions)-1]
		if strings.Contains(strings.ToLower(t.Metadata.Name), query) ||
			strings.Contains(strings.ToLower(t.Metadata.Description), query) ||
			strings.Contains(strings.ToLower(t.Metadata.Category), query) {
			result = append(result, t)
		}
	}
	return result
}

// ListByCategory returns templates filtered by category.
func (r *TemplateRegistry) ListByCategory(category string) []PipelineTemplate {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []PipelineTemplate
	for _, versions := range r.templates {
		if len(versions) == 0 {
			continue
		}
		t := versions[len(versions)-1]
		if strings.EqualFold(t.Metadata.Category, category) {
			result = append(result, t)
		}
	}
	return result
}

// Delete removes all versions of a template.
func (r *TemplateRegistry) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.templates[name]; !ok {
		return fmt.Errorf("template %q not found", name)
	}
	delete(r.templates, name)
	return nil
}

// Count returns the total number of unique templates.
func (r *TemplateRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.templates)
}
