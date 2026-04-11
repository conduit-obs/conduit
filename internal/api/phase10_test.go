package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGateway_ListTemplates_IncludesBuiltins(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var templates []map[string]any
	json.NewDecoder(rec.Body).Decode(&templates)

	if len(templates) < 6 {
		t.Errorf("expected at least 6 built-in templates, got %d", len(templates))
	}

	// Verify known template names
	names := make(map[string]bool)
	for _, tmpl := range templates {
		if name, ok := tmpl["name"].(string); ok {
			names[name] = true
		}
	}

	for _, expected := range []string{"otlp-ingestion", "k8s-cluster-telemetry", "redact-pii", "drop-sensitive-attrs", "trace-sampling", "log-routing"} {
		if !names[expected] {
			t.Errorf("missing expected built-in template: %s", expected)
		}
	}
}

func TestGateway_ListTemplates_CategoryFilter(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/templates?category=security", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var templates []map[string]any
	json.NewDecoder(rec.Body).Decode(&templates)

	if len(templates) != 2 {
		t.Errorf("expected 2 security templates, got %d", len(templates))
	}
}

func TestGateway_GetTemplate_Builtin(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/templates/otlp-ingestion", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var tmpl map[string]any
	json.NewDecoder(rec.Body).Decode(&tmpl)

	meta, ok := tmpl["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if meta["name"] != "otlp-ingestion" {
		t.Errorf("expected name 'otlp-ingestion', got %v", meta["name"])
	}
}

func TestGateway_GetTemplate_NotFound(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/templates/nonexistent-template", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGateway_GetTemplateVersions_Builtin(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/templates/redact-pii/versions", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var versions []any
	json.NewDecoder(rec.Body).Decode(&versions)

	if len(versions) != 1 {
		t.Errorf("expected 1 version for built-in, got %d", len(versions))
	}
}

func TestGateway_CreateTemplate_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"operator"})

	body, _ := json.Marshal(map[string]any{
		"metadata": map[string]any{
			"name":        "custom-template",
			"version":     "1.0.0",
			"description": "A custom template",
			"category":    "custom",
		},
		"parameters": []map[string]any{
			{"name": "endpoint", "type": "string", "required": true},
		},
		"intent": map[string]any{
			"signal":    "traces",
			"receivers": []map[string]any{{"type": "otlp"}},
			"exporters": []map[string]any{{"type": "otlp", "config": map[string]any{"endpoint": "{{.endpoint}}"}}},
		},
	})

	req := httptest.NewRequest("POST", "/api/v1/templates", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_DeprecateTemplate_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("PATCH", "/api/v1/templates/some-template/deprecate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d", rec.Code)
	}
}

func TestGateway_ListPolicyPacks_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/policy-packs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGateway_CreatePolicyPack_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"name":    "compliance-pack",
		"version": "1.0.0",
		"templates": []map[string]any{
			{"name": "redact-pii"},
			{"name": "drop-sensitive-attrs"},
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/policy-packs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_GetPolicyPack_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/policy-packs/some-pack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d", rec.Code)
	}
}
