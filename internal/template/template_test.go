package template

import (
	"testing"
)

func TestBuiltinTemplates_Count(t *testing.T) {
	builtins := BuiltinTemplates()
	if len(builtins) != 6 {
		t.Errorf("expected 6 built-in templates, got %d", len(builtins))
	}
}

func TestBuiltinTemplates_Names(t *testing.T) {
	expected := map[string]bool{
		"otlp-ingestion":        false,
		"k8s-cluster-telemetry": false,
		"redact-pii":            false,
		"drop-sensitive-attrs":  false,
		"trace-sampling":        false,
		"log-routing":           false,
	}

	for _, tmpl := range BuiltinTemplates() {
		if _, ok := expected[tmpl.Metadata.Name]; ok {
			expected[tmpl.Metadata.Name] = true
		} else {
			t.Errorf("unexpected built-in template: %s", tmpl.Metadata.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing built-in template: %s", name)
		}
	}
}

func TestBuiltinTemplates_HaveMetadata(t *testing.T) {
	for _, tmpl := range BuiltinTemplates() {
		if tmpl.Metadata.Name == "" {
			t.Error("template has empty name")
		}
		if tmpl.Metadata.Version == "" {
			t.Errorf("template %s has empty version", tmpl.Metadata.Name)
		}
		if tmpl.Metadata.Description == "" {
			t.Errorf("template %s has empty description", tmpl.Metadata.Name)
		}
		if tmpl.Metadata.Category == "" {
			t.Errorf("template %s has empty category", tmpl.Metadata.Name)
		}
	}
}

func TestBuiltinTemplates_HaveParameters(t *testing.T) {
	for _, tmpl := range BuiltinTemplates() {
		if len(tmpl.Parameters) == 0 {
			t.Errorf("template %s has no parameters", tmpl.Metadata.Name)
		}
		for _, p := range tmpl.Parameters {
			if p.Name == "" {
				t.Errorf("template %s has parameter with empty name", tmpl.Metadata.Name)
			}
			if p.Type == "" {
				t.Errorf("template %s parameter %s has empty type", tmpl.Metadata.Name, p.Name)
			}
		}
	}
}

func TestBuiltinTemplates_HaveIntent(t *testing.T) {
	for _, tmpl := range BuiltinTemplates() {
		if len(tmpl.Intent.Receivers) == 0 {
			t.Errorf("template %s has no receivers", tmpl.Metadata.Name)
		}
		if len(tmpl.Intent.Exporters) == 0 {
			t.Errorf("template %s has no exporters", tmpl.Metadata.Name)
		}
	}
}

func TestRegistry_CRUD(t *testing.T) {
	reg := NewRegistry()

	// Register
	reg.Register(PipelineTemplate{
		Metadata:   TemplateMetadata{Name: "test", Version: "1.0.0", Description: "Test", Category: "testing"},
		Parameters: []ParameterDef{{Name: "p1", Type: "string"}},
	})

	if reg.Count() != 1 {
		t.Errorf("expected count 1, got %d", reg.Count())
	}

	// Get
	tmpl, ok := reg.Get("test")
	if !ok {
		t.Fatal("expected to find template")
	}
	if tmpl.Metadata.Name != "test" {
		t.Errorf("expected name 'test', got %s", tmpl.Metadata.Name)
	}

	// Not found
	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent template")
	}

	// List
	list := reg.List()
	if len(list) != 1 {
		t.Errorf("expected 1 template in list, got %d", len(list))
	}

	// Delete
	err := reg.Delete("test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if reg.Count() != 0 {
		t.Error("expected count 0 after delete")
	}

	// Delete nonexistent
	err = reg.Delete("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent template")
	}
}

func TestRegistry_Versioning(t *testing.T) {
	reg := NewRegistry()

	reg.Register(PipelineTemplate{
		Metadata: TemplateMetadata{Name: "test", Version: "1.0.0", Category: "testing"},
	})
	reg.Register(PipelineTemplate{
		Metadata: TemplateMetadata{Name: "test", Version: "2.0.0", Category: "testing"},
	})

	// Get returns latest
	tmpl, ok := reg.Get("test")
	if !ok || tmpl.Metadata.Version != "2.0.0" {
		t.Errorf("expected latest version 2.0.0, got %s", tmpl.Metadata.Version)
	}

	// GetVersion returns specific
	tmpl, ok = reg.GetVersion("test", "1.0.0")
	if !ok || tmpl.Metadata.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0")
	}

	// GetVersions returns all
	versions := reg.GetVersions("test")
	if len(versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(versions))
	}
}

func TestRegistry_Search(t *testing.T) {
	reg := NewRegistry()
	for _, b := range BuiltinTemplates() {
		reg.Register(b)
	}

	results := reg.Search("otlp")
	if len(results) == 0 {
		t.Error("expected search results for 'otlp'")
	}

	results = reg.Search("security")
	if len(results) < 2 {
		t.Errorf("expected at least 2 security templates, got %d", len(results))
	}

	results = reg.Search("nonexistent-xyz")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRegistry_ListByCategory(t *testing.T) {
	reg := NewRegistry()
	for _, b := range BuiltinTemplates() {
		reg.Register(b)
	}

	results := reg.ListByCategory("security")
	if len(results) != 2 {
		t.Errorf("expected 2 security templates, got %d", len(results))
	}

	results = reg.ListByCategory("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestValidateParameters_Required(t *testing.T) {
	params := []ParameterDef{
		{Name: "endpoint", Type: "string", Required: true},
		{Name: "optional", Type: "string", Required: false},
	}

	// Missing required
	result := ValidateParameters(params, map[string]any{})
	if result.Valid {
		t.Error("expected validation failure for missing required param")
	}
	if len(result.Errors) != 1 || result.Errors[0].Field != "endpoint" {
		t.Errorf("expected error on 'endpoint', got %v", result.Errors)
	}

	// Provided
	result = ValidateParameters(params, map[string]any{"endpoint": "http://localhost"})
	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
}

func TestValidateParameters_StringConstraints(t *testing.T) {
	minLen := 3
	maxLen := 10
	params := []ParameterDef{
		{Name: "name", Type: "string", Required: true, Validation: &ParameterValidation{
			MinLength: &minLen,
			MaxLength: &maxLen,
			Pattern:   `^[a-z]+$`,
		}},
	}

	// Too short
	result := ValidateParameters(params, map[string]any{"name": "ab"})
	if result.Valid {
		t.Error("expected failure for too-short string")
	}

	// Too long
	result = ValidateParameters(params, map[string]any{"name": "abcdefghijk"})
	if result.Valid {
		t.Error("expected failure for too-long string")
	}

	// Bad pattern
	result = ValidateParameters(params, map[string]any{"name": "ABC123"})
	if result.Valid {
		t.Error("expected failure for pattern mismatch")
	}

	// Valid
	result = ValidateParameters(params, map[string]any{"name": "valid"})
	if !result.Valid {
		t.Errorf("expected valid, got: %v", result.Errors)
	}
}

func TestValidateParameters_NumberConstraints(t *testing.T) {
	min := 1.0
	max := 65535.0
	params := []ParameterDef{
		{Name: "port", Type: "number", Required: true, Validation: &ParameterValidation{Minimum: &min, Maximum: &max}},
	}

	// Below minimum
	result := ValidateParameters(params, map[string]any{"port": 0.0})
	if result.Valid {
		t.Error("expected failure for below minimum")
	}

	// Above maximum
	result = ValidateParameters(params, map[string]any{"port": 70000.0})
	if result.Valid {
		t.Error("expected failure for above maximum")
	}

	// Valid
	result = ValidateParameters(params, map[string]any{"port": 8080.0})
	if !result.Valid {
		t.Errorf("expected valid, got: %v", result.Errors)
	}
}

func TestValidateParameters_EnumConstraint(t *testing.T) {
	params := []ParameterDef{
		{Name: "mode", Type: "string", Required: true, Validation: &ParameterValidation{Enum: []string{"probability", "tail-based", "error-biased"}}},
	}

	result := ValidateParameters(params, map[string]any{"mode": "invalid"})
	if result.Valid {
		t.Error("expected failure for invalid enum value")
	}

	result = ValidateParameters(params, map[string]any{"mode": "probability"})
	if !result.Valid {
		t.Errorf("expected valid, got: %v", result.Errors)
	}
}

func TestValidateParameters_TypeChecking(t *testing.T) {
	params := []ParameterDef{
		{Name: "count", Type: "number", Required: true},
		{Name: "enabled", Type: "boolean", Required: true},
		{Name: "items", Type: "array", Required: true},
	}

	// Wrong types
	result := ValidateParameters(params, map[string]any{
		"count":   "not-a-number",
		"enabled": "not-a-bool",
		"items":   "not-an-array",
	})
	if result.Valid {
		t.Error("expected validation failures for wrong types")
	}
	if len(result.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Correct types
	result = ValidateParameters(params, map[string]any{
		"count":   42.0,
		"enabled": true,
		"items":   []any{"a", "b"},
	})
	if !result.Valid {
		t.Errorf("expected valid, got: %v", result.Errors)
	}
}

func TestPolicyPack_Resolve(t *testing.T) {
	reg := NewRegistry()
	for _, b := range BuiltinTemplates() {
		reg.Register(b)
	}

	pack := &PolicyPack{
		Name:    "compliance",
		Version: "1.0.0",
		Templates: []PolicyPackTemplateRef{
			{Name: "redact-pii"},
			{Name: "drop-sensitive-attrs"},
		},
	}

	resolved, err := pack.Resolve(reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 resolved templates, got %d", len(resolved))
	}
}
