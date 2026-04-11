package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/auth"
	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/eventbus"
	"github.com/conduit-obs/conduit/internal/opamp"
)

func setupIntegrationGateway(t *testing.T) (*Gateway, *rsa.PrivateKey, *db.Repo) {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.Migrate(ctx, pool, logger); err != nil {
		t.Fatal(err)
	}

	// Clean tables
	pool.Exec(ctx, "DELETE FROM rollouts")
	pool.Exec(ctx, "DELETE FROM fleets")
	pool.Exec(ctx, "DELETE FROM events")
	pool.Exec(ctx, "DELETE FROM config_intents")
	pool.Exec(ctx, "DELETE FROM agents")
	pool.Exec(ctx, "DELETE FROM user_roles")
	pool.Exec(ctx, "DELETE FROM roles")
	pool.Exec(ctx, "DELETE FROM tenants")

	repo := db.NewRepo(pool)

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	validator := auth.NewJWTValidator(&privKey.PublicKey, "conduit", "conduit-api")
	enforcer := auth.NewRBACEnforcer(auth.DefaultRoles())
	compiler := config.NewCompiler()
	bus := eventbus.New()
	tracker := opamp.NewHeartbeatTracker(30*time.Second, bus)
	handlers := NewHandlers(compiler, tracker, repo, nil, bus, logger)
	gw := NewGateway(handlers, validator, enforcer)

	return gw, privKey, repo
}

func TestIntegration_HealthCheck(t *testing.T) {
	gw, _, _ := setupIntegrationGateway(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestIntegration_AgentsWithoutAuth_Returns401(t *testing.T) {
	gw, _, _ := setupIntegrationGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestIntegration_AgentsWithJWT_Returns200(t *testing.T) {
	gw, privKey, repo := setupIntegrationGateway(t)
	ctx := context.Background()

	// Create a tenant and agent in the DB
	tenant, err := repo.CreateTenant(ctx, "integ-test-org")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateAgent(ctx, tenant.ID, "collector-integ", map[string]string{"env": "test"})
	if err != nil {
		t.Fatal(err)
	}

	token, _ := auth.IssueToken(privKey, "conduit", "conduit-api", "user-1", tenant.ID, []string{"admin"}, time.Hour)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var agents []db.AgentRow
	json.NewDecoder(rec.Body).Decode(&agents)
	if len(agents) != 1 {
		t.Errorf("expected 1 agent from Postgres, got %d", len(agents))
	}
	if len(agents) > 0 && agents[0].Name != "collector-integ" {
		t.Errorf("expected collector-integ, got %s", agents[0].Name)
	}
}

func TestIntegration_CreateAndListConfigIntents(t *testing.T) {
	gw, privKey, repo := setupIntegrationGateway(t)
	ctx := context.Background()

	tenant, err := repo.CreateTenant(ctx, "intent-integ-org")
	if err != nil {
		t.Fatal(err)
	}

	token, _ := auth.IssueToken(privKey, "conduit", "conduit-api", "user-1", tenant.ID, []string{"operator"}, time.Hour)

	// POST /api/v1/config/intents
	reqBody := map[string]any{
		"name": "my-pipeline",
		"intent": config.Intent{
			Version: "1.0",
			Pipelines: []config.PipelineIntent{
				{
					Name:   "default",
					Signal: "traces",
					Receivers: []config.ReceiverIntent{
						{Type: "otlp", Protocol: "grpc", Endpoint: "0.0.0.0:4317"},
					},
					Exporters: []config.ExporterIntent{
						{Type: "otlp", Endpoint: "tempo:4317"},
					},
				},
			},
		},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/config/intents", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var createResp db.ConfigIntentRow
	json.NewDecoder(rec.Body).Decode(&createResp)

	if createResp.CompiledYAML == nil || *createResp.CompiledYAML == "" {
		t.Error("expected compiled YAML in response")
	}
	if !config.IsValidYAML(*createResp.CompiledYAML) {
		t.Error("compiled YAML is not valid")
	}
	if createResp.Version != 1 {
		t.Errorf("expected version 1, got %d", createResp.Version)
	}

	// GET /api/v1/config/intents
	req = httptest.NewRequest("GET", "/api/v1/config/intents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var intents []db.ConfigIntentRow
	json.NewDecoder(rec.Body).Decode(&intents)
	if len(intents) != 1 {
		t.Errorf("expected 1 intent, got %d", len(intents))
	}
}
