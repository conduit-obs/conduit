package featureflag

import (
	"hash/fnv"
	"sync"
	"time"
)

// Flag represents a feature flag.
type Flag struct {
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Enabled         bool              `json:"enabled"`
	TenantOverrides map[string]bool   `json:"tenant_overrides,omitempty"`
	PercentRollout  int               `json:"percent_rollout"` // 0-100, 0 = use Enabled
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// FlagRegistry provides in-memory feature flag evaluation.
type FlagRegistry struct {
	mu    sync.RWMutex
	flags map[string]*Flag
}

// NewRegistry creates a new feature flag registry.
func NewRegistry() *FlagRegistry {
	return &FlagRegistry{
		flags: make(map[string]*Flag),
	}
}

// Register adds or updates a flag.
func (r *FlagRegistry) Register(flag *Flag) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if flag.CreatedAt.IsZero() {
		flag.CreatedAt = now
	}
	flag.UpdatedAt = now
	r.flags[flag.Name] = flag
}

// Evaluate determines if a flag is enabled for the given tenant.
func (r *FlagRegistry) Evaluate(flagName, tenantID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	flag, ok := r.flags[flagName]
	if !ok {
		return false
	}

	// Check tenant-specific override first
	if flag.TenantOverrides != nil {
		if override, exists := flag.TenantOverrides[tenantID]; exists {
			return override
		}
	}

	// Check percentage rollout
	if flag.PercentRollout > 0 {
		h := fnv.New32a()
		h.Write([]byte(flagName + ":" + tenantID))
		bucket := int(h.Sum32() % 100)
		return bucket < flag.PercentRollout
	}

	return flag.Enabled
}

// Get returns a flag by name.
func (r *FlagRegistry) Get(name string) (*Flag, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.flags[name]
	if !ok {
		return nil, false
	}
	cp := *f
	return &cp, true
}

// List returns all flags.
func (r *FlagRegistry) List() []*Flag {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Flag
	for _, f := range r.flags {
		cp := *f
		result = append(result, &cp)
	}
	return result
}

// Delete removes a flag.
func (r *FlagRegistry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.flags, name)
}
