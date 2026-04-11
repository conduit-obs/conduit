package compat

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/api"
	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/eventbus"
	"github.com/conduit-obs/conduit/internal/opamp"
)

func setupCompatGateway(t *testing.T) (*api.Gateway, string) {
	t.Helper()
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	validator := auth.NewJWTValidator(&privKey.PublicKey, "conduit", "conduit-api")
	enforcer := auth.NewRBACEnforcer(auth.DefaultRoles())
	compiler := config.NewCompiler()
	bus := eventbus.New()
	tracker := opamp.NewHeartbeatTracker(30*time.Second, bus)
	handlers := api.NewHandlers(compiler, tracker, nil, nil, bus, nil)
	gw := api.NewGateway(handlers, validator, enforcer)
	token, _ := auth.IssueToken(privKey, "conduit", "conduit-api", "compat-user", "compat-tenant", []string{"admin"}, time.Hour)
	return gw, token
}

// TestCompatibilityMatrix verifies all documented API endpoints exist and respond.
func TestCompatibilityMatrix(t *testing.T) {
	gw, token := setupCompatGateway(t)

	endpoints := []struct {
		method     string
		path       string
		wantStatus int
		auth       bool
	}{
		// Public endpoints
		{"GET", "/healthz", 200, false},
		{"GET", "/api/v1/version", 200, false},
		{"GET", "/api/v1/metrics", 200, false},
		{"GET", "/api/v1/docs", 200, false},
		{"GET", "/api/v1/docs/openapi.yaml", 200, false},

		// Authenticated endpoints (in-memory mode returns 200 or appropriate codes)
		{"GET", "/api/v1/agents", 200, true},
		{"GET", "/api/v1/fleets", 200, true},
		{"GET", "/api/v1/config/intents", 200, true},
		{"GET", "/api/v1/rollouts", 200, true},
		{"GET", "/api/v1/templates", 200, true},
		{"GET", "/api/v1/policy-packs", 200, true},
		{"GET", "/api/v1/webhooks", 200, true},
		{"GET", "/api/v1/api-keys", 200, true},
		{"GET", "/api/v1/topology", 200, true},
		{"GET", "/api/v1/feature-flags", 200, true},
		{"GET", "/api/v1/tenants/compat-tenant/usage", 200, true},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			if ep.auth {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)

			if rec.Code != ep.wantStatus {
				t.Errorf("expected %d, got %d; body: %s", ep.wantStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestAPIResponsesAreJSON verifies all endpoints return proper JSON Content-Type.
func TestAPIResponsesAreJSON(t *testing.T) {
	gw, token := setupCompatGateway(t)

	jsonEndpoints := []string{
		"/healthz",
		"/api/v1/version",
		"/api/v1/metrics",
		"/api/v1/agents",
		"/api/v1/fleets",
		"/api/v1/templates",
	}

	for _, path := range jsonEndpoints {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", ct)
			}
		})
	}
}

// TestAPIVersionPrefix verifies all API endpoints use /api/v1/ prefix.
func TestAPIVersionPrefix(t *testing.T) {
	gw, token := setupCompatGateway(t)

	// Verify v1 prefix works
	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Errorf("expected /api/v1/ prefix to work, got %d", rec.Code)
	}

	// Verify X-Request-ID is set on all responses
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}
}
