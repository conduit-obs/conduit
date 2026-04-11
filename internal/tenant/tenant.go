package tenant

import "context"

type contextKey struct{}

// Tenant represents a tenant in the system.
type Tenant struct {
	ID   string
	Name string
}

// WithTenant adds a tenant to the context.
func WithTenant(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// FromContext extracts the tenant from the context.
func FromContext(ctx context.Context) (*Tenant, bool) {
	t, ok := ctx.Value(contextKey{}).(*Tenant)
	return t, ok
}
