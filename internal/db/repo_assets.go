package db

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type OrganizationRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectRow struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func (r *Repo) CreateOrganization(ctx context.Context, tenantID, name string) (*OrganizationRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	row := tx.QueryRow(ctx,
		`INSERT INTO organizations (tenant_id, name, slug) VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, name, slug, created_at`, tenantID, name, slugify(name))
	var o OrganizationRow
	if err := row.Scan(&o.ID, &o.TenantID, &o.Name, &o.Slug, &o.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating organization: %w", err)
	}
	return &o, tx.Commit(ctx)
}

func (r *Repo) ListOrganizations(ctx context.Context, tenantID string) ([]OrganizationRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	rows, err := tx.Query(ctx, `SELECT id, tenant_id, name, slug, created_at FROM organizations WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	var orgs []OrganizationRow
	for rows.Next() {
		var o OrganizationRow
		if err := rows.Scan(&o.ID, &o.TenantID, &o.Name, &o.Slug, &o.CreatedAt); err != nil { return nil, err }
		orgs = append(orgs, o)
	}
	return orgs, tx.Commit(ctx)
}

func (r *Repo) GetOrganization(ctx context.Context, tenantID, orgID string) (*OrganizationRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	row := tx.QueryRow(ctx, `SELECT id, tenant_id, name, slug, created_at FROM organizations WHERE id = $1 AND tenant_id = $2`, orgID, tenantID)
	var o OrganizationRow
	if err := row.Scan(&o.ID, &o.TenantID, &o.Name, &o.Slug, &o.CreatedAt); err != nil { return nil, fmt.Errorf("getting organization: %w", err) }
	return &o, tx.Commit(ctx)
}

func (r *Repo) CreateProject(ctx context.Context, tenantID, orgID, name string) (*ProjectRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	row := tx.QueryRow(ctx,
		`INSERT INTO projects (org_id, tenant_id, name, slug) VALUES ($1, $2, $3, $4)
		 RETURNING id, org_id, tenant_id, name, slug, created_at`, orgID, tenantID, name, slugify(name))
	var p ProjectRow
	if err := row.Scan(&p.ID, &p.OrgID, &p.TenantID, &p.Name, &p.Slug, &p.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	return &p, tx.Commit(ctx)
}

func (r *Repo) ListProjects(ctx context.Context, tenantID, orgID string) ([]ProjectRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	rows, err := tx.Query(ctx, `SELECT id, org_id, tenant_id, name, slug, created_at FROM projects WHERE tenant_id = $1 AND org_id = $2 ORDER BY name`, tenantID, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var projects []ProjectRow
	for rows.Next() {
		var p ProjectRow
		if err := rows.Scan(&p.ID, &p.OrgID, &p.TenantID, &p.Name, &p.Slug, &p.CreatedAt); err != nil { return nil, err }
		projects = append(projects, p)
	}
	return projects, tx.Commit(ctx)
}

func (r *Repo) GetProject(ctx context.Context, tenantID, projectID string) (*ProjectRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil { return nil, err }
	row := tx.QueryRow(ctx, `SELECT id, org_id, tenant_id, name, slug, created_at FROM projects WHERE id = $1 AND tenant_id = $2`, projectID, tenantID)
	var p ProjectRow
	if err := row.Scan(&p.ID, &p.OrgID, &p.TenantID, &p.Name, &p.Slug, &p.CreatedAt); err != nil { return nil, fmt.Errorf("getting project: %w", err) }
	return &p, tx.Commit(ctx)
}
