package featureflag

import (
	"testing"
)

func TestEvaluate_Enabled(t *testing.T) {
	r := NewRegistry()
	r.Register(&Flag{Name: "test-flag", Enabled: true})

	if !r.Evaluate("test-flag", "tenant-1") {
		t.Error("expected flag to be enabled")
	}
}

func TestEvaluate_Disabled(t *testing.T) {
	r := NewRegistry()
	r.Register(&Flag{Name: "test-flag", Enabled: false})

	if r.Evaluate("test-flag", "tenant-1") {
		t.Error("expected flag to be disabled")
	}
}

func TestEvaluate_NotFound(t *testing.T) {
	r := NewRegistry()
	if r.Evaluate("nonexistent", "tenant-1") {
		t.Error("expected false for nonexistent flag")
	}
}

func TestEvaluate_TenantOverride(t *testing.T) {
	r := NewRegistry()
	r.Register(&Flag{
		Name:            "test-flag",
		Enabled:         false,
		TenantOverrides: map[string]bool{"tenant-1": true, "tenant-2": false},
	})

	if !r.Evaluate("test-flag", "tenant-1") {
		t.Error("expected override to enable for tenant-1")
	}
	if r.Evaluate("test-flag", "tenant-2") {
		t.Error("expected override to disable for tenant-2")
	}
	if r.Evaluate("test-flag", "tenant-3") {
		t.Error("expected default (disabled) for tenant-3")
	}
}

func TestEvaluate_PercentRollout(t *testing.T) {
	r := NewRegistry()
	r.Register(&Flag{Name: "rollout-flag", PercentRollout: 50})

	enabled := 0
	for i := 0; i < 100; i++ {
		if r.Evaluate("rollout-flag", "tenant-"+string(rune('A'+i))) {
			enabled++
		}
	}

	// With 50% rollout over 100 tenants, expect roughly 30-70
	if enabled < 20 || enabled > 80 {
		t.Errorf("expected ~50%% enabled, got %d%%", enabled)
	}
}

func TestRegistry_CRUD(t *testing.T) {
	r := NewRegistry()

	r.Register(&Flag{Name: "flag-1", Enabled: true})
	r.Register(&Flag{Name: "flag-2", Enabled: false})

	flags := r.List()
	if len(flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(flags))
	}

	f, ok := r.Get("flag-1")
	if !ok || !f.Enabled {
		t.Error("expected to find enabled flag-1")
	}

	r.Delete("flag-1")
	_, ok = r.Get("flag-1")
	if ok {
		t.Error("expected flag-1 to be deleted")
	}
}
