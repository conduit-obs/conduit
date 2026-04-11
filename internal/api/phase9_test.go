package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGateway_Version(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["version"] == "" {
		t.Error("expected non-empty version")
	}
	if resp["go_version"] == "" {
		t.Error("expected non-empty go_version")
	}
	if _, ok := resp["build_time"]; !ok {
		t.Error("expected build_time in version response")
	}
	if _, ok := resp["git_commit"]; !ok {
		t.Error("expected git_commit in version response")
	}
}

func TestGateway_Version_HasRequestID(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header on version endpoint")
	}
}
