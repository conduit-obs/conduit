package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/ratelimit"
)

func TestGateway_RequestID_Generated(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	requestID := rec.Header().Get("X-Request-ID")
	if requestID == "" {
		t.Error("expected X-Request-ID header in response")
	}
	if len(requestID) != 32 { // 16 bytes hex encoded
		t.Errorf("expected 32 char request ID, got %d: %s", len(requestID), requestID)
	}
}

func TestGateway_RequestID_Propagated(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-ID", "my-custom-trace-id")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	requestID := rec.Header().Get("X-Request-ID")
	if requestID != "my-custom-trace-id" {
		t.Errorf("expected propagated request ID, got %s", requestID)
	}
}

func TestGateway_CreateConfigIntent_WithTags(t *testing.T) {
	gw, privKey := setupTestGateway(t)

	reqBody := map[string]any{
		"name": "test-tagged-intent",
		"tags": []string{"production", "monitoring"},
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

	tags, ok := resp["tags"].([]any)
	if !ok {
		t.Fatal("expected tags in response")
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}

	// Verify X-Request-ID is in the response
	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID in response headers")
	}
}

func TestGateway_UpdateConfigIntentTags_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{"tags": []string{"staging"}})
	req := httptest.NewRequest("PATCH", "/api/v1/config/intents/some-id/tags", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d", rec.Code)
	}
}

func TestGateway_ExportConfigIntent_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/config/intents/test-intent/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ImportConfigIntent_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"name": "test-import",
		"versions": []map[string]any{
			{"intent_json": `{"version":"1.0"}`, "tags": []string{"prod"}},
		},
	})
	req := httptest.NewRequest("POST", "/api/v1/config/intents/import", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_GetTopology_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/topology", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if _, ok := resp["regions"]; !ok {
		t.Error("expected regions in topology response")
	}
}

func TestGateway_GetTenantUsage(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant-123/usage", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["tenant_id"] != "tenant-123" {
		t.Errorf("expected tenant_id tenant-123, got %v", resp["tenant_id"])
	}
	if resp["rate_limit"] == nil {
		t.Error("expected rate_limit in response")
	}
	if resp["total_requests"] == nil {
		t.Error("expected total_requests in response")
	}
}

func TestGateway_ScheduledRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	body, _ := json.Marshal(map[string]any{
		"fleet_id":     "some-fleet",
		"intent_id":    "some-intent",
		"scheduled_at": "2026-04-10T15:00:00Z",
	})
	req := httptest.NewRequest("POST", "/api/v1/rollouts", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no DB), got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_ListRollouts_StatusFilter(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/rollouts?status=scheduled", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestTokenBucket_AllowAndLimit(t *testing.T) {
	tb := ratelimit.New()

	// Rate limit = 2 req/s
	allowed, _ := tb.Allow("t1", 2)
	if !allowed {
		t.Error("first request should be allowed")
	}

	allowed, _ = tb.Allow("t1", 2)
	if !allowed {
		t.Error("second request should be allowed")
	}

	allowed, retryAfter := tb.Allow("t1", 2)
	if allowed {
		t.Error("third request should be rate limited")
	}
	if retryAfter <= 0 {
		t.Error("retry-after should be positive")
	}

	// Check usage stats
	requests, limited := tb.Usage("t1")
	if requests != 3 {
		t.Errorf("expected 3 total requests, got %d", requests)
	}
	if limited != 1 {
		t.Errorf("expected 1 limited request, got %d", limited)
	}
}

func TestTokenBucket_UnlimitedRate(t *testing.T) {
	tb := ratelimit.New()

	// Rate limit = 0 means unlimited
	for i := 0; i < 100; i++ {
		allowed, _ := tb.Allow("t1", 0)
		if !allowed {
			t.Errorf("request %d should be allowed (unlimited)", i)
		}
	}
}

func TestTokenBucket_MultiTenant(t *testing.T) {
	tb := ratelimit.New()

	// Each tenant has separate buckets
	tb.Allow("t1", 1)
	allowed, _ := tb.Allow("t1", 1)
	if allowed {
		t.Error("t1 should be limited after 1 req/s")
	}

	// t2 should not be affected
	allowed2, _ := tb.Allow("t2", 1)
	if !allowed2 {
		t.Error("t2 should be allowed (separate bucket)")
	}
}

func TestGateway_AllResponsesHaveRequestID(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/healthz"},
		{"GET", "/api/v1/agents"},
		{"GET", "/api/v1/topology"},
		{"GET", "/api/v1/rollouts"},
		{"GET", "/api/v1/tenants/tenant-123/usage"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		if ep.path != "/healthz" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)

		if rec.Header().Get("X-Request-ID") == "" {
			t.Errorf("%s %s: missing X-Request-ID header", ep.method, ep.path)
		}
	}
}

func TestGateway_ListConfigIntents_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	// Test with tag filter (should return empty in memory mode)
	req := httptest.NewRequest("GET", "/api/v1/config/intents?tag=production", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}
