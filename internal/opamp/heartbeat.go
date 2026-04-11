package opamp

import (
	"context"
	"sync"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
)

// AgentStatus represents the current status of a connected agent.
type AgentStatus struct {
	InstanceID      string
	TenantID        string
	LastHeartbeat   time.Time
	Status          string // "connected", "disconnected", "unhealthy"
	EffectiveConfig string
	ReportedConfig  string
	Capabilities    map[string]any
	Topology        map[string]string // region, zone, cluster
}

// HeartbeatTracker tracks agent heartbeats and detects stale agents.
type HeartbeatTracker struct {
	mu       sync.RWMutex
	agents   map[string]*AgentStatus
	timeout  time.Duration
	eventBus *eventbus.Bus
}

// NewHeartbeatTracker creates a heartbeat tracker with the given timeout.
func NewHeartbeatTracker(timeout time.Duration, bus *eventbus.Bus) *HeartbeatTracker {
	return &HeartbeatTracker{
		agents:   make(map[string]*AgentStatus),
		timeout:  timeout,
		eventBus: bus,
	}
}

// RecordHeartbeat records a heartbeat from an agent.
func (h *HeartbeatTracker) RecordHeartbeat(instanceID, tenantID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	agent, exists := h.agents[instanceID]
	if !exists {
		agent = &AgentStatus{
			InstanceID: instanceID,
			TenantID:   tenantID,
			Status:     "connected",
		}
		h.agents[instanceID] = agent

		if h.eventBus != nil {
			h.eventBus.Publish(context.Background(), eventbus.Event{
				Type:     "agent.connected",
				TenantID: tenantID,
				Payload:  map[string]string{"instance_id": instanceID},
			})
		}
	}

	agent.LastHeartbeat = time.Now()
	agent.Status = "connected"
}

// SetEffectiveConfig records the effective config for an agent.
func (h *HeartbeatTracker) SetEffectiveConfig(instanceID, config string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if agent, ok := h.agents[instanceID]; ok {
		agent.EffectiveConfig = config
	}
}

// SetCapabilities records the capabilities reported by the agent.
func (h *HeartbeatTracker) SetCapabilities(instanceID string, capabilities map[string]any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if agent, ok := h.agents[instanceID]; ok {
		agent.Capabilities = capabilities
	}
}

// SetReportedConfig records the config reported by the agent.
func (h *HeartbeatTracker) SetReportedConfig(instanceID, config string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if agent, ok := h.agents[instanceID]; ok {
		agent.ReportedConfig = config
	}
}

// CheckDrift compares reported vs effective config for each agent and emits drift events.
func (h *HeartbeatTracker) CheckDrift() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, agent := range h.agents {
		if agent.Status != "connected" {
			continue
		}
		if agent.EffectiveConfig != "" && agent.ReportedConfig != "" &&
			agent.EffectiveConfig != agent.ReportedConfig {
			if h.eventBus != nil {
				h.eventBus.Publish(context.Background(), eventbus.Event{
					Type:     "agent.config_drift",
					TenantID: agent.TenantID,
					Payload: map[string]string{
						"instance_id": agent.InstanceID,
					},
				})
			}
		}
	}
}

// GetAgent returns the status of a specific agent.
func (h *HeartbeatTracker) GetAgent(instanceID string) (*AgentStatus, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	agent, ok := h.agents[instanceID]
	if !ok {
		return nil, false
	}
	cp := *agent
	return &cp, true
}

// GetAgentsByTenant returns all agents for a given tenant.
func (h *HeartbeatTracker) GetAgentsByTenant(tenantID string) []*AgentStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var result []*AgentStatus
	for _, agent := range h.agents {
		if agent.TenantID == tenantID {
			cp := *agent
			result = append(result, &cp)
		}
	}
	return result
}

// CheckStale marks agents as unhealthy/disconnected if they haven't sent
// a heartbeat within the timeout period.
func (h *HeartbeatTracker) CheckStale() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for _, agent := range h.agents {
		if agent.Status == "connected" && now.Sub(agent.LastHeartbeat) > h.timeout {
			agent.Status = "unhealthy"
			if h.eventBus != nil {
				h.eventBus.Publish(context.Background(), eventbus.Event{
					Type:     "agent.unhealthy",
					TenantID: agent.TenantID,
					Payload:  map[string]string{"instance_id": agent.InstanceID},
				})
			}
		}
	}
}

// SyncToDB persists current heartbeat state to the database.
// It calls the provided function for each agent with its tenant, instance ID, status, and last heartbeat.
func (h *HeartbeatTracker) SyncToDB(syncFn func(tenantID, instanceID, status string, lastHeartbeat time.Time)) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, agent := range h.agents {
		syncFn(agent.TenantID, agent.InstanceID, agent.Status, agent.LastHeartbeat)
	}
}

// SetTopology records the topology metadata reported by the agent.
func (h *HeartbeatTracker) SetTopology(instanceID string, topology map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if agent, ok := h.agents[instanceID]; ok {
		agent.Topology = topology
	}
}

// RemoveAgent removes an agent from tracking.
func (h *HeartbeatTracker) RemoveAgent(instanceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if agent, ok := h.agents[instanceID]; ok {
		if h.eventBus != nil {
			h.eventBus.Publish(context.Background(), eventbus.Event{
				Type:     "agent.disconnected",
				TenantID: agent.TenantID,
				Payload:  map[string]string{"instance_id": instanceID},
			})
		}
		delete(h.agents, instanceID)
	}
}
