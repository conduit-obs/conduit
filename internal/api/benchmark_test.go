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

func BenchmarkHealthCheck(b *testing.B) {
	gw, _ := setupBenchGateway(b)

	req := httptest.NewRequest("GET", "/healthz", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func BenchmarkListAgents(b *testing.B) {
	gw, token := setupBenchGatewayWithToken(b)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

func BenchmarkCreateAgent(b *testing.B) {
	gw, token := setupBenchGatewayWithToken(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body, _ := json.Marshal(map[string]any{
			"name":   "bench-agent",
			"labels": map[string]string{"env": "bench"},
		})
		req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}

func BenchmarkCompileIntent(b *testing.B) {
	gw, token := setupBenchGatewayWithToken(b)

	intent := config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{
				Name:   "bench",
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/api/v1/config/compile", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d", rec.Code)
		}
	}
}

// helpers

func setupBenchGateway(b *testing.B) (*Gateway, *rsa.PrivateKey) {
	b.Helper()
	return setupBenchGatewayInternal(b)
}

func setupBenchGatewayWithToken(b *testing.B) (*Gateway, string) {
	b.Helper()
	gw, privKey := setupBenchGatewayInternal(b)
	token, err := auth.IssueToken(privKey, "conduit", "conduit-api", "bench-user", "bench-tenant", []string{"admin"}, time.Hour)
	if err != nil {
		b.Fatal(err)
	}
	return gw, token
}

func setupBenchGatewayInternal(b *testing.B) (*Gateway, *rsa.PrivateKey) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	validator := auth.NewJWTValidator(&privKey.PublicKey, "conduit", "conduit-api")
	enforcer := auth.NewRBACEnforcer(auth.DefaultRoles())
	compiler := config.NewCompiler()
	bus := eventbus.New()
	tracker := opamp.NewHeartbeatTracker(30*time.Second, bus)
	handlers := NewHandlers(compiler, tracker, nil, nil, bus, nil)
	gw := NewGateway(handlers, validator, enforcer)
	return gw, privKey
}
