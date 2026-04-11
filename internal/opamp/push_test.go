package opamp

import (
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
)

func TestServer_GetConnectedAgentIDsByTenant(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	srv := NewServer(ServerConfig{}, tracker, bus, nil)
	srv.connections["agent-1"] = &agentConnection{instanceID: "agent-1", tenantID: "t1"}
	srv.connections["agent-2"] = &agentConnection{instanceID: "agent-2", tenantID: "t1"}
	srv.connections["agent-3"] = &agentConnection{instanceID: "agent-3", tenantID: "t2"}

	ids := srv.GetConnectedAgentIDsByTenant("t1")
	if len(ids) != 2 {
		t.Errorf("expected 2 agents for t1, got %d", len(ids))
	}

	ids = srv.GetConnectedAgentIDsByTenant("t2")
	if len(ids) != 1 {
		t.Errorf("expected 1 agent for t2, got %d", len(ids))
	}

	ids = srv.GetConnectedAgentIDsByTenant("t3")
	if len(ids) != 0 {
		t.Errorf("expected 0 agents for t3, got %d", len(ids))
	}
}

func TestServer_PushConfig_NotConnected(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	srv := NewServer(ServerConfig{}, tracker, bus, nil)

	ok := srv.PushConfig("nonexistent", "some config")
	if ok {
		t.Error("expected PushConfig to return false for non-connected agent")
	}
}
