package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conduit-obs/conduit/internal/config"
)

func TestGateway_CreateAPIKey_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{"name": "test-key"})
	req := httptest.NewRequest("POST", "/api/v1/api-keys", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListAPIKeys_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_GetAgentConfigHistory_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/agents/some-id/config-history", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ExecuteBatch_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"operations": []map[string]any{
			{"op": "register-agent", "data": map[string]any{"name": "agent-1"}},
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ExecuteBatch_RequiresOperations(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{"operations": []any{}})
	req := httptest.NewRequest("POST", "/api/v1/batch", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	// In-memory mode: 503 (DB required) takes precedence over 400
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListAgents_WithCapabilityFilter_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	// Without capability filter, should return normal agent list
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// With capability filter (no repo, falls back to ListAgents)
	req = httptest.NewRequest("GET", "/api/v1/agents?capability=otlp", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_CreateRollout_WithCanaryStrategy_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"fleet_id":       "some-fleet",
		"intent_id":     "some-intent",
		"strategy":      "canary",
		"canary_percent": 10,
	})
	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (requires DB), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_CreateRollout_CanaryInvalidPercent(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"fleet_id":       "some-fleet",
		"intent_id":     "some-intent",
		"strategy":      "canary",
		"canary_percent": 0,
	})
	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	// In in-memory mode, 503 (DB required) takes precedence
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestResolveTemplateVars(t *testing.T) {
	input := `endpoint: {{.endpoint}}
name: {{.name}}`

	vars := map[string]string{
		"endpoint": "https://tempo.example.com:4317",
		"name":     "production",
	}

	result, err := resolveTemplateVars(input, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsString(result, "https://tempo.example.com:4317") {
		t.Error("expected endpoint to be resolved")
	}
	if !containsString(result, "production") {
		t.Error("expected name to be resolved")
	}
}

func TestResolveTemplateVars_MissingVar(t *testing.T) {
	input := `endpoint: {{.missing}}`
	vars := map[string]string{}

	_, err := resolveTemplateVars(input, vars)
	if err == nil {
		t.Error("expected error for missing variable")
	}
}

func TestGateway_APIKeyAuth_NoKey(t *testing.T) {
	gw, _ := setupTestGateway(t)

	// No auth at all
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGateway_APIKeyAuth_InvalidKey(t *testing.T) {
	gw, _ := setupTestGateway(t)

	// API key auth on in-memory mode should return 503
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("X-API-Key", "cdkt_invalid_key_here")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	// In-memory mode returns 503 for API key auth
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// Test that validate endpoint works with intent that would trigger warnings
func TestGateway_ValidateIntent_CompilationCheck(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	intent := config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{
				Name:   "test",
				Signal: "traces",
				Receivers: []config.ReceiverIntent{
					{Type: "otlp", Protocol: "grpc", Endpoint: "0.0.0.0:4317"},
				},
				Processors: []config.ProcessorIntent{
					{Type: "batch"},
				},
				Exporters: []config.ExporterIntent{
					{Type: "otlp", Endpoint: "tempo:4317"},
				},
			},
		},
	}
	body, _ := json.Marshal(intent)

	req := httptest.NewRequest("POST", "/api/v1/config/validate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["valid"] != true {
		t.Error("expected valid=true for well-formed intent")
	}
	// Should have no warnings for well-formed intent with processors and endpoints
	warnings, _ := resp["warnings"].([]any)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %d: %v", len(warnings), warnings)
	}
}
