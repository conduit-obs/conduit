package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGateway_RegisterAgent_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]any{
		"name":   "collector-1",
		"labels": map[string]string{"env": "prod", "region": "us-east-1"},
	})
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["name"] != "collector-1" {
		t.Errorf("expected name collector-1, got %v", resp["name"])
	}
	if resp["cert_hint"] == nil || resp["cert_hint"] == "" {
		t.Error("expected cert_hint in response")
	}
}

func TestGateway_RegisterAgent_RequiresAuth(t *testing.T) {
	gw, _ := setupTestGateway(t)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGateway_RegisterAgent_RequiresWritePermission(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	token := issueTestToken(t, privKey, "tenant-123", []string{"viewer"})

	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGateway_RegisterAgent_RequiresName(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]string{})
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGateway_CreateFleet_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]any{
		"name":     "production",
		"selector": map[string]string{"env": "prod"},
	})
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/fleets", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["name"] != "production" {
		t.Errorf("expected name production, got %v", resp["name"])
	}
}

func TestGateway_ListFleets_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"viewer"})

	req := httptest.NewRequest("GET", "/api/v1/fleets", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp []any
	json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("expected empty list, got %d items", len(resp))
	}
}

func TestGateway_CreateFleet_RequiresAuth(t *testing.T) {
	gw, _ := setupTestGateway(t)

	body, _ := json.Marshal(map[string]any{"name": "test"})
	req := httptest.NewRequest("POST", "/api/v1/fleets", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGateway_CreateRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]string{
		"fleet_id":  "some-fleet-id",
		"intent_id": "some-intent-id",
	})
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	// Rollouts require a database, so should return 503
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_CreateRollout_RequiresFields(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	body, _ := json.Marshal(map[string]string{})
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
