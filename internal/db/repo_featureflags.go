package db

import (
	"context"
	"fmt"
	"time"
)

// FeatureFlagRow represents a row in the feature_flags table.
type FeatureFlagRow struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Enabled         bool      `json:"enabled"`
	TenantOverrides string    `json:"tenant_overrides"`
	PercentRollout  int       `json:"percent_rollout"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateFeatureFlag creates a new feature flag.
func (r *Repo) CreateFeatureFlag(ctx context.Context, name, description string, enabled bool, tenantOverrides string, percentRollout int) (*FeatureFlagRow, error) {
	if tenantOverrides == "" {
		tenantOverrides = "{}"
	}
	row := r.pool.QueryRow(ctx,
		`INSERT INTO feature_flags (name, description, enabled, tenant_overrides, percent_rollout)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, description, enabled, tenant_overrides::text, percent_rollout, created_at, updated_at`,
		name, description, enabled, tenantOverrides, percentRollout)

	var f FeatureFlagRow
	if err := row.Scan(&f.ID, &f.Name, &f.Description, &f.Enabled, &f.TenantOverrides, &f.PercentRollout, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, fmt.Errorf("creating feature flag: %w", err)
	}
	return &f, nil
}

// GetFeatureFlag returns a feature flag by name.
func (r *Repo) GetFeatureFlag(ctx context.Context, name string) (*FeatureFlagRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, name, description, enabled, tenant_overrides::text, percent_rollout, created_at, updated_at
		 FROM feature_flags WHERE name = $1`, name)

	var f FeatureFlagRow
	if err := row.Scan(&f.ID, &f.Name, &f.Description, &f.Enabled, &f.TenantOverrides, &f.PercentRollout, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, fmt.Errorf("getting feature flag: %w", err)
	}
	return &f, nil
}

// ListFeatureFlags returns all feature flags.
func (r *Repo) ListFeatureFlags(ctx context.Context) ([]FeatureFlagRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, enabled, tenant_overrides::text, percent_rollout, created_at, updated_at
		 FROM feature_flags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing feature flags: %w", err)
	}
	defer rows.Close()

	var flags []FeatureFlagRow
	for rows.Next() {
		var f FeatureFlagRow
		if err := rows.Scan(&f.ID, &f.Name, &f.Description, &f.Enabled, &f.TenantOverrides, &f.PercentRollout, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, nil
}

// UpdateFeatureFlag updates a feature flag.
func (r *Repo) UpdateFeatureFlag(ctx context.Context, name string, enabled *bool, percentRollout *int, tenantOverrides *string) (*FeatureFlagRow, error) {
	// Build dynamic update
	query := `UPDATE feature_flags SET updated_at = now()`
	args := []any{}
	argIdx := 1

	if enabled != nil {
		args = append(args, *enabled)
		query += fmt.Sprintf(`, enabled = $%d`, argIdx)
		argIdx++
	}
	if percentRollout != nil {
		args = append(args, *percentRollout)
		query += fmt.Sprintf(`, percent_rollout = $%d`, argIdx)
		argIdx++
	}
	if tenantOverrides != nil {
		args = append(args, *tenantOverrides)
		query += fmt.Sprintf(`, tenant_overrides = $%d`, argIdx)
		argIdx++
	}

	args = append(args, name)
	query += fmt.Sprintf(` WHERE name = $%d RETURNING id, name, description, enabled, tenant_overrides::text, percent_rollout, created_at, updated_at`, argIdx)

	row := r.pool.QueryRow(ctx, query, args...)
	var f FeatureFlagRow
	if err := row.Scan(&f.ID, &f.Name, &f.Description, &f.Enabled, &f.TenantOverrides, &f.PercentRollout, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, fmt.Errorf("updating feature flag: %w", err)
	}
	return &f, nil
}
