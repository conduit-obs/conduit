package db

import (
	"context"
	"fmt"
	"time"
)

// --- Quotas ---

type QuotaRow struct {
	TenantID   string `json:"tenant_id"`
	MaxAgents  int    `json:"max_agents"`
	MaxFleets  int    `json:"max_fleets"`
	MaxConfigs int    `json:"max_configs"`
	MaxAPIKeys int    `json:"max_api_keys"`
}

func (r *Repo) GetTenantQuotas(ctx context.Context, tenantID string) (*QuotaRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT tenant_id, max_agents, max_fleets, max_configs, max_api_keys FROM quotas WHERE tenant_id = $1`, tenantID)
	var q QuotaRow
	if err := row.Scan(&q.TenantID, &q.MaxAgents, &q.MaxFleets, &q.MaxConfigs, &q.MaxAPIKeys); err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *Repo) UpsertTenantQuotas(ctx context.Context, tenantID string, maxAgents, maxFleets, maxConfigs, maxAPIKeys int) (*QuotaRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO quotas (tenant_id, max_agents, max_fleets, max_configs, max_api_keys)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id) DO UPDATE SET max_agents=$2, max_fleets=$3, max_configs=$4, max_api_keys=$5, updated_at=now()
		 RETURNING tenant_id, max_agents, max_fleets, max_configs, max_api_keys`,
		tenantID, maxAgents, maxFleets, maxConfigs, maxAPIKeys)
	var q QuotaRow
	if err := row.Scan(&q.TenantID, &q.MaxAgents, &q.MaxFleets, &q.MaxConfigs, &q.MaxAPIKeys); err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *Repo) CountAgents(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL`, tenantID).Scan(&count)
	return count, err
}

func (r *Repo) CountFleets(ctx context.Context, tenantID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM fleets WHERE tenant_id = $1`, tenantID).Scan(&count)
	return count, err
}

// --- Environments ---

type EnvironmentRow struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	Name            string    `json:"name"`
	ConfigOverrides string    `json:"config_overrides"`
	CreatedAt       time.Time `json:"created_at"`
}

func (r *Repo) CreateEnvironment(ctx context.Context, tenantID, name string) (*EnvironmentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO environments (tenant_id, name) VALUES ($1, $2)
		 RETURNING id, tenant_id, name, config_overrides::text, created_at`, tenantID, name)
	var e EnvironmentRow
	if err := row.Scan(&e.ID, &e.TenantID, &e.Name, &e.ConfigOverrides, &e.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating environment: %w", err)
	}
	return &e, tx.Commit(ctx)
}

func (r *Repo) ListEnvironments(ctx context.Context, tenantID string) ([]EnvironmentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, tenant_id, name, config_overrides::text, created_at FROM environments WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var envs []EnvironmentRow
	for rows.Next() {
		var e EnvironmentRow
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Name, &e.ConfigOverrides, &e.CreatedAt); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}
	return envs, tx.Commit(ctx)
}

// --- Rollout Snapshots ---

func (r *Repo) CreateRolloutSnapshot(ctx context.Context, tenantID, rolloutID, agentID, prevYAML, prevHash string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO rollout_snapshots (tenant_id, rollout_id, agent_id, previous_config_yaml, previous_config_hash)
		 VALUES ($1, $2, $3, $4, $5)`, tenantID, rolloutID, agentID, prevYAML, prevHash)
	return err
}

type SnapshotRow struct {
	AgentID          string `json:"agent_id"`
	PreviousConfigYAML string `json:"previous_config_yaml"`
	PreviousConfigHash string `json:"previous_config_hash"`
}

func (r *Repo) GetRolloutSnapshots(ctx context.Context, rolloutID string) ([]SnapshotRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT agent_id, previous_config_yaml, previous_config_hash FROM rollout_snapshots WHERE rollout_id = $1`, rolloutID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []SnapshotRow
	for rows.Next() {
		var s SnapshotRow
		if err := rows.Scan(&s.AgentID, &s.PreviousConfigYAML, &s.PreviousConfigHash); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

// --- Rollout Approvals ---

type ApprovalRow struct {
	ID        string    `json:"id"`
	RolloutID string    `json:"rollout_id"`
	Approver  string    `json:"approver"`
	Status    string    `json:"status"`
	Comment   *string   `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *Repo) CreateRolloutApproval(ctx context.Context, tenantID, rolloutID, approver, status string, comment *string) (*ApprovalRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO rollout_approvals (tenant_id, rollout_id, approver, status, comment)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, rollout_id, approver, status, comment, created_at`,
		tenantID, rolloutID, approver, status, comment)
	var a ApprovalRow
	if err := row.Scan(&a.ID, &a.RolloutID, &a.Approver, &a.Status, &a.Comment, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

// --- Custom Roles ---

type CustomRoleRow struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
}

func (r *Repo) CreateCustomRole(ctx context.Context, tenantID, name string, permissions []string) (*CustomRoleRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO custom_roles (tenant_id, name, permissions) VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, name, permissions, created_at`, tenantID, name, permissions)
	var cr CustomRoleRow
	if err := row.Scan(&cr.ID, &cr.TenantID, &cr.Name, &cr.Permissions, &cr.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating custom role: %w", err)
	}
	return &cr, tx.Commit(ctx)
}

func (r *Repo) ListCustomRoles(ctx context.Context, tenantID string) ([]CustomRoleRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, tenant_id, name, permissions, created_at FROM custom_roles WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []CustomRoleRow
	for rows.Next() {
		var cr CustomRoleRow
		if err := rows.Scan(&cr.ID, &cr.TenantID, &cr.Name, &cr.Permissions, &cr.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, cr)
	}
	return roles, tx.Commit(ctx)
}

func (r *Repo) DeleteCustomRole(ctx context.Context, tenantID, name string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM custom_roles WHERE tenant_id = $1 AND name = $2`, tenantID, name)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- Alert Rules ---

type AlertRuleRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Condition string    `json:"condition"`
	Threshold float64   `json:"threshold"`
	Channels  []string  `json:"channels"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *Repo) CreateAlertRule(ctx context.Context, tenantID, name, condition string, threshold float64, channels []string) (*AlertRuleRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO alert_rules (tenant_id, name, condition, threshold, channels)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, condition, threshold, channels, enabled, created_at`,
		tenantID, name, condition, threshold, channels)
	var ar AlertRuleRow
	if err := row.Scan(&ar.ID, &ar.TenantID, &ar.Name, &ar.Condition, &ar.Threshold, &ar.Channels, &ar.Enabled, &ar.CreatedAt); err != nil {
		return nil, err
	}
	return &ar, tx.Commit(ctx)
}

func (r *Repo) ListAlertRules(ctx context.Context, tenantID string) ([]AlertRuleRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, tenant_id, name, condition, threshold, channels, enabled, created_at FROM alert_rules WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []AlertRuleRow
	for rows.Next() {
		var ar AlertRuleRow
		if err := rows.Scan(&ar.ID, &ar.TenantID, &ar.Name, &ar.Condition, &ar.Threshold, &ar.Channels, &ar.Enabled, &ar.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, ar)
	}
	return rules, tx.Commit(ctx)
}

// --- Usage Snapshots ---

type UsageSnapshotRow struct {
	TenantID         string `json:"tenant_id"`
	SnapshotDate     string `json:"snapshot_date"`
	AgentCount       int    `json:"agent_count"`
	APICalls         int64  `json:"api_calls"`
	ConfigDeployments int   `json:"config_deployments"`
}

func (r *Repo) ListUsageSnapshots(ctx context.Context, tenantID string) ([]UsageSnapshotRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT tenant_id, snapshot_date::text, agent_count, api_calls, config_deployments
		 FROM usage_snapshots WHERE tenant_id = $1 ORDER BY snapshot_date DESC LIMIT 30`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []UsageSnapshotRow
	for rows.Next() {
		var s UsageSnapshotRow
		if err := rows.Scan(&s.TenantID, &s.SnapshotDate, &s.AgentCount, &s.APICalls, &s.ConfigDeployments); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

// --- Entitlements ---

type EntitlementRow struct {
	TenantID          string `json:"tenant_id"`
	Tier              string `json:"tier"`
	MaxAgents         int    `json:"max_agents"`
	MaxFleets         int    `json:"max_fleets"`
	MaxAPICallsPerMin int    `json:"max_api_calls_per_min"`
}

func (r *Repo) GetEntitlements(ctx context.Context, tenantID string) (*EntitlementRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT tenant_id, tier, max_agents, max_fleets, max_api_calls_per_min FROM entitlements WHERE tenant_id = $1`, tenantID)
	var e EntitlementRow
	if err := row.Scan(&e.TenantID, &e.Tier, &e.MaxAgents, &e.MaxFleets, &e.MaxAPICallsPerMin); err != nil {
		return nil, err
	}
	return &e, nil
}

// --- Audit Log Query ---

type AuditLogRow struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	EventType    string    `json:"event_type"`
	Payload      string    `json:"payload"`
	RequestID    *string   `json:"request_id,omitempty"`
	Actor        *string   `json:"actor,omitempty"`
	ResourceType *string   `json:"resource_type,omitempty"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (r *Repo) QueryAuditLogs(ctx context.Context, tenantID, actor, action, from, to string, limit, offset int) ([]AuditLogRow, error) {
	query := `SELECT id, tenant_id, event_type, payload::text, request_id, actor, resource_type, resource_id, created_at
		 FROM events WHERE tenant_id = $1`
	args := []any{tenantID}
	idx := 2

	if actor != "" {
		query += fmt.Sprintf(` AND actor = $%d`, idx)
		args = append(args, actor)
		idx++
	}
	if action != "" {
		query += fmt.Sprintf(` AND event_type = $%d`, idx)
		args = append(args, action)
		idx++
	}
	if from != "" {
		query += fmt.Sprintf(` AND created_at >= $%d`, idx)
		args = append(args, from)
		idx++
	}
	if to != "" {
		query += fmt.Sprintf(` AND created_at <= $%d`, idx)
		args = append(args, to)
		idx++
	}

	query += ` ORDER BY created_at DESC`

	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(` LIMIT $%d`, idx)
	args = append(args, limit)
	idx++

	if offset > 0 {
		query += fmt.Sprintf(` OFFSET $%d`, idx)
		args = append(args, offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []AuditLogRow
	for rows.Next() {
		var al AuditLogRow
		if err := rows.Scan(&al.ID, &al.TenantID, &al.EventType, &al.Payload, &al.RequestID, &al.Actor, &al.ResourceType, &al.ResourceID, &al.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, al)
	}
	return logs, nil
}

// --- GDPR Data Deletion ---

func (r *Repo) DeleteAllTenantData(ctx context.Context, tenantID string) error {
	tables := []string{"rollout_agents", "rollout_history", "rollout_snapshots", "rollout_approvals",
		"rollouts", "agent_config_history", "webhooks", "api_keys", "config_intents",
		"fleets", "agents", "events", "pipeline_templates", "policy_packs",
		"feature_flags", "alert_rules", "usage_snapshots", "entitlements", "quotas",
		"environments", "retention_policies", "custom_roles"}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, table := range tables {
		_, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE tenant_id = $1", table), tenantID)
		if err != nil {
			// Some tables may not have tenant_id column — skip
			continue
		}
	}

	_, err = tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// --- Compliance Export ---

func (r *Repo) ExportComplianceData(ctx context.Context, tenantID, from, to string) ([]AuditLogRow, error) {
	return r.QueryAuditLogs(ctx, tenantID, "", "", from, to, 10000, 0)
}
