package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// EventStream handles WebSocket connections for real-time event streaming.
func (h *Handlers) EventStream(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.eventBus == nil {
		http.Error(w, `{"error":"event bus not available"}`, http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow cross-origin in dev
	})
	if err != nil {
		if h.logger != nil {
			h.logger.Error("websocket accept failed", "error", err)
		}
		return
	}
	defer conn.CloseNow()

	stream := h.eventBus.OpenStream(t.ID)
	defer h.eventBus.CloseStream(stream)

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			conn.Close(websocket.StatusNormalClosure, "connection closed")
			return
		case event, ok := <-stream.Ch():
			if !ok {
				conn.Close(websocket.StatusNormalClosure, "stream closed")
				return
			}

			msg := map[string]any{
				"type":      event.Type,
				"tenant_id": event.TenantID,
				"payload":   event.Payload,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}

			if err := wsjson.Write(ctx, conn, msg); err != nil {
				return
			}
		}
	}
}

// GetRollout returns a rollout by ID with status.
func (h *Handlers) GetRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract rollout ID from path: /api/v1/rollouts/{id}
	rolloutID := r.PathValue("id")
	if rolloutID == "" {
		http.Error(w, `{"error":"rollout id is required"}`, http.StatusBadRequest)
		return
	}

	rollout, err := h.repo.GetRollout(r.Context(), t.ID, rolloutID)
	if err != nil {
		http.Error(w, `{"error":"rollout not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollout)
}

// GetFleetAgents returns agents matched by a fleet's label selector.
func (h *Handlers) GetFleetAgents(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}

	// Extract fleet ID from path: /api/v1/fleets/{id}/agents
	fleetID := r.PathValue("id")
	if fleetID == "" {
		http.Error(w, `{"error":"fleet id is required"}`, http.StatusBadRequest)
		return
	}

	fleet, err := h.repo.GetFleet(r.Context(), t.ID, fleetID)
	if err != nil {
		http.Error(w, `{"error":"fleet not found"}`, http.StatusNotFound)
		return
	}

	var selector map[string]string
	json.Unmarshal([]byte(fleet.Selector), &selector)

	agents, err := h.repo.MatchAgentsBySelector(r.Context(), t.ID, selector)
	if err != nil {
		http.Error(w, `{"error":"failed to match agents"}`, http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []db.AgentRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// DiffConfigIntents returns a unified diff of compiled YAML between two versions of a config intent.
func (h *Handlers) DiffConfigIntents(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"config diff requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	// Extract intent name from path: /api/v1/config/intents/{name}/diff
	intentName := r.PathValue("name")
	if intentName == "" {
		http.Error(w, `{"error":"intent name is required"}`, http.StatusBadRequest)
		return
	}

	v1Str := r.URL.Query().Get("v1")
	v2Str := r.URL.Query().Get("v2")
	if v1Str == "" || v2Str == "" {
		http.Error(w, `{"error":"v1 and v2 query parameters are required"}`, http.StatusBadRequest)
		return
	}

	v1, err := strconv.Atoi(v1Str)
	if err != nil {
		http.Error(w, `{"error":"v1 must be an integer"}`, http.StatusBadRequest)
		return
	}
	v2, err := strconv.Atoi(v2Str)
	if err != nil {
		http.Error(w, `{"error":"v2 must be an integer"}`, http.StatusBadRequest)
		return
	}

	intent1, err := h.repo.GetConfigIntentByVersion(r.Context(), t.ID, intentName, v1)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"version %d not found"}`, v1), http.StatusNotFound)
		return
	}

	intent2, err := h.repo.GetConfigIntentByVersion(r.Context(), t.ID, intentName, v2)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"version %d not found"}`, v2), http.StatusNotFound)
		return
	}

	yaml1 := ""
	if intent1.CompiledYAML != nil {
		yaml1 = *intent1.CompiledYAML
	}
	yaml2 := ""
	if intent2.CompiledYAML != nil {
		yaml2 = *intent2.CompiledYAML
	}

	diff := unifiedDiff(yaml1, yaml2, fmt.Sprintf("v%d", v1), fmt.Sprintf("v%d", v2))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"name": intentName,
		"v1":   v1,
		"v2":   v2,
		"diff": diff,
	})
}

// unifiedDiff produces a simple unified diff between two strings.
func unifiedDiff(a, b, labelA, labelB string) string {
	linesA := strings.Split(a, "\n")
	linesB := strings.Split(b, "\n")

	var out strings.Builder
	out.WriteString(fmt.Sprintf("--- %s\n", labelA))
	out.WriteString(fmt.Sprintf("+++ %s\n", labelB))

	// Simple line-by-line diff using LCS
	table := make([][]int, len(linesA)+1)
	for i := range table {
		table[i] = make([]int, len(linesB)+1)
	}
	for i := 1; i <= len(linesA); i++ {
		for j := 1; j <= len(linesB); j++ {
			if linesA[i-1] == linesB[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}

	// Backtrack to produce diff
	type diffLine struct {
		op   byte // ' ', '-', '+'
		text string
	}
	var lines []diffLine
	i, j := len(linesA), len(linesB)
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && linesA[i-1] == linesB[j-1] {
			lines = append(lines, diffLine{' ', linesA[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || table[i][j-1] >= table[i-1][j]) {
			lines = append(lines, diffLine{'+', linesB[j-1]})
			j--
		} else {
			lines = append(lines, diffLine{'-', linesA[i-1]})
			i--
		}
	}

	// Reverse
	for left, right := 0, len(lines)-1; left < right; left, right = left+1, right-1 {
		lines[left], lines[right] = lines[right], lines[left]
	}

	for _, l := range lines {
		out.WriteByte(l.op)
		out.WriteString(l.text)
		out.WriteByte('\n')
	}

	return out.String()
}
