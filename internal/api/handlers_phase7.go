package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// Metrics returns compilation cache stats and system health.
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	metrics := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if h.cachingCompiler != nil {
		hits, misses := h.cachingCompiler.Stats()
		metrics["config_cache"] = map[string]any{
			"hits":   hits,
			"misses": misses,
			"ratio":  cacheRatio(hits, misses),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func cacheRatio(hits, misses int64) float64 {
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// PauseRollout pauses an in-progress rollout.
func (h *Handlers) PauseRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	rolloutID := r.PathValue("id")
	rollout, err := h.repo.GetRollout(r.Context(), t.ID, rolloutID)
	if err != nil {
		http.Error(w, `{"error":"rollout not found"}`, http.StatusNotFound)
		return
	}

	if rollout.Status != "in_progress" {
		http.Error(w, `{"error":"can only pause in_progress rollouts"}`, http.StatusBadRequest)
		return
	}

	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "paused", rollout.CompletedCount)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "paused", "Rollout paused by user")
	h.publishAudit(r.Context(), t.ID, "rollout.paused", map[string]any{"rollout_id": rolloutID})

	rollout.Status = "paused"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollout)
}

// ResumeRollout resumes a paused rollout.
func (h *Handlers) ResumeRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	rolloutID := r.PathValue("id")
	rollout, err := h.repo.GetRollout(r.Context(), t.ID, rolloutID)
	if err != nil {
		http.Error(w, `{"error":"rollout not found"}`, http.StatusNotFound)
		return
	}

	if rollout.Status != "paused" {
		http.Error(w, `{"error":"can only resume paused rollouts"}`, http.StatusBadRequest)
		return
	}

	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "in_progress", rollout.CompletedCount)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "in_progress", "Rollout resumed by user")
	h.publishAudit(r.Context(), t.ID, "rollout.resumed", map[string]any{"rollout_id": rolloutID})

	rollout.Status = "in_progress"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollout)
}

// CancelRollout cancels a rollout.
func (h *Handlers) CancelRollout(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"rollouts require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	rolloutID := r.PathValue("id")
	rollout, err := h.repo.GetRollout(r.Context(), t.ID, rolloutID)
	if err != nil {
		http.Error(w, `{"error":"rollout not found"}`, http.StatusNotFound)
		return
	}

	if rollout.Status == "completed" || rollout.Status == "failed" {
		http.Error(w, `{"error":"cannot cancel a completed or failed rollout"}`, http.StatusBadRequest)
		return
	}

	h.repo.UpdateRolloutStatus(r.Context(), t.ID, rolloutID, "failed", rollout.CompletedCount)
	h.repo.CreateRolloutHistory(r.Context(), t.ID, rolloutID, "failed", "Rollout cancelled by user")
	h.publishAudit(r.Context(), t.ID, "rollout.cancelled", map[string]any{"rollout_id": rolloutID})

	rollout.Status = "failed"
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollout)
}

// CreateWebhook registers a new webhook.
func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"webhooks require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" {
		http.Error(w, `{"error":"name and url are required"}`, http.StatusBadRequest)
		return
	}
	if req.Events == nil {
		req.Events = []string{}
	}

	wh, err := h.repo.CreateWebhook(r.Context(), t.ID, req.Name, req.URL, req.Events)
	if err != nil {
		http.Error(w, `{"error":"failed to create webhook"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "webhook.created", map[string]any{"webhook_id": wh.ID, "name": wh.Name})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

// ListWebhooks returns all webhooks for the tenant.
func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
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

	webhooks, err := h.repo.ListWebhooks(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if webhooks == nil {
		webhooks = []db.WebhookRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(webhooks)
}

// DeleteWebhook removes a webhook.
func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"webhooks require database mode"}`, http.StatusServiceUnavailable)
		return
	}

	webhookID := r.PathValue("id")
	if err := h.repo.DeleteWebhook(r.Context(), t.ID, webhookID); err != nil {
		http.Error(w, `{"error":"failed to delete webhook"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "webhook.deleted", map[string]any{"webhook_id": webhookID})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// DeregisterAgent soft-deletes an agent.
func (h *Handlers) DeregisterAgent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}
	if h.repo == nil {
		http.Error(w, `{"error":"agent deregistration requires database mode"}`, http.StatusServiceUnavailable)
		return
	}

	agentID := r.PathValue("id")
	if err := h.repo.SoftDeleteAgent(r.Context(), t.ID, agentID); err != nil {
		http.Error(w, `{"error":"failed to deregister agent"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "agent.deregistered", map[string]any{"agent_id": agentID})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deregistered"})
}

// CompileIntentWithInheritance compiles an intent resolving base intent inheritance.
func (h *Handlers) CompileIntentWithInheritance(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		// For compile endpoint without tenant context, just compile directly
		h.CompileIntent(w, r)
		return
	}

	var intent config.Intent
	if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Resolve inheritance
	if intent.BaseIntent != "" && h.repo != nil {
		baseCI, err := h.repo.GetConfigIntent(r.Context(), t.ID, intent.BaseIntent)
		if err == nil {
			var baseIntent config.Intent
			json.Unmarshal([]byte(baseCI.IntentJSON), &baseIntent)
			intent.MergeBase(&baseIntent)
		}
	}

	compiler := h.compiler
	if h.cachingCompiler != nil {
		yamlOut, err := h.cachingCompiler.Compile(&intent)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"yaml": yamlOut})
		return
	}

	yamlOut, err := compiler.Compile(&intent)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"yaml": yamlOut})
}

// ListAgentsWithHealthFilter lists agents with optional min_health filter.
func (h *Handlers) ListAgentsWithHealthFilter(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	minHealthStr := r.URL.Query().Get("min_health")
	capability := r.URL.Query().Get("capability")

	// Handle min_health filter
	if minHealthStr != "" && h.repo != nil {
		minHealth, err := strconv.Atoi(minHealthStr)
		if err != nil {
			http.Error(w, `{"error":"min_health must be an integer"}`, http.StatusBadRequest)
			return
		}
		agents, err := h.repo.ListAgentsByMinHealth(r.Context(), t.ID, minHealth)
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

	// Handle capability filter
	if capability != "" && h.repo != nil {
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

	// Default: list all agents
	h.ListAgents(w, r)
}

// WebhookDeliverer delivers events to registered webhooks.
type WebhookDeliverer struct {
	repo   *db.Repo
	logger *slog.Logger
}

// NewWebhookDeliverer creates a new webhook deliverer.
func NewWebhookDeliverer(repo *db.Repo, logger *slog.Logger) *WebhookDeliverer {
	return &WebhookDeliverer{repo: repo, logger: logger}
}

// Deliver sends an event to all matching webhooks for the tenant.
func (wd *WebhookDeliverer) Deliver(tenantID, eventType string, payload any) {
	if wd.repo == nil {
		return
	}

	webhooks, err := wd.repo.GetActiveWebhooksForEvent(context.Background(), tenantID, eventType)
	if err != nil || len(webhooks) == 0 {
		return
	}

	body, _ := json.Marshal(map[string]any{
		"type":      eventType,
		"tenant_id": tenantID,
		"payload":   payload,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	for _, wh := range webhooks {
		go wd.deliverWithRetry(wh.URL, body, 3)
	}
}

func (wd *WebhookDeliverer) deliverWithRetry(url string, body []byte, maxRetries int) {
	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		if err == nil && resp.StatusCode < 500 {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		if wd.logger != nil {
			wd.logger.Warn("webhook delivery failed", "url", url, "attempt", attempt+1, "error", err)
		}
		time.Sleep(time.Duration(attempt+1) * time.Second)
	}
}

// suppress unused import
var _ = fmt.Sprint
var _ = strconv.Itoa
