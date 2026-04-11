-- Conduit: Initiative 1-8 Gap Closure Migration

-- Resource quotas per tenant
CREATE TABLE IF NOT EXISTS quotas (
    tenant_id   UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    max_agents  INTEGER NOT NULL DEFAULT 100,
    max_fleets  INTEGER NOT NULL DEFAULT 20,
    max_configs INTEGER NOT NULL DEFAULT 50,
    max_api_keys INTEGER NOT NULL DEFAULT 10,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Environments (dev/staging/prod)
CREATE TABLE IF NOT EXISTS environments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    config_overrides JSONB NOT NULL DEFAULT '{}',
    promoted_from   UUID REFERENCES environments(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_environments_tenant ON environments(tenant_id);
ALTER TABLE environments ENABLE ROW LEVEL SECURITY;
ALTER TABLE environments FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_environments ON environments;
CREATE POLICY tenant_isolation_environments ON environments
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Rollout snapshots for rollback
CREATE TABLE IF NOT EXISTS rollout_snapshots (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rollout_id          UUID NOT NULL,
    agent_id            UUID NOT NULL,
    previous_config_yaml TEXT NOT NULL DEFAULT '',
    previous_config_hash TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rollout_snapshots_rollout ON rollout_snapshots(rollout_id);

-- Rollout approvals
CREATE TABLE IF NOT EXISTS rollout_approvals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rollout_id  UUID NOT NULL,
    approver    TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    comment     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rollout_approvals_rollout ON rollout_approvals(rollout_id);

-- Custom roles
CREATE TABLE IF NOT EXISTS custom_roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
CREATE INDEX IF NOT EXISTS idx_custom_roles_tenant ON custom_roles(tenant_id);
ALTER TABLE custom_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE custom_roles FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_custom_roles ON custom_roles;
CREATE POLICY tenant_isolation_custom_roles ON custom_roles
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Alert rules
CREATE TABLE IF NOT EXISTS alert_rules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    condition   TEXT NOT NULL,
    threshold   DOUBLE PRECISION NOT NULL DEFAULT 0,
    channels    TEXT[] NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
ALTER TABLE alert_rules ENABLE ROW LEVEL SECURITY;
ALTER TABLE alert_rules FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_alert_rules ON alert_rules;
CREATE POLICY tenant_isolation_alert_rules ON alert_rules
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Usage metering snapshots
CREATE TABLE IF NOT EXISTS usage_snapshots (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    snapshot_date     DATE NOT NULL,
    agent_count       INTEGER NOT NULL DEFAULT 0,
    api_calls         BIGINT NOT NULL DEFAULT 0,
    config_deployments INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, snapshot_date)
);

-- Entitlements per tenant
CREATE TABLE IF NOT EXISTS entitlements (
    tenant_id           UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    tier                TEXT NOT NULL DEFAULT 'free',
    max_agents          INTEGER NOT NULL DEFAULT 10,
    max_fleets          INTEGER NOT NULL DEFAULT 5,
    max_api_calls_per_min INTEGER NOT NULL DEFAULT 60,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Retention policies
CREATE TABLE IF NOT EXISTS retention_policies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    resource_type   TEXT NOT NULL,
    retention_days  INTEGER NOT NULL DEFAULT 90,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, resource_type)
);

-- Add drift_check_interval to fleets
ALTER TABLE fleets ADD COLUMN IF NOT EXISTS drift_check_interval INTEGER NOT NULL DEFAULT 15;

-- Add connection_state to agents
ALTER TABLE agents ADD COLUMN IF NOT EXISTS connection_state TEXT NOT NULL DEFAULT 'disconnected';

-- Add actor column to events for audit queries
ALTER TABLE events ADD COLUMN IF NOT EXISTS actor TEXT;
ALTER TABLE events ADD COLUMN IF NOT EXISTS resource_type TEXT;
ALTER TABLE events ADD COLUMN IF NOT EXISTS resource_id TEXT;
CREATE INDEX IF NOT EXISTS idx_events_actor ON events(actor) WHERE actor IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at);
