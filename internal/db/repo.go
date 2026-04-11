package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantRow represents a row in the tenants table.
type TenantRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RateLimit int       `json:"rate_limit"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentRow represents a row in the agents table.
type AgentRow struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	LastHeartbeat   *time.Time `json:"last_heartbeat,omitempty"`
	EffectiveConfig *string    `json:"effective_config,omitempty"`
	Labels          string     `json:"labels"`
	Capabilities    string     `json:"capabilities"`
	HealthScore     int        `json:"health_score"`
	Topology        string     `json:"topology"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// WebhookRow represents a row in the webhooks table.
type WebhookRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// APIKeyRow represents a row in the api_keys table.
type APIKeyRow struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenant_id"`
	Name        string     `json:"name"`
	KeyPrefix   string     `json:"key_prefix"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// AgentConfigHistoryRow represents a row in the agent_config_history table.
type AgentConfigHistoryRow struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	AgentID    string    `json:"agent_id"`
	ConfigYAML string    `json:"config_yaml"`
	ConfigHash string    `json:"config_hash"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
}

// ConfigIntentRow represents a row in the config_intents table.
type ConfigIntentRow struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	Version      int       `json:"version"`
	IntentJSON   string    `json:"intent_json"`
	CompiledYAML *string   `json:"compiled_yaml,omitempty"`
	Promoted     bool      `json:"promoted"`
	Tags         []string  `json:"tags"`
	CreatedAt    time.Time `json:"created_at"`
}

// RolloutAgentRow represents a row in the rollout_agents table.
type RolloutAgentRow struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	RolloutID  string    `json:"rollout_id"`
	AgentID    string    `json:"agent_id"`
	Status     string    `json:"status"`
	ConfigHash *string   `json:"config_hash,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RolloutHistoryRow represents a row in the rollout_history table.
type RolloutHistoryRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	RolloutID string    `json:"rollout_id"`
	Status    string    `json:"status"`
	Message   *string   `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Repo provides data access with tenant isolation.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo creates a new repository.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// setTenantContext sets the tenant context for RLS on the given connection.
func setTenantContext(ctx context.Context, tx pgx.Tx, tenantID string) error {
	_, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.tenant_id = '%s'", tenantID))
	return err
}

// --- Tenants ---

// CreateTenant creates a new tenant.
func (r *Repo) CreateTenant(ctx context.Context, name string) (*TenantRow, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO tenants (name) VALUES ($1) RETURNING id, name, rate_limit, created_at`, name)

	var t TenantRow
	if err := row.Scan(&t.ID, &t.Name, &t.RateLimit, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating tenant: %w", err)
	}
	return &t, nil
}

// GetTenantByID returns a tenant by ID.
func (r *Repo) GetTenantByID(ctx context.Context, id string) (*TenantRow, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, name, rate_limit, created_at FROM tenants WHERE id = $1`, id)

	var t TenantRow
	if err := row.Scan(&t.ID, &t.Name, &t.RateLimit, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting tenant: %w", err)
	}
	return &t, nil
}

// --- Agents (tenant-scoped) ---

// CreateAgent creates a new agent within a tenant.
func (r *Repo) CreateAgent(ctx context.Context, tenantID, name string, labels map[string]string) (*AgentRow, error) {
	labelsJSON, _ := json.Marshal(labels)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO agents (tenant_id, name, labels) VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at`,
		tenantID, name, labelsJSON)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// ListAgents returns all agents for a tenant.
func (r *Repo) ListAgents(ctx context.Context, tenantID string) ([]AgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateAgentStatus updates the status, heartbeat, and effective config for an agent by name.
func (r *Repo) UpdateAgentStatus(ctx context.Context, tenantID, agentName, status string, lastHeartbeat time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE agents SET status = $1, last_heartbeat = $2, updated_at = now()
		 WHERE name = $3 AND tenant_id = $4`, status, lastHeartbeat, agentName, tenantID)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}

	return tx.Commit(ctx)
}

// UpdateAgentHeartbeat updates the heartbeat and status for an agent.
func (r *Repo) UpdateAgentHeartbeat(ctx context.Context, tenantID, agentName, status string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE agents SET status = $1, last_heartbeat = now(), updated_at = now()
		 WHERE name = $2 AND tenant_id = $3`, status, agentName, tenantID)
	if err != nil {
		return fmt.Errorf("updating heartbeat: %w", err)
	}

	return tx.Commit(ctx)
}

func scanAgent(row pgx.Row) (*AgentRow, error) {
	var a AgentRow
	err := row.Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat,
		&a.EffectiveConfig, &a.Labels, &a.Capabilities, &a.HealthScore, &a.Topology, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanAgentRows(rows pgx.Rows) (*AgentRow, error) {
	var a AgentRow
	err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.Status, &a.LastHeartbeat,
		&a.EffectiveConfig, &a.Labels, &a.Capabilities, &a.HealthScore, &a.Topology, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const agentColumns = `id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at`

// GetAgentByID returns an agent by ID.
func (r *Repo) GetAgentByID(ctx context.Context, tenantID, agentID string) (*AgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at
		 FROM agents WHERE id = $1 AND tenant_id = $2`, agentID, tenantID)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("getting agent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgentLabels updates the labels of an agent.
func (r *Repo) UpdateAgentLabels(ctx context.Context, tenantID, agentID string, labels map[string]string) (*AgentRow, error) {
	labelsJSON, _ := json.Marshal(labels)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`UPDATE agents SET labels = $1, updated_at = now()
		 WHERE id = $2 AND tenant_id = $3
		 RETURNING id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at`,
		labelsJSON, agentID, tenantID)

	a, err := scanAgent(row)
	if err != nil {
		return nil, fmt.Errorf("updating agent labels: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return a, nil
}

// --- Config Intents (tenant-scoped) ---

// CreateConfigIntent stores a new config intent with its compiled YAML.
func (r *Repo) CreateConfigIntent(ctx context.Context, tenantID, name, intentJSON string, compiledYAML *string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	// Get next version for this intent name
	var nextVersion int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM config_intents WHERE tenant_id = $1 AND name = $2`,
		tenantID, name).Scan(&nextVersion)
	if err != nil {
		return nil, fmt.Errorf("getting next version: %w", err)
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO config_intents (tenant_id, name, version, intent_json, compiled_yaml)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at`,
		tenantID, name, nextVersion, intentJSON, compiledYAML)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating config intent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// ListConfigIntents returns config intents for a tenant.
func (r *Repo) ListConfigIntents(ctx context.Context, tenantID string) ([]ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE tenant_id = $1 ORDER BY name, version DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing intents: %w", err)
	}
	defer rows.Close()

	var intents []ConfigIntentRow
	for rows.Next() {
		var ci ConfigIntentRow
		if err := rows.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
			return nil, err
		}
		intents = append(intents, ci)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return intents, nil
}

// GetConfigIntent returns a specific config intent by name (latest version).
func (r *Repo) GetConfigIntent(ctx context.Context, tenantID, name string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE tenant_id = $1 AND name = $2
		 ORDER BY version DESC LIMIT 1`, tenantID, name)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting intent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// GetConfigIntentByVersion returns a config intent by name and specific version.
func (r *Repo) GetConfigIntentByVersion(ctx context.Context, tenantID, name string, version int) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE tenant_id = $1 AND name = $2 AND version = $3`,
		tenantID, name, version)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting intent version %d: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// --- Fleets (tenant-scoped) ---

// FleetRow represents a row in the fleets table.
type FleetRow struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Selector  string    `json:"selector"`  // JSON object of label key/value pairs
	Variables string    `json:"variables"` // JSON object of template variables
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateFleet creates a new fleet with a label selector.
func (r *Repo) CreateFleet(ctx context.Context, tenantID, name string, selector map[string]string) (*FleetRow, error) {
	selectorJSON, _ := json.Marshal(selector)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO fleets (tenant_id, name, selector) VALUES ($1, $2, $3)
		 RETURNING id, tenant_id, name, selector::text, variables::text, created_at, updated_at`,
		tenantID, name, selectorJSON)

	var f FleetRow
	if err := row.Scan(&f.ID, &f.TenantID, &f.Name, &f.Selector, &f.Variables, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, fmt.Errorf("creating fleet: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &f, nil
}

// ListFleets returns all fleets for a tenant.
func (r *Repo) ListFleets(ctx context.Context, tenantID string) ([]FleetRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, selector::text, variables::text, created_at, updated_at
		 FROM fleets WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing fleets: %w", err)
	}
	defer rows.Close()

	var fleets []FleetRow
	for rows.Next() {
		var f FleetRow
		if err := rows.Scan(&f.ID, &f.TenantID, &f.Name, &f.Selector, &f.Variables, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		fleets = append(fleets, f)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return fleets, nil
}

// GetFleet returns a fleet by ID.
func (r *Repo) GetFleet(ctx context.Context, tenantID, fleetID string) (*FleetRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, selector::text, variables::text, created_at, updated_at
		 FROM fleets WHERE id = $1 AND tenant_id = $2`, fleetID, tenantID)

	var f FleetRow
	if err := row.Scan(&f.ID, &f.TenantID, &f.Name, &f.Selector, &f.Variables, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, fmt.Errorf("getting fleet: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &f, nil
}

// MatchAgentsBySelector returns agents whose labels contain all key/value pairs in the selector.
func (r *Repo) MatchAgentsBySelector(ctx context.Context, tenantID string, selector map[string]string) ([]AgentRow, error) {
	selectorJSON, _ := json.Marshal(selector)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND labels @> $2 AND deleted_at IS NULL ORDER BY name`, tenantID, selectorJSON)
	if err != nil {
		return nil, fmt.Errorf("matching agents: %w", err)
	}
	defer rows.Close()

	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// --- Rollouts (tenant-scoped) ---

// RolloutRow represents a row in the rollouts table.
type RolloutRow struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenant_id"`
	FleetID        string     `json:"fleet_id"`
	IntentID       string     `json:"intent_id"`
	Status         string     `json:"status"`
	TargetCount    int        `json:"target_count"`
	CompletedCount int        `json:"completed_count"`
	Strategy       string     `json:"strategy"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// CreateRollout creates a new rollout targeting a fleet with a config intent.
func (r *Repo) CreateRollout(ctx context.Context, tenantID, fleetID, intentID string, targetCount int, strategyJSON string) (*RolloutRow, error) {
	return r.CreateRolloutWithSchedule(ctx, tenantID, fleetID, intentID, targetCount, strategyJSON, nil)
}

// CreateRolloutWithSchedule creates a rollout, optionally scheduled for a future time.
func (r *Repo) CreateRolloutWithSchedule(ctx context.Context, tenantID, fleetID, intentID string, targetCount int, strategyJSON string, scheduledAt *time.Time) (*RolloutRow, error) {
	if strategyJSON == "" {
		strategyJSON = `{"type":"all-at-once"}`
	}

	status := "in_progress"
	if scheduledAt != nil {
		status = "scheduled"
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO rollouts (tenant_id, fleet_id, intent_id, status, target_count, strategy, scheduled_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, tenant_id, fleet_id, intent_id, status, target_count, completed_count, strategy::text, scheduled_at, created_at, updated_at`,
		tenantID, fleetID, intentID, status, targetCount, strategyJSON, scheduledAt)

	var ro RolloutRow
	if err := row.Scan(&ro.ID, &ro.TenantID, &ro.FleetID, &ro.IntentID, &ro.Status,
		&ro.TargetCount, &ro.CompletedCount, &ro.Strategy, &ro.ScheduledAt, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
		return nil, fmt.Errorf("creating rollout: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ro, nil
}

// UpdateRolloutStatus updates the status and completed count of a rollout.
func (r *Repo) UpdateRolloutStatus(ctx context.Context, tenantID, rolloutID, status string, completedCount int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE rollouts SET status = $1, completed_count = $2, updated_at = now()
		 WHERE id = $3 AND tenant_id = $4`, status, completedCount, rolloutID, tenantID)
	if err != nil {
		return fmt.Errorf("updating rollout: %w", err)
	}

	return tx.Commit(ctx)
}

// GetRollout returns a rollout by ID.
func (r *Repo) GetRollout(ctx context.Context, tenantID, rolloutID string) (*RolloutRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, fleet_id, intent_id, status, target_count, completed_count, strategy::text, scheduled_at, created_at, updated_at
		 FROM rollouts WHERE id = $1 AND tenant_id = $2`, rolloutID, tenantID)

	var ro RolloutRow
	if err := row.Scan(&ro.ID, &ro.TenantID, &ro.FleetID, &ro.IntentID, &ro.Status,
		&ro.TargetCount, &ro.CompletedCount, &ro.Strategy, &ro.ScheduledAt, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
		return nil, fmt.Errorf("getting rollout: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ro, nil
}

// PromoteConfigIntent marks the latest version of an intent as promoted.
func (r *Repo) PromoteConfigIntent(ctx context.Context, tenantID, name string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`UPDATE config_intents SET promoted = true
		 WHERE id = (
			SELECT id FROM config_intents
			WHERE tenant_id = $1 AND name = $2
			ORDER BY version DESC LIMIT 1
		 )
		 RETURNING id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at`,
		tenantID, name)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("promoting intent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// IsIntentPromoted checks if a config intent is promoted.
func (r *Repo) IsIntentPromoted(ctx context.Context, tenantID, intentID string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return false, err
	}

	var promoted bool
	err = tx.QueryRow(ctx,
		`SELECT promoted FROM config_intents WHERE id = $1 AND tenant_id = $2`,
		intentID, tenantID).Scan(&promoted)
	if err != nil {
		return false, err
	}

	tx.Commit(ctx)
	return promoted, nil
}

// ListRollouts returns rollouts for a tenant with optional filtering.
func (r *Repo) ListRollouts(ctx context.Context, tenantID string, fleetID, status string) ([]RolloutRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	query := `SELECT id, tenant_id, fleet_id, intent_id, status, target_count, completed_count, strategy::text, scheduled_at, created_at, updated_at
		 FROM rollouts WHERE tenant_id = $1`
	args := []any{tenantID}

	if fleetID != "" {
		args = append(args, fleetID)
		query += fmt.Sprintf(` AND fleet_id = $%d`, len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing rollouts: %w", err)
	}
	defer rows.Close()

	var rollouts []RolloutRow
	for rows.Next() {
		var ro RolloutRow
		if err := rows.Scan(&ro.ID, &ro.TenantID, &ro.FleetID, &ro.IntentID, &ro.Status,
			&ro.TargetCount, &ro.CompletedCount, &ro.Strategy, &ro.ScheduledAt, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
			return nil, err
		}
		rollouts = append(rollouts, ro)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return rollouts, nil
}

// --- Rollout Agents (per-agent config acknowledgment) ---

// CreateRolloutAgent creates a per-agent rollout tracking entry.
func (r *Repo) CreateRolloutAgent(ctx context.Context, tenantID, rolloutID, agentID, configHash string) (*RolloutAgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO rollout_agents (tenant_id, rollout_id, agent_id, config_hash)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, rollout_id, agent_id, status, config_hash, created_at, updated_at`,
		tenantID, rolloutID, agentID, configHash)

	var ra RolloutAgentRow
	if err := row.Scan(&ra.ID, &ra.TenantID, &ra.RolloutID, &ra.AgentID, &ra.Status, &ra.ConfigHash, &ra.CreatedAt, &ra.UpdatedAt); err != nil {
		return nil, fmt.Errorf("creating rollout agent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ra, nil
}

// CreateRolloutAgentWithPhase creates a per-agent rollout entry with a phase (canary/remainder/all).
func (r *Repo) CreateRolloutAgentWithPhase(ctx context.Context, tenantID, rolloutID, agentID, configHash, phase string) (*RolloutAgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO rollout_agents (tenant_id, rollout_id, agent_id, config_hash, phase)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, rollout_id, agent_id, status, config_hash, created_at, updated_at`,
		tenantID, rolloutID, agentID, configHash, phase)

	var ra RolloutAgentRow
	if err := row.Scan(&ra.ID, &ra.TenantID, &ra.RolloutID, &ra.AgentID, &ra.Status, &ra.ConfigHash, &ra.CreatedAt, &ra.UpdatedAt); err != nil {
		return nil, fmt.Errorf("creating rollout agent with phase: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ra, nil
}

// ListRolloutAgents returns per-agent status for a rollout.
func (r *Repo) ListRolloutAgents(ctx context.Context, tenantID, rolloutID string) ([]RolloutAgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, rollout_id, agent_id, status, config_hash, created_at, updated_at
		 FROM rollout_agents WHERE rollout_id = $1 AND tenant_id = $2 ORDER BY created_at`,
		rolloutID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing rollout agents: %w", err)
	}
	defer rows.Close()

	var agents []RolloutAgentRow
	for rows.Next() {
		var ra RolloutAgentRow
		if err := rows.Scan(&ra.ID, &ra.TenantID, &ra.RolloutID, &ra.AgentID, &ra.Status, &ra.ConfigHash, &ra.CreatedAt, &ra.UpdatedAt); err != nil {
			return nil, err
		}
		agents = append(agents, ra)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// UpdateRolloutAgentStatus updates the acknowledgment status for a specific agent in a rollout.
func (r *Repo) UpdateRolloutAgentStatus(ctx context.Context, tenantID, rolloutID, agentID, status string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE rollout_agents SET status = $1, updated_at = now()
		 WHERE rollout_id = $2 AND agent_id = $3 AND tenant_id = $4`,
		status, rolloutID, agentID, tenantID)
	if err != nil {
		return fmt.Errorf("updating rollout agent: %w", err)
	}

	return tx.Commit(ctx)
}

// AcknowledgeConfigByHash finds a pending rollout_agent entry matching the config hash and marks it acknowledged.
func (r *Repo) AcknowledgeConfigByHash(ctx context.Context, tenantID, agentID, configHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE rollout_agents SET status = 'acknowledged', updated_at = now()
		 WHERE agent_id = $1 AND config_hash = $2 AND status = 'pending' AND tenant_id = $3`,
		agentID, configHash, tenantID)
	if err != nil {
		return fmt.Errorf("acknowledging config: %w", err)
	}

	return tx.Commit(ctx)
}

// --- Rollout History ---

// CreateRolloutHistory records a status transition for a rollout.
func (r *Repo) CreateRolloutHistory(ctx context.Context, tenantID, rolloutID, status, message string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	var msg *string
	if message != "" {
		msg = &message
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO rollout_history (tenant_id, rollout_id, status, message) VALUES ($1, $2, $3, $4)`,
		tenantID, rolloutID, status, msg)
	if err != nil {
		return fmt.Errorf("creating rollout history: %w", err)
	}

	return tx.Commit(ctx)
}

// ListRolloutHistory returns the status transition history for a rollout.
func (r *Repo) ListRolloutHistory(ctx context.Context, tenantID, rolloutID string) ([]RolloutHistoryRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, rollout_id, status, message, created_at
		 FROM rollout_history WHERE rollout_id = $1 AND tenant_id = $2 ORDER BY created_at`,
		rolloutID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing rollout history: %w", err)
	}
	defer rows.Close()

	var history []RolloutHistoryRow
	for rows.Next() {
		var rh RolloutHistoryRow
		if err := rows.Scan(&rh.ID, &rh.TenantID, &rh.RolloutID, &rh.Status, &rh.Message, &rh.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, rh)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return history, nil
}

// GetConfigIntentByID returns a config intent by its ID.
func (r *Repo) GetConfigIntentByID(ctx context.Context, tenantID, intentID string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE id = $1 AND tenant_id = $2`, intentID, tenantID)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("getting intent by id: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// --- Events (tenant-scoped) ---

// CreateEvent stores an audit event.
func (r *Repo) CreateEvent(ctx context.Context, tenantID, eventType string, payload any) error {
	payloadJSON, _ := json.Marshal(payload)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO events (tenant_id, event_type, payload) VALUES ($1, $2, $3)`,
		tenantID, eventType, payloadJSON)
	if err != nil {
		return fmt.Errorf("creating event: %w", err)
	}

	return tx.Commit(ctx)
}

// --- API Keys ---

// CreateAPIKey creates a new API key (stores hash, returns nothing about plaintext).
func (r *Repo) CreateAPIKey(ctx context.Context, tenantID, name, keyHash, keyPrefix string, permissions []string) (*APIKeyRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO api_keys (tenant_id, name, key_hash, key_prefix, permissions)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, tenant_id, name, key_prefix, permissions, created_at, expires_at`,
		tenantID, name, keyHash, keyPrefix, permissions)

	var ak APIKeyRow
	if err := row.Scan(&ak.ID, &ak.TenantID, &ak.Name, &ak.KeyPrefix, &ak.Permissions, &ak.CreatedAt, &ak.ExpiresAt); err != nil {
		return nil, fmt.Errorf("creating api key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ak, nil
}

// ListAPIKeys returns all API keys for a tenant (no secrets).
func (r *Repo) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKeyRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, key_prefix, permissions, created_at, expires_at
		 FROM api_keys WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyRow
	for rows.Next() {
		var ak APIKeyRow
		if err := rows.Scan(&ak.ID, &ak.TenantID, &ak.Name, &ak.KeyPrefix, &ak.Permissions, &ak.CreatedAt, &ak.ExpiresAt); err != nil {
			return nil, err
		}
		keys = append(keys, ak)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return keys, nil
}

// GetAPIKeyByHash looks up an API key by its hash (for authentication).
func (r *Repo) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRow, string, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, key_prefix, permissions, created_at, expires_at
		 FROM api_keys WHERE key_hash = $1`, keyHash)

	var ak APIKeyRow
	if err := row.Scan(&ak.ID, &ak.TenantID, &ak.Name, &ak.KeyPrefix, &ak.Permissions, &ak.CreatedAt, &ak.ExpiresAt); err != nil {
		return nil, "", fmt.Errorf("api key not found: %w", err)
	}
	return &ak, ak.TenantID, nil
}

// --- Agent Capabilities ---

// UpdateAgentCapabilities updates the capabilities for an agent.
func (r *Repo) UpdateAgentCapabilities(ctx context.Context, tenantID, agentName string, capabilities map[string]any) error {
	capJSON, _ := json.Marshal(capabilities)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE agents SET capabilities = $1, updated_at = now()
		 WHERE name = $2 AND tenant_id = $3`, capJSON, agentName, tenantID)
	if err != nil {
		return fmt.Errorf("updating capabilities: %w", err)
	}

	return tx.Commit(ctx)
}

// ListAgentsByCapability returns agents that have a given capability.
func (r *Repo) ListAgentsByCapability(ctx context.Context, tenantID, capability string) ([]AgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL AND (
			capabilities->'receivers' ? $2 OR
			capabilities->'exporters' ? $2 OR
			capabilities->'processors' ? $2
		 ) ORDER BY name`, tenantID, capability)
	if err != nil {
		return nil, fmt.Errorf("listing agents by capability: %w", err)
	}
	defer rows.Close()

	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// --- Agent Config History ---

// CreateAgentConfigHistory records a config change for an agent.
func (r *Repo) CreateAgentConfigHistory(ctx context.Context, tenantID, agentID, configYAML, cfgHash, source string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO agent_config_history (tenant_id, agent_id, config_yaml, config_hash, source)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, agentID, configYAML, cfgHash, source)
	if err != nil {
		return fmt.Errorf("creating config history: %w", err)
	}

	return tx.Commit(ctx)
}

// SoftDeleteAgent marks an agent as deregistered with a deleted_at timestamp.
func (r *Repo) SoftDeleteAgent(ctx context.Context, tenantID, agentID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE agents SET status = 'deregistered', deleted_at = now(), updated_at = now()
		 WHERE id = $1 AND tenant_id = $2`, agentID, tenantID)
	if err != nil {
		return fmt.Errorf("soft-deleting agent: %w", err)
	}
	return tx.Commit(ctx)
}

// UpdateAgentHealthScore updates the health score for an agent.
func (r *Repo) UpdateAgentHealthScore(ctx context.Context, tenantID, agentName string, score int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`UPDATE agents SET health_score = $1, updated_at = now()
		 WHERE name = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		score, agentName, tenantID)
	if err != nil {
		return fmt.Errorf("updating health score: %w", err)
	}
	return tx.Commit(ctx)
}

// ListAgentsByMinHealth returns agents with health_score >= minHealth.
func (r *Repo) ListAgentsByMinHealth(ctx context.Context, tenantID string, minHealth int) ([]AgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, status, last_heartbeat, effective_config, labels::text, capabilities::text, health_score, topology::text, deleted_at, created_at, updated_at
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL AND health_score >= $2 ORDER BY name`,
		tenantID, minHealth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// CreateWebhook creates a new webhook.
func (r *Repo) CreateWebhook(ctx context.Context, tenantID, name, url string, events []string) (*WebhookRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	row := tx.QueryRow(ctx,
		`INSERT INTO webhooks (tenant_id, name, url, events) VALUES ($1, $2, $3, $4)
		 RETURNING id, tenant_id, name, url, events, active, created_at`,
		tenantID, name, url, events)
	var wh WebhookRow
	if err := row.Scan(&wh.ID, &wh.TenantID, &wh.Name, &wh.URL, &wh.Events, &wh.Active, &wh.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating webhook: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &wh, nil
}

// ListWebhooks returns all webhooks for a tenant.
func (r *Repo) ListWebhooks(ctx context.Context, tenantID string) ([]WebhookRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, url, events, active, created_at
		 FROM webhooks WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var webhooks []WebhookRow
	for rows.Next() {
		var wh WebhookRow
		if err := rows.Scan(&wh.ID, &wh.TenantID, &wh.Name, &wh.URL, &wh.Events, &wh.Active, &wh.CreatedAt); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, wh)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return webhooks, nil
}

// DeleteWebhook removes a webhook.
func (r *Repo) DeleteWebhook(ctx context.Context, tenantID, webhookID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx,
		`DELETE FROM webhooks WHERE id = $1 AND tenant_id = $2`, webhookID, tenantID)
	if err != nil {
		return fmt.Errorf("deleting webhook: %w", err)
	}
	return tx.Commit(ctx)
}

// GetActiveWebhooksForEvent returns active webhooks that match an event type.
func (r *Repo) GetActiveWebhooksForEvent(ctx context.Context, tenantID, eventType string) ([]WebhookRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, url, events, active, created_at
		 FROM webhooks WHERE tenant_id = $1 AND active = true AND (events = '{}' OR $2 = ANY(events))`,
		tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var webhooks []WebhookRow
	for rows.Next() {
		var wh WebhookRow
		if err := rows.Scan(&wh.ID, &wh.TenantID, &wh.Name, &wh.URL, &wh.Events, &wh.Active, &wh.CreatedAt); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, wh)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return webhooks, nil
}

// GetCachedCompilation looks up a cached compilation by intent hash.
func (r *Repo) GetCachedCompilation(ctx context.Context, intentHash string) (string, bool, error) {
	var yaml string
	err := r.pool.QueryRow(ctx,
		`UPDATE config_cache SET hits = hits + 1 WHERE intent_hash = $1 RETURNING compiled_yaml`,
		intentHash).Scan(&yaml)
	if err != nil {
		return "", false, nil
	}
	return yaml, true, nil
}

// SetCachedCompilation stores a compilation result.
func (r *Repo) SetCachedCompilation(ctx context.Context, intentHash, compiledYAML string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO config_cache (intent_hash, compiled_yaml) VALUES ($1, $2) ON CONFLICT (intent_hash) DO NOTHING`,
		intentHash, compiledYAML)
	return err
}

// ListAgentConfigHistory returns the config history for an agent (most recent first, limit 50).
func (r *Repo) ListAgentConfigHistory(ctx context.Context, tenantID, agentID string) ([]AgentConfigHistoryRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, agent_id, config_yaml, config_hash, source, created_at
		 FROM agent_config_history WHERE agent_id = $1 AND tenant_id = $2
		 ORDER BY created_at DESC LIMIT 50`, agentID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing config history: %w", err)
	}
	defer rows.Close()

	var history []AgentConfigHistoryRow
	for rows.Next() {
		var h AgentConfigHistoryRow
		if err := rows.Scan(&h.ID, &h.TenantID, &h.AgentID, &h.ConfigYAML, &h.ConfigHash, &h.Source, &h.CreatedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return history, nil
}

// --- Phase 8: Tags, Topology, Scheduled Rollouts, Export/Import ---

// UpdateConfigIntentTags updates the tags on a specific config intent.
func (r *Repo) UpdateConfigIntentTags(ctx context.Context, tenantID, intentID string, tags []string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx,
		`UPDATE config_intents SET tags = $1
		 WHERE id = $2 AND tenant_id = $3
		 RETURNING id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at`,
		tags, intentID, tenantID)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("updating tags: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// ListConfigIntentsByTag returns config intents filtered by a tag.
func (r *Repo) ListConfigIntentsByTag(ctx context.Context, tenantID, tag string) ([]ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE tenant_id = $1 AND $2 = ANY(tags) ORDER BY name, version DESC`,
		tenantID, tag)
	if err != nil {
		return nil, fmt.Errorf("listing intents by tag: %w", err)
	}
	defer rows.Close()

	var intents []ConfigIntentRow
	for rows.Next() {
		var ci ConfigIntentRow
		if err := rows.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
			return nil, err
		}
		intents = append(intents, ci)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return intents, nil
}

// GetAllConfigIntentVersions returns all versions of a config intent by name.
func (r *Repo) GetAllConfigIntentVersions(ctx context.Context, tenantID, name string) ([]ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at
		 FROM config_intents WHERE tenant_id = $1 AND name = $2 ORDER BY version ASC`,
		tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("listing intent versions: %w", err)
	}
	defer rows.Close()

	var intents []ConfigIntentRow
	for rows.Next() {
		var ci ConfigIntentRow
		if err := rows.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
			return nil, err
		}
		intents = append(intents, ci)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return intents, nil
}

// UpdateAgentTopology updates the topology metadata for an agent.
func (r *Repo) UpdateAgentTopology(ctx context.Context, tenantID, agentName string, topology map[string]string) error {
	topoJSON, _ := json.Marshal(topology)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE agents SET topology = $1, updated_at = now()
		 WHERE name = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		topoJSON, agentName, tenantID)
	if err != nil {
		return fmt.Errorf("updating topology: %w", err)
	}

	return tx.Commit(ctx)
}

// ListAgentsWithTopology returns all non-deleted agents with their topology for a tenant.
func (r *Repo) ListAgentsWithTopology(ctx context.Context, tenantID string) ([]AgentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT `+agentColumns+`
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing agents with topology: %w", err)
	}
	defer rows.Close()

	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}

// ListScheduledRolloutsDue returns rollouts with status=scheduled whose scheduled_at is in the past.
func (r *Repo) ListScheduledRolloutsDue(ctx context.Context) ([]RolloutRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, fleet_id, intent_id, status, target_count, completed_count, strategy::text, scheduled_at, created_at, updated_at
		 FROM rollouts WHERE status = 'scheduled' AND scheduled_at <= now()
		 ORDER BY scheduled_at`)
	if err != nil {
		return nil, fmt.Errorf("listing scheduled rollouts: %w", err)
	}
	defer rows.Close()

	var rollouts []RolloutRow
	for rows.Next() {
		var ro RolloutRow
		if err := rows.Scan(&ro.ID, &ro.TenantID, &ro.FleetID, &ro.IntentID, &ro.Status,
			&ro.TargetCount, &ro.CompletedCount, &ro.Strategy, &ro.ScheduledAt, &ro.CreatedAt, &ro.UpdatedAt); err != nil {
			return nil, err
		}
		rollouts = append(rollouts, ro)
	}
	return rollouts, nil
}

// GetTenantRateLimit returns the rate limit for a tenant.
func (r *Repo) GetTenantRateLimit(ctx context.Context, tenantID string) (int, error) {
	var rateLimit int
	err := r.pool.QueryRow(ctx,
		`SELECT rate_limit FROM tenants WHERE id = $1`, tenantID).Scan(&rateLimit)
	if err != nil {
		return 0, fmt.Errorf("getting tenant rate limit: %w", err)
	}
	return rateLimit, nil
}

// CreateEventWithRequestID stores an audit event with a request ID for correlation.
func (r *Repo) CreateEventWithRequestID(ctx context.Context, tenantID, eventType string, payload any, requestID string) error {
	payloadJSON, _ := json.Marshal(payload)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO events (tenant_id, event_type, payload, request_id) VALUES ($1, $2, $3, $4)`,
		tenantID, eventType, payloadJSON, requestID)
	if err != nil {
		return fmt.Errorf("creating event with request_id: %w", err)
	}

	return tx.Commit(ctx)
}

// CreateConfigIntentWithTags stores a new config intent with tags.
func (r *Repo) CreateConfigIntentWithTags(ctx context.Context, tenantID, name, intentJSON string, compiledYAML *string, tags []string) (*ConfigIntentRow, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	var nextVersion int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM config_intents WHERE tenant_id = $1 AND name = $2`,
		tenantID, name).Scan(&nextVersion)
	if err != nil {
		return nil, fmt.Errorf("getting next version: %w", err)
	}

	row := tx.QueryRow(ctx,
		`INSERT INTO config_intents (tenant_id, name, version, intent_json, compiled_yaml, tags)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, tenant_id, name, version, intent_json::text, compiled_yaml, promoted, tags, created_at`,
		tenantID, name, nextVersion, intentJSON, compiledYAML, tags)

	var ci ConfigIntentRow
	if err := row.Scan(&ci.ID, &ci.TenantID, &ci.Name, &ci.Version, &ci.IntentJSON, &ci.CompiledYAML, &ci.Promoted, &ci.Tags, &ci.CreatedAt); err != nil {
		return nil, fmt.Errorf("creating config intent with tags: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &ci, nil
}

// MatchAgentsBySelectorAndTopology returns agents matching both label selector and topology fields.
func (r *Repo) MatchAgentsBySelectorAndTopology(ctx context.Context, tenantID string, selector map[string]string, topology map[string]string) ([]AgentRow, error) {
	selectorJSON, _ := json.Marshal(selector)
	topoJSON, _ := json.Marshal(topology)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if err := setTenantContext(ctx, tx, tenantID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT `+agentColumns+`
		 FROM agents WHERE tenant_id = $1 AND labels @> $2 AND topology @> $3 AND deleted_at IS NULL ORDER BY name`,
		tenantID, selectorJSON, topoJSON)
	if err != nil {
		return nil, fmt.Errorf("matching agents by selector and topology: %w", err)
	}
	defer rows.Close()

	var agents []AgentRow
	for rows.Next() {
		a, err := scanAgentRows(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *a)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return agents, nil
}
