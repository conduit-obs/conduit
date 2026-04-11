package reconciler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// AgentConfig represents an agent's configuration state for reconciliation.
type AgentConfig struct {
	AgentID        string
	AgentName      string
	TenantID       string
	EffectiveHash  string
	ReportedHash   string
	RetryCount     int
	LastRetry      time.Time
}

// ConfigPusher pushes config to agents.
type ConfigPusher func(tenantID, agentName, configYAML string) error

// Reconciler periodically checks agent config state and re-pushes drifted configs.
type Reconciler struct {
	mu       sync.Mutex
	drifted  map[string]*AgentConfig // agentID -> config
	logger   *slog.Logger
	maxRetry int
}

// New creates a new Reconciler.
func New(logger *slog.Logger) *Reconciler {
	return &Reconciler{
		drifted:  make(map[string]*AgentConfig),
		logger:   logger,
		maxRetry: 3,
	}
}

// RecordDrift marks an agent as drifted.
func (r *Reconciler) RecordDrift(ac *AgentConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drifted[ac.AgentID] = ac
}

// ClearDrift removes an agent from the drifted set.
func (r *Reconciler) ClearDrift(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.drifted, agentID)
}

// DriftedAgents returns the current list of drifted agents.
func (r *Reconciler) DriftedAgents() []*AgentConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []*AgentConfig
	for _, ac := range r.drifted {
		cp := *ac
		result = append(result, &cp)
	}
	return result
}

// Reconcile attempts to remediate all drifted agents using exponential backoff.
func (r *Reconciler) Reconcile(ctx context.Context, pusher ConfigPusher, getConfig func(tenantID, agentID string) (string, error)) {
	r.mu.Lock()
	toProcess := make([]*AgentConfig, 0, len(r.drifted))
	for _, ac := range r.drifted {
		toProcess = append(toProcess, ac)
	}
	r.mu.Unlock()

	for _, ac := range toProcess {
		if ac.RetryCount >= r.maxRetry {
			if r.logger != nil {
				r.logger.Warn("max retries exceeded for drifted agent", "agent_id", ac.AgentID, "retries", ac.RetryCount)
			}
			continue
		}

		// Exponential backoff
		backoff := time.Duration(1<<uint(ac.RetryCount)) * time.Second
		if time.Since(ac.LastRetry) < backoff {
			continue
		}

		configYAML, err := getConfig(ac.TenantID, ac.AgentID)
		if err != nil {
			continue
		}

		if err := pusher(ac.TenantID, ac.AgentName, configYAML); err != nil {
			r.mu.Lock()
			ac.RetryCount++
			ac.LastRetry = time.Now()
			r.mu.Unlock()
			if r.logger != nil {
				r.logger.Error("reconciliation push failed", "agent_id", ac.AgentID, "retry", ac.RetryCount, "error", err)
			}
		} else {
			r.ClearDrift(ac.AgentID)
			if r.logger != nil {
				r.logger.Info("reconciliation succeeded", "agent_id", ac.AgentID)
			}
		}
	}
}
