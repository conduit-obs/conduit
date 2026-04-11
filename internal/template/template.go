package template

import (
	"encoding/json"
	"time"
)

// TemplateMetadata holds identifying information for a pipeline template.
type TemplateMetadata struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// ParameterValidation defines constraints on a parameter value.
type ParameterValidation struct {
	Pattern   string   `json:"pattern,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	Enum      []string `json:"enum,omitempty"`
}

// ParameterDef defines a configurable parameter for a template.
type ParameterDef struct {
	Name         string              `json:"name"`
	Type         string              `json:"type"` // string, number, boolean, array
	Description  string              `json:"description"`
	Required     bool                `json:"required"`
	Default      any                 `json:"default,omitempty"`
	Validation   *ParameterValidation `json:"validation,omitempty"`
	ErrorMessage string              `json:"error_message,omitempty"`
}

// TemplateReceiverSpec defines a receiver in a template intent.
type TemplateReceiverSpec struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// TemplateProcessorSpec defines a processor in a template intent.
type TemplateProcessorSpec struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// TemplateExporterSpec defines an exporter in a template intent.
type TemplateExporterSpec struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// TemplateIntent defines the pipeline configuration the template generates.
type TemplateIntent struct {
	Signal     string                 `json:"signal"` // traces, metrics, logs, all
	Receivers  []TemplateReceiverSpec  `json:"receivers"`
	Processors []TemplateProcessorSpec `json:"processors,omitempty"`
	Exporters  []TemplateExporterSpec  `json:"exporters"`
}

// PipelineTemplate is the top-level template structure.
type PipelineTemplate struct {
	Metadata   TemplateMetadata `json:"metadata"`
	Parameters []ParameterDef   `json:"parameters"`
	Intent     TemplateIntent   `json:"intent"`
}

// TemplateRow represents a persisted template in the database.
type TemplateRow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Parameters  string    `json:"parameters"`  // JSON
	IntentJSON  string    `json:"intent_json"` // JSON
	Deprecated  bool      `json:"deprecated"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToTemplate converts a TemplateRow to a PipelineTemplate.
func (r *TemplateRow) ToTemplate() (*PipelineTemplate, error) {
	t := &PipelineTemplate{
		Metadata: TemplateMetadata{
			Name:        r.Name,
			Version:     r.Version,
			Description: r.Description,
			Category:    r.Category,
		},
	}
	if err := json.Unmarshal([]byte(r.Parameters), &t.Parameters); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(r.IntentJSON), &t.Intent); err != nil {
		return nil, err
	}
	return t, nil
}
