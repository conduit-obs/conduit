package opamp

import (
	"context"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
)

func TestHeartbeatTracker_RecordAndGet(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(5*time.Second, bus)

	tracker.RecordHeartbeat("agent-1", "tenant-a")
	tracker.RecordHeartbeat("agent-2", "tenant-a")
	tracker.RecordHeartbeat("agent-3", "tenant-b")

	agent, ok := tracker.GetAgent("agent-1")
	if !ok {
		t.Fatal("agent-1 not found")
	}
	if agent.Status != "connected" {
		t.Errorf("expected connected, got %s", agent.Status)
	}
	if agent.TenantID != "tenant-a" {
		t.Errorf("expected tenant-a, got %s", agent.TenantID)
	}

	tenantA := tracker.GetAgentsByTenant("tenant-a")
	if len(tenantA) != 2 {
		t.Errorf("expected 2 agents for tenant-a, got %d", len(tenantA))
	}

	tenantB := tracker.GetAgentsByTenant("tenant-b")
	if len(tenantB) != 1 {
		t.Errorf("expected 1 agent for tenant-b, got %d", len(tenantB))
	}
}

func TestHeartbeatTracker_StaleDetection(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(1*time.Millisecond, bus)

	tracker.RecordHeartbeat("agent-1", "tenant-a")
	time.Sleep(5 * time.Millisecond)
	tracker.CheckStale()

	agent, ok := tracker.GetAgent("agent-1")
	if !ok {
		t.Fatal("agent not found")
	}
	if agent.Status != "unhealthy" {
		t.Errorf("expected unhealthy, got %s", agent.Status)
	}
}

func TestHeartbeatTracker_EffectiveConfig(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	tracker.RecordHeartbeat("agent-1", "tenant-a")
	tracker.SetEffectiveConfig("agent-1", "receivers:\n  otlp:\n")

	agent, ok := tracker.GetAgent("agent-1")
	if !ok {
		t.Fatal("agent not found")
	}
	if agent.EffectiveConfig != "receivers:\n  otlp:\n" {
		t.Error("effective config not set")
	}
}

func TestHeartbeatTracker_Remove(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	tracker.RecordHeartbeat("agent-1", "tenant-a")
	tracker.RemoveAgent("agent-1")

	_, ok := tracker.GetAgent("agent-1")
	if ok {
		t.Error("agent should have been removed")
	}
}

func TestHeartbeatTracker_Events(t *testing.T) {
	bus := eventbus.New()
	var connected, disconnected bool

	bus.Subscribe("agent.connected", func(_ context.Context, e eventbus.Event) {
		connected = true
	})
	bus.Subscribe("agent.disconnected", func(_ context.Context, e eventbus.Event) {
		disconnected = true
	})

	tracker := NewHeartbeatTracker(30*time.Second, bus)
	tracker.RecordHeartbeat("agent-1", "tenant-a")
	if !connected {
		t.Error("expected agent.connected event")
	}

	tracker.RemoveAgent("agent-1")
	if !disconnected {
		t.Error("expected agent.disconnected event")
	}
}
