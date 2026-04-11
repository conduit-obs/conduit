package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conduit-obs/conduit/internal/config"
)

func TestGateway_Metrics(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	cache, ok := resp["config_cache"].(map[string]any)
	if !ok {
		t.Fatal("expected config_cache in metrics")
	}
	if cache["hits"] == nil {
		t.Error("expected hits in config_cache")
	}
	if cache["misses"] == nil {
		t.Error("expected misses in config_cache")
	}
}

func TestGateway_PauseRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/rollouts/some-id/pause", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGateway_ResumeRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/rollouts/some-id/resume", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGateway_CancelRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("POST", "/api/v1/rollouts/some-id/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGateway_CreateWebhook_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"name": "test-hook",
		"url":  "https://example.com/hook",
	})
	req := httptest.NewRequest("POST", "/api/v1/webhooks", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListWebhooks_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/webhooks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestGateway_DeregisterAgent_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("DELETE", "/api/v1/agents/some-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestGateway_CompileWithInheritance(t *testing.T) {
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
				Exporters: []config.ExporterIntent{
					{Type: "debug"},
				},
			},
		},
	}
	body, _ := json.Marshal(intent)

	req := httptest.NewRequest("POST", "/api/v1/config/compile", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListAgents_WithMinHealth(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	// In-memory mode: min_health filter falls through to ListAgents
	req := httptest.NewRequest("GET", "/api/v1/agents?min_health=80", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCachingCompiler(t *testing.T) {
	cc := config.NewCachingCompiler()

	intent := &config.Intent{
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

	// First compile — cache miss
	result1, err := cc.Compile(intent)
	if err != nil {
		t.Fatalf("first compile failed: %v", err)
	}
	if result1 == "" {
		t.Fatal("empty compilation result")
	}

	hits1, misses1 := cc.Stats()
	if hits1 != 0 || misses1 != 1 {
		t.Errorf("expected 0 hits, 1 miss; got %d hits, %d misses", hits1, misses1)
	}

	// Second compile — cache hit
	result2, err := cc.Compile(intent)
	if err != nil {
		t.Fatalf("second compile failed: %v", err)
	}
	if result1 != result2 {
		t.Error("cached result differs from original")
	}

	hits2, misses2 := cc.Stats()
	if hits2 != 1 || misses2 != 1 {
		t.Errorf("expected 1 hit, 1 miss; got %d hits, %d misses", hits2, misses2)
	}
}

func TestIntentMergeBase(t *testing.T) {
	base := &config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{Name: "shared", Signal: "traces", Receivers: []config.ReceiverIntent{{Type: "otlp", Protocol: "grpc"}}, Exporters: []config.ExporterIntent{{Type: "debug"}}},
			{Name: "base-only", Signal: "metrics", Receivers: []config.ReceiverIntent{{Type: "prometheus"}}, Exporters: []config.ExporterIntent{{Type: "debug"}}},
		},
	}

	child := &config.Intent{
		Version: "1.0",
		Pipelines: []config.PipelineIntent{
			{Name: "shared", Signal: "traces", Receivers: []config.ReceiverIntent{{Type: "otlp", Protocol: "http"}}, Exporters: []config.ExporterIntent{{Type: "otlp"}}},
			{Name: "child-only", Signal: "logs", Receivers: []config.ReceiverIntent{{Type: "filelog"}}, Exporters: []config.ExporterIntent{{Type: "debug"}}},
		},
	}

	child.MergeBase(base)

	if len(child.Pipelines) != 3 {
		t.Fatalf("expected 3 pipelines after merge, got %d", len(child.Pipelines))
	}

	// base-only should come from base
	found := false
	for _, p := range child.Pipelines {
		if p.Name == "base-only" {
			found = true
			if p.Signal != "metrics" {
				t.Error("base-only pipeline should retain base signal")
			}
		}
		// "shared" should use child's version (http, not grpc)
		if p.Name == "shared" {
			if p.Receivers[0].Protocol != "http" {
				t.Error("shared pipeline should use child's override")
			}
		}
	}
	if !found {
		t.Error("base-only pipeline not found after merge")
	}
}
