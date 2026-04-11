package opamp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/conduit-obs/conduit/internal/eventbus"
)

func TestHeartbeatTracker_ConfigDrift(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	var mu sync.Mutex
	var events []eventbus.Event
	bus.Subscribe("agent.config_drift", func(ctx context.Context, e eventbus.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	// Record agent
	tracker.RecordHeartbeat("agent-1", "t1")

	// Set effective config (what we pushed)
	tracker.SetEffectiveConfig("agent-1", "config-v1")

	// Set reported config (what agent reports it's running) — same
	tracker.SetReportedConfig("agent-1", "config-v1")

	// No drift
	tracker.CheckDrift()
	mu.Lock()
	if len(events) != 0 {
		t.Errorf("expected 0 drift events, got %d", len(events))
	}
	mu.Unlock()

	// Now create drift
	tracker.SetReportedConfig("agent-1", "config-v2-different")
	tracker.CheckDrift()

	mu.Lock()
	if len(events) != 1 {
		t.Fatalf("expected 1 drift event, got %d", len(events))
	}
	if events[0].Type != "agent.config_drift" {
		t.Errorf("expected agent.config_drift, got %s", events[0].Type)
	}
	if events[0].TenantID != "t1" {
		t.Errorf("expected tenant t1, got %s", events[0].TenantID)
	}
	mu.Unlock()
}

func TestHeartbeatTracker_NoDriftWhenDisconnected(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(1*time.Millisecond, bus)

	var mu sync.Mutex
	var events []eventbus.Event
	bus.Subscribe("agent.config_drift", func(ctx context.Context, e eventbus.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	tracker.RecordHeartbeat("agent-1", "t1")
	tracker.SetEffectiveConfig("agent-1", "config-v1")
	tracker.SetReportedConfig("agent-1", "config-v2")

	// Make agent unhealthy
	time.Sleep(5 * time.Millisecond)
	tracker.CheckStale()

	// Check drift — should not emit because agent is unhealthy
	tracker.CheckDrift()

	mu.Lock()
	if len(events) != 0 {
		t.Errorf("expected no drift events for unhealthy agent, got %d", len(events))
	}
	mu.Unlock()
}

func TestHeartbeatTracker_SyncToDB(t *testing.T) {
	bus := eventbus.New()
	tracker := NewHeartbeatTracker(30*time.Second, bus)

	tracker.RecordHeartbeat("agent-1", "t1")
	tracker.RecordHeartbeat("agent-2", "t1")
	tracker.RecordHeartbeat("agent-3", "t2")

	var synced []string
	tracker.SyncToDB(func(tenantID, instanceID, status string, lastHeartbeat time.Time) {
		synced = append(synced, instanceID)
	})

	if len(synced) != 3 {
		t.Errorf("expected 3 synced agents, got %d", len(synced))
	}
}
