package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGateway_DocsEndpoint(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/docs", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("expected Swagger UI in docs response")
	}
	if !strings.Contains(body, "Conduit API") {
		t.Error("expected 'Conduit API' title in docs response")
	}
}

func TestGateway_OpenAPISpec(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "openapi: 3.0") {
		t.Error("expected OpenAPI 3.0 spec")
	}
	if !strings.Contains(body, "/api/v1/agents") {
		t.Error("expected agents endpoint in spec")
	}
	if !strings.Contains(body, "/api/v1/templates") {
		t.Error("expected templates endpoint in spec")
	}
}
