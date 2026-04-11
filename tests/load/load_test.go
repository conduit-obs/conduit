package load

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/api"
	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/eventbus"
	"github.com/conduit-obs/conduit/internal/opamp"
)

func setupLoadGateway(b *testing.B) (*api.Gateway, string) {
	b.Helper()
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	validator := auth.NewJWTValidator(&privKey.PublicKey, "conduit", "conduit-api")
	enforcer := auth.NewRBACEnforcer(auth.DefaultRoles())
	compiler := config.NewCompiler()
	bus := eventbus.New()
	tracker := opamp.NewHeartbeatTracker(30*time.Second, bus)
	handlers := api.NewHandlers(compiler, tracker, nil, nil, bus, nil)
	gw := api.NewGateway(handlers, validator, enforcer)
	token, _ := auth.IssueToken(privKey, "conduit", "conduit-api", "load-user", "load-tenant", []string{"admin"}, time.Hour)
	return gw, token
}

func BenchmarkParallelAgentRegistration(b *testing.B) {
	gw, token := setupLoadGateway(b)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			body, _ := json.Marshal(map[string]any{
				"name":   fmt.Sprintf("load-agent-%d", i),
				"labels": map[string]string{"env": "load-test"},
			})
			req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d", rec.Code)
			}
			i++
		}
	})
}

func BenchmarkParallelConfigCompile(b *testing.B) {
	gw, token := setupLoadGateway(b)

	intent := config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{
				Name:      "load-test",
				Signal:    "traces",
				Receivers: []config.ReceiverIntent{{Type: "otlp", Protocol: "grpc"}},
				Exporters: []config.ExporterIntent{{Type: "debug"}},
			},
		},
	}
	body, _ := json.Marshal(intent)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("POST", "/api/v1/config/compile", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", rec.Code)
			}
		}
	})
}

func BenchmarkParallelListAgents(b *testing.B) {
	gw, token := setupLoadGateway(b)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/api/v1/agents", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			gw.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", rec.Code)
			}
		}
	})
}
