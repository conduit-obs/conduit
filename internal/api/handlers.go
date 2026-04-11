package api

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/conduit-obs/conduit/internal/config"
	"github.com/conduit-obs/conduit/internal/db"
	"github.com/conduit-obs/conduit/internal/eventbus"
	"github.com/conduit-obs/conduit/internal/opamp"
	"github.com/conduit-obs/conduit/internal/ratelimit"
	"github.com/conduit-obs/conduit/internal/tenant"
)

// Handlers holds the API request handlers.
type Handlers struct {
	compiler         *config.Compiler
	cachingCompiler  *config.CachingCompiler
	tracker          *opamp.HeartbeatTracker
	repo             *db.Repo          // nil = in-memory mode (for tests)
	opampServer      *opamp.Server     // nil if OpAMP not configured
	eventBus         *eventbus.Bus
	logger           *slog.Logger
	rateLimiter      *ratelimit.TokenBucket
	tenantRateLimits map[string]int // cached rate limits
	jwtPrivateKey    *rsa.PrivateKey   // for issuing tokens (nil in test mode)
	jwtIssuer        string
	jwtAudience      string
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(compiler *config.Compiler, tracker *opamp.HeartbeatTracker, repo *db.Repo, opampServer *opamp.Server, bus *eventbus.Bus, logger *slog.Logger) *Handlers {
	cc := config.NewCachingCompiler()
	return &Handlers{
		compiler:         compiler,
		cachingCompiler:  cc,
		tracker:          tracker,
		repo:             repo,
		opampServer:      opampServer,
		eventBus:         bus,
		logger:           logger,
		rateLimiter:      ratelimit.New(),
		tenantRateLimits: make(map[string]int),
		jwtIssuer:        "conduit",
		jwtAudience:      "conduit-api",
	}
}

// SetJWTKey sets the private key for token issuance.
func (h *Handlers) SetJWTKey(key *rsa.PrivateKey, issuer, audience string) {
	h.jwtPrivateKey = key
	h.jwtIssuer = issuer
	h.jwtAudience = audience
}

// GetRateLimiter returns the rate limiter instance.
func (h *Handlers) GetRateLimiter() *ratelimit.TokenBucket {
	return h.rateLimiter
}

// GetTenantRateLimit returns the cached rate limit for a tenant, fetching from DB if needed.
func (h *Handlers) GetTenantRateLimit(tenantID string) int {
	if rl, ok := h.tenantRateLimits[tenantID]; ok {
		return rl
	}
	if h.repo != nil {
		rl, err := h.repo.GetTenantRateLimit(context.Background(), tenantID)
		if err == nil {
			h.tenantRateLimits[tenantID] = rl
			return rl
		}
	}
	return 0
}

// publishAudit publishes an audit event to the event bus and persists it if a repo is available.
// Includes X-Request-ID correlation if present in context.
func (h *Handlers) publishAudit(ctx context.Context, tenantID, eventType string, payload any) {
	requestID := RequestIDFromContext(ctx)

	if h.eventBus != nil {
		h.eventBus.Publish(ctx, eventbus.Event{
			Type:     eventType,
			TenantID: tenantID,
			Payload:  payload,
		})
	}
	if h.repo != nil {
		if requestID != "" {
			if err := h.repo.CreateEventWithRequestID(ctx, tenantID, eventType, payload, requestID); err != nil && h.logger != nil {
				h.logger.Error("failed to persist audit event", "type", eventType, "request_id", requestID, "error", err)
			}
		} else {
			if err := h.repo.CreateEvent(ctx, tenantID, eventType, payload); err != nil && h.logger != nil {
				h.logger.Error("failed to persist audit event", "type", eventType, "error", err)
			}
		}
	}
}

// HealthCheck returns service health.
func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ListAgents returns agents for the current tenant.
func (h *Handlers) ListAgents(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.repo != nil {
		agents, err := h.repo.ListAgents(r.Context(), t.ID)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		if agents == nil {
			agents = []db.AgentRow{}
		}
		json.NewEncoder(w).Encode(agents)
		return
	}

	// Fallback: in-memory tracker
	agents := h.tracker.GetAgentsByTenant(t.ID)
	type agentResponse struct {
		InstanceID    string `json:"instance_id"`
		Status        string `json:"status"`
		LastHeartbeat string `json:"last_heartbeat,omitempty"`
	}
	var resp []agentResponse
	for _, a := range agents {
		resp = append(resp, agentResponse{
			InstanceID:    a.InstanceID,
			Status:        a.Status,
			LastHeartbeat: a.LastHeartbeat.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	if resp == nil {
		resp = []agentResponse{}
	}
	json.NewEncoder(w).Encode(resp)
}

// CompileIntent compiles an intent document to OTel YAML.
func (h *Handlers) CompileIntent(w http.ResponseWriter, r *http.Request) {
	var intent config.Intent
	if err := json.NewDecoder(r.Body).Decode(&intent); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	yamlOut, err := h.compiler.Compile(&intent)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"yaml": yamlOut})
}

// CreateConfigIntent creates a new config intent, compiles it, and persists both.
// Supports optional "tags" field in the request body.
func (h *Handlers) CreateConfigIntent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	var req struct {
		Name   string        `json:"name"`
		Intent config.Intent `json:"intent"`
		Tags   []string      `json:"tags,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	yamlOut, err := h.compiler.Compile(&req.Intent)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	intentJSON, _ := json.Marshal(req.Intent)

	if h.repo == nil {
		h.publishAudit(r.Context(), t.ID, "config_intent.created", map[string]any{
			"name": req.Name,
			"tags": req.Tags,
		})

		// In-memory mode: just return compiled result
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"name":          req.Name,
			"intent_json":   string(intentJSON),
			"compiled_yaml": yamlOut,
			"tags":          req.Tags,
		})
		return
	}

	ci, err := h.repo.CreateConfigIntentWithTags(r.Context(), t.ID, req.Name, string(intentJSON), &yamlOut, req.Tags)
	if err != nil {
		http.Error(w, `{"error":"failed to persist intent"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "config_intent.created", map[string]any{
		"intent_id": ci.ID,
		"name":      ci.Name,
		"version":   ci.Version,
		"tags":      ci.Tags,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(ci)
}

// RegisterAgent creates a new agent in the database.
func (h *Handlers) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	var req struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}

	if req.Labels == nil {
		req.Labels = map[string]string{}
	}

	if h.repo == nil {
		h.publishAudit(r.Context(), t.ID, "agent.registered", map[string]any{
			"name": req.Name,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"name":      req.Name,
			"tenant_id": t.ID,
			"labels":    req.Labels,
			"status":    "registered",
			"cert_hint": "Use mTLS with CN=" + req.Name + ", O=" + t.ID + " to connect via OpAMP",
		})
		return
	}

	agent, err := h.repo.CreateAgent(r.Context(), t.ID, req.Name, req.Labels)
	if err != nil {
		http.Error(w, `{"error":"failed to register agent"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "agent.registered", map[string]any{
		"agent_id": agent.ID,
		"name":     agent.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"agent":     agent,
		"cert_hint": "Use mTLS with CN=" + req.Name + ", O=" + t.ID + " to connect via OpAMP",
	})
}

// CreateFleet creates a new fleet with a label selector.
func (h *Handlers) CreateFleet(w http.ResponseWriter, r *http.Request) {
	t, ok := tenant.FromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"no tenant context"}`, http.StatusInternalServerError)
		return
	}

	var req struct {
		Name     string            `json:"name"`
		Selector map[string]string `json:"selector"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if req.Selector == nil {
		req.Selector = map[string]string{}
	}

	if h.repo == nil {
		h.publishAudit(r.Context(), t.ID, "fleet.created", map[string]any{
			"name":     req.Name,
			"selector": req.Selector,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"name":      req.Name,
			"tenant_id": t.ID,
			"selector":  req.Selector,
		})
		return
	}

	fleet, err := h.repo.CreateFleet(r.Context(), t.ID, req.Name, req.Selector)
	if err != nil {
		http.Error(w, `{"error":"failed to create fleet"}`, http.StatusInternalServerError)
		return
	}

	h.publishAudit(r.Context(), t.ID, "fleet.created", map[string]any{
		"fleet_id": fleet.ID,
		"name":     fleet.Name,
		"selector": req.Selector,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(fleet)
}

// ListFleets returns all fleets for the tenant.
func (h *Handlers) ListFleets(w http.ResponseWriter, r *http.Request) {
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

	fleets, err := h.repo.ListFleets(r.Context(), t.ID)
	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if fleets == nil {
		fleets = []db.FleetRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fleets)
}

// CreateRollout creates a rollout that pushes a config intent to a fleet's matched agents.
// Supports optional strategy and canary_percent fields.
func (h *Handlers) CreateRollout(w http.ResponseWriter, r *http.Request) {
	h.CreateRolloutWithStrategy(w, r)
}

// ListConfigIntents lists config intents for the tenant, optionally filtered by tag.
func (h *Handlers) ListConfigIntents(w http.ResponseWriter, r *http.Request) {
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

	tag := r.URL.Query().Get("tag")

	var intents []db.ConfigIntentRow
	var err error

	if tag != "" {
		intents, err = h.repo.ListConfigIntentsByTag(r.Context(), t.ID, tag)
	} else {
		intents, err = h.repo.ListConfigIntents(r.Context(), t.ID)
	}

	if err != nil {
		http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
		return
	}
	if intents == nil {
		intents = []db.ConfigIntentRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intents)
}
