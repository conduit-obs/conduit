package db

import (
	"context"
	"fmt"
	"time"
)

// TemplateRow represents a row in the pipeline_templates table.
type TemplateRow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Parameters  string    `json:"parameters"`
	IntentJSON  string    `json:"intent_json"`
	Deprecated  bool      `json:"deprecated"`
	CreatedAt   time.Time `json:"created_at"`
}

// PolicyPackRow represents a row in the policy_packs table.
type PolicyPackRow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	PackJSON    string    `json:"pack_json"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateTemplate creates a new pipeline template.
func (r *Repo) CreateTemplate(ctx context.Context, tenantID, name, version, description, category, parametersJSON, intentJSON string) (*TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO pipeline_templates (tenant_id, name, version, description, category, parameters, intent_json)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at`,
		tenantID, name, version, description, category, parametersJSON, intentJSON)

	var t TemplateRow
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating template: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns all templates for a tenant (latest version of each name).
func (r *Repo) ListTemplates(ctx context.Context, tenantID string) ([]TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT DISTINCT ON (name) id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at
		 FROM pipeline_templates WHERE tenant_id = $1
		 ORDER BY name, created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return templates, nil
}

// ListTemplatesByCategory returns templates filtered by category.
func (r *Repo) ListTemplatesByCategory(ctx context.Context, tenantID, category string) ([]TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT DISTINCT ON (name) id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at
		 FROM pipeline_templates WHERE tenant_id = $1 AND category = $2
		 ORDER BY name, created_at DESC`, tenantID, category)
	if err != nil {
		return nil, fmt.Errorf("listing templates by category: %w", err)
	}
	defer rows.Close()

	var templates []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return templates, nil
}

// GetTemplate returns the latest version of a template by name.
func (r *Repo) GetTemplate(ctx context.Context, tenantID, name string) (*TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at
		 FROM pipeline_templates WHERE tenant_id = $1 AND name = $2
		 ORDER BY created_at DESC LIMIT 1`, tenantID, name)

	var t TemplateRow
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting template: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTemplateByVersion returns a specific version of a template.
func (r *Repo) GetTemplateByVersion(ctx context.Context, tenantID, name, version string) (*TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at
		 FROM pipeline_templates WHERE tenant_id = $1 AND name = $2 AND version = $3`, tenantID, name, version)

	var t TemplateRow
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting template version: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplateVersions returns all versions of a template.
func (r *Repo) ListTemplateVersions(ctx context.Context, tenantID, name string) ([]TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at
		 FROM pipeline_templates WHERE tenant_id = $1 AND name = $2
		 ORDER BY created_at ASC`, tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("listing template versions: %w", err)
	}
	defer rows.Close()

	var templates []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return templates, nil
}

// DeprecateTemplate marks a template as deprecated.
func (r *Repo) DeprecateTemplate(ctx context.Context, tenantID, name string) (*TemplateRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`UPDATE pipeline_templates SET deprecated = true
		 WHERE id = (
			SELECT id FROM pipeline_templates
			WHERE tenant_id = $1 AND name = $2
			ORDER BY created_at DESC LIMIT 1
		 )
		 RETURNING id, tenant_id, name, version, description, category, parameters::text, intent_json::text, deprecated, created_at`,
		tenantID, name)

	var t TemplateRow
	if err := row.Scan(&t.ID, &t.TenantID, &t.Name, &t.Version, &t.Description, &t.Category, &t.Parameters, &t.IntentJSON, &t.Deprecated, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("deprecating template: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &t, nil
}

// CreatePolicyPack creates a new policy pack.
func (r *Repo) CreatePolicyPack(ctx context.Context, tenantID, name, version, description, packJSON string) (*PolicyPackRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO policy_packs (tenant_id, name, version, description, pack_json)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, version, description, pack_json::text, created_at`,
		tenantID, name, version, description, packJSON)

	var pp PolicyPackRow
	if err := row.Scan(&pp.ID, &pp.TenantID, &pp.Name, &pp.Version, &pp.Description, &pp.PackJSON, &pp.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating policy pack: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &pp, nil
}

// ListPolicyPacks returns all policy packs for a tenant.
func (r *Repo) ListPolicyPacks(ctx context.Context, tenantID string) ([]PolicyPackRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT DISTINCT ON (name) id, tenant_id, name, version, description, pack_json::text, created_at
		 FROM policy_packs WHERE tenant_id = $1
		 ORDER BY name, created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing policy packs: %w", err)
	}
	defer rows.Close()

	var packs []PolicyPackRow
	for rows.Next() {
		var pp PolicyPackRow
		if err := rows.Scan(&pp.ID, &pp.TenantID, &pp.Name, &pp.Version, &pp.Description, &pp.PackJSON, &pp.CreatedAt); err != nil {
			return nil, err
		}
		packs = append(packs, pp)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return packs, nil
}

// GetPolicyPack returns a policy pack by name.
func (r *Repo) GetPolicyPack(ctx context.Context, tenantID, name string) (*PolicyPackRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, description, pack_json::text, created_at
		 FROM policy_packs WHERE tenant_id = $1 AND name = $2
		 ORDER BY created_at DESC LIMIT 1`, tenantID, name)

	var pp PolicyPackRow
	if err := row.Scan(&pp.ID, &pp.TenantID, &pp.Name, &pp.Version, &pp.Description, &pp.PackJSON, &pp.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting policy pack: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &pp, nil
}
