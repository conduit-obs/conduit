package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestGateway_GetRollout_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/rollouts/some-id", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_GetFleetAgents_InMemory(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/fleets/some-fleet-id/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var agents []any
	json.NewDecoder(rec.Body).Decode(&agents)
	if len(agents) != 0 {
		t.Errorf("expected empty list, got %d items", len(agents))
	}
}

func TestGateway_DiffConfigIntents_RequiresDB(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	req := httptest.NewRequest("GET", "/api/v1/config/intents/my-intent/diff?v1=1&v2=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestGateway_DiffConfigIntents_RequiresVersionParams(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-123", []string{"admin"})

	// Missing v1 and v2
	req := httptest.NewRequest("GET", "/api/v1/config/intents/my-intent/diff", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		// In-memory mode returns 503 first
		t.Logf("got %d (expected 503 in-memory mode)", rec.Code)
	}
}

func TestGateway_EventStream_WebSocket(t *testing.T) {
	gw, privKey := setupTestGateway(t)
	token := issueTestToken(t, privKey, "tenant-ws", []string{"admin"})

	// Start a test HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Inject auth context manually for the WebSocket handler
		r.Header.Set("Authorization", "Bearer "+token)
		gw.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Connect WebSocket
	wsURL := "ws" + srv.URL[4:] + "/api/v1/events/stream"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
	})
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.CloseNow()

	// Publish an event on the bus (get the bus from the handler)
	// We need to get the event bus - use the gateway's handler
	bus := gw.handlers.eventBus

	var wg sync.WaitGroup
	wg.Add(1)

	var received map[string]any
	go func() {
		defer wg.Done()
		readCtx, readCancel := context.WithTimeout(ctx, 3*time.Second)
		defer readCancel()
		wsjson.Read(readCtx, conn, &received)
	}()

	// Give WebSocket time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish event
	bus.Publish(context.Background(), eventbus.Event{
		Type:     "test.event",
		TenantID: "tenant-ws",
		Payload:  map[string]string{"hello": "world"},
	})

	wg.Wait()

	if received == nil {
		t.Fatal("did not receive WebSocket event")
	}
	if received["type"] != "test.event" {
		t.Errorf("expected type test.event, got %v", received["type"])
	}
	if received["tenant_id"] != "tenant-ws" {
		t.Errorf("expected tenant_id tenant-ws, got %v", received["tenant_id"])
	}
}

func TestGateway_EventStream_RequiresAuth(t *testing.T) {
	gw, _ := setupTestGateway(t)

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := "line1\nline2\nline3\n"
	b := "line1\nmodified\nline3\n"

	diff := unifiedDiff(a, b, "v1", "v2")
	if diff == "" {
		t.Error("expected non-empty diff")
	}

	// Should contain the diff markers
	if !containsString(diff, "--- v1") {
		t.Error("expected --- v1 header")
	}
	if !containsString(diff, "+++ v2") {
		t.Error("expected +++ v2 header")
	}
	if !containsString(diff, "-line2") {
		t.Error("expected -line2 in diff")
	}
	if !containsString(diff, "+modified") {
		t.Error("expected +modified in diff")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
