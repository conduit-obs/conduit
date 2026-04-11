package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conduit-obs/conduit/internal/config"
)

func TestGateway_ValidateIntent_Valid(t *testing.T) {
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
				Exporters: []config.ExporterIntent{
					{Type: "debug"},
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
		t.Error("expected valid=true")
	}
	if resp["yaml"] == nil || resp["yaml"] == "" {
		t.Error("expected compiled YAML in response")
	}
}

func TestGateway_ValidateIntent_Invalid(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	intent := config.Intent{} // Missing required fields
	body, _ := json.Marshal(intent)

	req := httptest.NewRequest("POST", "/api/v1/config/validate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["valid"] != false {
		t.Error("expected valid=false")
	}
	errors, ok := resp["errors"].([]any)
	if !ok || len(errors) == 0 {
		t.Error("expected errors in response")
	}
}

func TestGateway_ValidateIntent_WithWarnings(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	intent := config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{
				Name:   "test",
				Signal: "traces",
				Receivers: []config.ReceiverIntent{
					{Type: "otlp", Protocol: "grpc"},
				},
				// No processors — should produce warning
				Exporters: []config.ExporterIntent{
					{Type: "otlp"}, // No endpoint — should produce warning
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
		t.Error("expected valid=true")
	}
	warnings, ok := resp["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Error("expected warnings in response")
	}
}

func TestGateway_UpdateAgentLabels_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"labels": map[string]string{"env": "staging", "region": "us-west-2"},
	})

	req := httptest.NewRequest("PATCH", "/api/v1/agents/some-agent-id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_UpdateAgentLabels_RequiresLabels(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{})

	req := httptest.NewRequest("PATCH", "/api/v1/agents/some-id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListRollouts_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/rollouts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var rollouts []any
	json.NewDecoder(rec.Body).Decode(&rollouts)
	if len(rollouts) != 0 {
		t.Errorf("expected empty list, got %d", len(rollouts))
	}
}

func TestGateway_ListRollouts_WithFilters(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/rollouts?fleet_id=abc&status=completed", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_RolloutHistory_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/rollouts/some-id/history", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_PromoteIntent_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/config/intents/my-intent/promote", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_CreateTenant_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]string{"name": "new-org"})
	req := httptest.NewRequest("POST", "/api/v1/tenants", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_GetTenant_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/tenants/some-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_TenantRoutes_RequireAdminPermission(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"viewer"})

	body, _ := json.Marshal(map[string]string{"name": "test"})
	req := httptest.NewRequest("POST", "/api/v1/tenants", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer role, got %d", rec.Code)
	}
}

func TestGateway_CreateRollout_RequiresPromotedIntent(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	// In in-memory mode, rollout creation returns 503 (requires DB)
	body, _ := json.Marshal(map[string]string{
		"fleet_id":  "some-fleet",
		"intent_id": "some-intent",
	})
	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	// 503 because we're in-memory mode
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
