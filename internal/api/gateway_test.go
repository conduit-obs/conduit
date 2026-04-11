package api

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/eventbus"
	"github.com/conduit-obs/conduit/internal/opamp"
)

func setupTestGateway(t *testing.T) (*Gateway, *rsa.PrivateKey) {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	validator := auth.NewJWTValidator(&privKey.PublicKey, "conduit", "conduit-api")
	enforcer := auth.NewRBACEnforcer(auth.DefaultRoles())
	compiler := config.NewCompiler()
	bus := eventbus.New()
	tracker := opamp.NewHeartbeatTracker(30*time.Second, bus)
	handlers := NewHandlers(compiler, tracker, nil, nil, bus, nil) // nil repo/opamp = in-memory mode

	gw := NewGateway(handlers, validator, enforcer)
	return gw, privKey
}

func issueTestToken(t *testing.T, privKey *rsa.PrivateKey, tenantID string, roles []string) string {
	t.Helper()
	token, err := auth.IssueToken(privKey, "conduit", "conduit-api", "test-user", tenantID, roles, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestGateway_HealthCheck_NoAuth(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGateway_Agents_Returns401WithoutAuth(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGateway_Agents_Returns200WithValidJWT(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_Agents_Returns401WithInvalidToken(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-here")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGateway_Agents_Returns403WithInsufficientPermissions(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	token := issueTestToken(t, privKey, "tenant-123", []string{})
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_CompileIntent_WithValidJWT(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	intent := config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{
				Name:   "test",
				Signal: "traces",
				Receivers: []config.ReceiverIntent{
					{Type: "otlp", Protocol: "grpc"},
				},
				Exporters: []config.ExporterIntent{
					{Type: "debug"},
				},
			},
		},
	}

	body, _ := json.Marshal(intent)
	token := issueTestToken(t, privKey, "tenant-123", []string{"viewer"})

	req := httptest.NewRequest("POST", "/api/v1/config/compile", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)

	yamlOut, ok := resp["yaml"]
	if !ok || yamlOut == "" {
		t.Error("expected yaml in response")
	}

	if !config.IsValidYAML(yamlOut) {
		t.Error("compiled output is not valid YAML")
	}
}

func TestGateway_CreateConfigIntent_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	reqBody := map[string]any{
		"name": "test-intent",
		"intent": config.Intent{
			Version: "1.0",
			Pipelines: []config.PipelineIntent{
				{
					Name:   "default",
					Signal: "traces",
					Receivers: []config.ReceiverIntent{
						{Type: "otlp", Protocol: "grpc"},
					},
					Exporters: []config.ExporterIntent{
						{Type: "debug"},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(reqBody)
	token := issueTestToken(t, privKey, "tenant-123", []string{"operator"})

	req := httptest.NewRequest("POST", "/api/v1/config/intents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["compiled_yaml"] == nil || resp["compiled_yaml"] == "" {
		t.Error("expected compiled_yaml in response")
	}
}
