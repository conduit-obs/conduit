package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// CreateAPIKey creates a new API key and returns the plaintext key once.
func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"API keys require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Permissions) == 0 {
		req.Permissions = []string{"*"}
	}

	// Generate random key
	keyBytes := make([]byte, 32)
	rand.Read(keyBytes)
	plaintext := "cdkt_" + hex.EncodeToString(keyBytes)
	prefix := plaintext[:13]

	hash := sha256.Sum256([]byte(plaintext))
	keyHash := fmt.Sprintf("%x", hash)

	ak, err := h.repo.CreateAPIKey(r.Context(), t.ID, req.Name, keyHash, prefix, req.Permissions)
	if err != nil {
		http.Error(w, `{"error":"failed to create API key"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "api_key.created", map[string]any{
		"key_id": ak.ID,
		"name":   ak.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"id":          ak.ID,
		"name":        ak.Name,
		"key":         plaintext, // Only returned once
		"key_prefix":  ak.KeyPrefix,
		"permissions": ak.Permissions,
		"created_at":  ak.CreatedAt,
	})
}

// ListAPIKeys returns all API keys for the tenant (no secrets).
func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
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

	keys, err := h.repo.ListAPIKeys(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if keys == nil {
		keys = []db.APIKeyRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

// GetAgentConfigHistory returns the config history for an agent.
func (h *Handlers) GetAgentConfigHistory(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"config history requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	agentID := r.PathValue("id")
	if agentID == "" {
		http.Error(w, `{"error":"agent id is required"}`, http.StatusBadRequest)
		return
	}

	history, err := h.repo.ListAgentConfigHistory(r.Context(), t.ID, agentID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if history == nil {
		history = []db.AgentConfigHistoryRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// BatchOperation represents a single operation in a batch request.
type BatchOperation struct {
	Op   string         `json:"op"`   // register-agent, update-labels, create-intent
	Data map[string]any `json:"data"` // operation-specific data
}

// BatchRequest is the request body for POST /api/v1/batch.
type BatchRequest struct {
	Operations []BatchOperation `json:"operations"`
}

// BatchResult is the result of a single operation.
type BatchResult struct {
	Op     string `json:"op"`
	Status string `json:"status"` // success, error
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ExecuteBatch executes a batch of operations atomically.
func (h *Handlers) ExecuteBatch(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"batch operations require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if len(req.Operations) == 0 {
		http.Error(w, `{"error":"at least one operation is required"}`, http.StatusBadRequest)
		return
	}

	results := make([]BatchResult, len(req.Operations))

	for i, op := range req.Operations {
		result := BatchResult{Op: op.Op}

		switch op.Op {
		case "register-agent":
			name, _ := op.Data["name"].(string)
			labelsRaw, _ := op.Data["labels"].(map[string]any)
			labels := make(map[string]string)
			for k, v := range labelsRaw {
				if s, ok := v.(string); ok {
					labels[k] = s
				}
			}
			if name == "" {
				result.Status = "error"
				result.Error = "name is required"
			} else {
				agent, err := h.repo.CreateAgent(r.Context(), t.ID, name, labels)
				if err != nil {
					result.Status = "error"
					result.Error = err.Error()
				} else {
					result.Status = "success"
					result.Result = agent
				}
			}

		case "update-labels":
			agentID, _ := op.Data["agent_id"].(string)
			labelsRaw, _ := op.Data["labels"].(map[string]any)
			labels := make(map[string]string)
			for k, v := range labelsRaw {
				if s, ok := v.(string); ok {
					labels[k] = s
				}
			}
			if agentID == "" {
				result.Status = "error"
				result.Error = "agent_id is required"
			} else {
				agent, err := h.repo.UpdateAgentLabels(r.Context(), t.ID, agentID, labels)
				if err != nil {
					result.Status = "error"
					result.Error = err.Error()
				} else {
					result.Status = "success"
					result.Result = agent
				}
			}

		case "create-intent":
			name, _ := op.Data["name"].(string)
			intentRaw, _ := json.Marshal(op.Data["intent"])
			var intent config.Intent
			json.Unmarshal(intentRaw, &intent)

			if name == "" {
				result.Status = "error"
				result.Error = "name is required"
			} else {
				yamlOut, err := h.compiler.Compile(&intent)
				if err != nil {
					result.Status = "error"
					result.Error = err.Error()
				} else {
					intentJSON, _ := json.Marshal(intent)
					ci, err := h.repo.CreateConfigIntent(r.Context(), t.ID, name, string(intentJSON), &yamlOut)
					if err != nil {
						result.Status = "error"
						result.Error = err.Error()
					} else {
						result.Status = "success"
						result.Result = ci
					}
				}
			}

		default:
			result.Status = "error"
			result.Error = fmt.Sprintf("unknown operation: %s", op.Op)
		}

		results[i] = result
	}

	// Check if any failed
	allSuccess := true
	for _, r := range results {
		if r.Status == "error" {
			allSuccess = false
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if !allSuccess {
		w.WriteHeader(http.StatusMultiStatus)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"results": results,
	})
}

// CreateRolloutWithStrategy creates a rollout with canary or all-at-once strategy.
func (h *Handlers) CreateRolloutWithStrategy(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	var req struct {
		FleetID      string `json:"fleet_id"`
		IntentID     string `json:"intent_id"`
		StrategyType string `json:"strategy"`       // "canary" or "all-at-once"
		CanaryPct    int    `json:"canary_percent"`  // 0-100
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.FleetID == "" || req.IntentID == "" {
		http.Error(w, `{"error":"fleet_id and intent_id are required"}`, http.StatusBadRequest)
		return
	}

	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	if req.StrategyType == "" {
		req.StrategyType = "all-at-once"
	}
	if req.StrategyType == "canary" && (req.CanaryPct <= 0 || req.CanaryPct > 100) {
		http.Error(w, `{"error":"canary_percent must be between 1 and 100"}`, http.StatusBadRequest)
		return
	}

	fleet, err := h.repo.GetFleet(r.Context(), t.ID, req.FleetID)
	if err != nil {
		http.Error(w, `{"error":"fleet not found"}`, http.StatusNotFound)
		return
	}

	intent, err := h.repo.GetConfigIntentByID(r.Context(), t.ID, req.IntentID)
	if err != nil {
		http.Error(w, `{"error":"intent not found"}`, http.StatusNotFound)
		return
	}

	if !intent.Promoted {
		http.Error(w, `{"error":"only promoted intents can be used in rollouts"}`, http.StatusBadRequest)
		return
	}

	// Resolve fleet variables in intent if needed
	compiledYAML := intent.CompiledYAML
	if compiledYAML != nil && fleet.Variables != "{}" && fleet.Variables != "" {
		var vars map[string]string
		json.Unmarshal([]byte(fleet.Variables), &vars)
		if len(vars) > 0 {
			resolved, err := resolveTemplateVars(*compiledYAML, vars)
			if err == nil {
				compiledYAML = &resolved
			}
		}
	}

	var selector map[string]string
	json.Unmarshal([]byte(fleet.Selector), &selector)

	agents, err := h.repo.MatchAgentsBySelector(r.Context(), t.ID, selector)
	if err != nil {
		http.Error(w, `{"error":"failed to match agents"}`, http.StatusInternalServerError)
		return
	}

	strategyJSON, _ := json.Marshal(map[string]any{
		"type":           req.StrategyType,
		"canary_percent": req.CanaryPct,
	})

	rollout, err := h.repo.CreateRollout(r.Context(), t.ID, req.FleetID, req.IntentID, len(agents), string(strategyJSON))
	if err != nil {
		http.Error(w, `{"error":"failed to create rollout"}`, http.StatusInternalServerError)
		return
	}

	h.repo.CreateRolloutHistory(r.Context(), t.ID, rollout.ID, "in_progress", "Rollout created")

	cfgHash := ""
	if compiledYAML != nil {
		cfgHash = configHash(*compiledYAML)
	}

	// Determine canary vs remainder agents
	canaryCount := len(agents)
	if req.StrategyType == "canary" {
		canaryCount = (len(agents) * req.CanaryPct) / 100
		if canaryCount < 1 && len(agents) > 0 {
			canaryCount = 1
		}
	}

	// Create per-agent entries with phase
	for i, a := range agents {
		phase := "all"
		if req.StrategyType == "canary" {
			if i < canaryCount {
				phase = "canary"
			} else {
				phase = "remainder"
			}
		}
		h.repo.CreateRolloutAgentWithPhase(r.Context(), t.ID, rollout.ID, a.ID, cfgHash, phase)
	}

	// Push config to canary agents only (or all for all-at-once)
	pushedCount := 0
	if h.opampServer != nil && compiledYAML != nil {
		pushAgents := agents
		if req.StrategyType == "canary" {
			pushAgents = agents[:canaryCount]
		}
		var agentNames []string
		for _, a := range pushAgents {
			agentNames = append(agentNames, a.Name)
		}
		pushedCount = h.opampServer.PushConfigToTenantAgents(t.ID, *compiledYAML, agentNames)

		// Record config history for pushed agents
		for _, a := range pushAgents {
			h.repo.CreateAgentConfigHistory(r.Context(), t.ID, a.ID, *compiledYAML, cfgHash, "rollout")
		}
	}

	status := "in_progress"
	if req.StrategyType != "canary" && (pushedCount >= len(agents) || len(agents) == 0) {
		status = "completed"
	}

	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rollout.ID, status, pushedCount)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rollout.ID, status,
		fmt.Sprintf("Pushed to %d/%d agents (strategy: %s)", pushedCount, len(agents), req.StrategyType))
	rollout.Status = status
	rollout.CompletedCount = pushedCount

	h.publishAudit(r.Context(), t.ID, "rollout.created", map[string]any{
		"rollout_id":   rollout.ID,
		"fleet_id":     req.FleetID,
		"intent_id":    req.IntentID,
		"strategy":     req.StrategyType,
		"target_count": len(agents),
		"pushed_count": pushedCount,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rollout)
}

// resolveTemplateVars resolves {{.var}} placeholders in YAML using Go templates.
func resolveTemplateVars(yamlStr string, vars map[string]string) (string, error) {
	tmpl, err := template.New("config").Option("missingkey=error").Parse(yamlStr)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// ListAgentsWithCapability lists agents filtered by capability query param.
func (h *Handlers) ListAgentsWithCapability(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	capability := r.URL.Query().Get("capability")

	if h.repo != nil && capability != "" {
		agents, err := h.repo.ListAgentsByCapability(r.Context(), t.ID, capability)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		if agents == nil {
			agents = []db.AgentRow{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)
		return
	}

	// Delegate to standard ListAgents for non-capability queries
	h.ListAgents(w, r)
}

// suppress unused import warning
var _ = strings.Join
