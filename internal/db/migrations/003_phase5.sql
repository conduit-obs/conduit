-- Conduit Phase 5: Config Acknowledgment, Promotion, Rollout History, Tenant Management

-- Per-agent rollout tracking (config acknowledgment protocol)
CREATE TABLE IF NOT EXISTS rollout_agents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rollout_id  UUID NOT NULL REFERENCES rollouts(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending, acknowledged, rejected
    config_hash TEXT,                             -- hash of pushed config for ack matching
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(rollout_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_rollout_agents_rollout ON rollout_agents(rollout_id);
CREATE INDEX IF NOT EXISTS idx_rollout_agents_tenant ON rollout_agents(tenant_id);

ALTER TABLE rollout_agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE rollout_agents FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_rollout_agents ON rollout_agents;
CREATE POLICY tenant_isolation_rollout_agents ON rollout_agents
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Rollout history (timestamped status transitions)
CREATE TABLE IF NOT EXISTS rollout_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    rollout_id  UUID NOT NULL REFERENCES rollouts(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    message     TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rollout_history_rollout ON rollout_history(rollout_id);

ALTER TABLE rollout_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE rollout_history FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_rollout_history ON rollout_history;
CREATE POLICY tenant_isolation_rollout_history ON rollout_history
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Add promoted flag to config_intents
ALTER TABLE config_intents ADD COLUMN IF NOT EXISTS promoted BOOLEAN NOT NULL DEFAULT false;

-- Add status filter index for rollouts
CREATE INDEX IF NOT EXISTS idx_rollouts_status ON rollouts(tenant_id, status);
