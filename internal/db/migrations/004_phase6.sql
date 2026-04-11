-- Conduit Phase 6: Capabilities, API Keys, Config History, Canary Strategy, Templating

-- Add capabilities column to agents
ALTER TABLE agents ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '{}';

-- Add variables column to fleets (for intent templating)
ALTER TABLE fleets ADD COLUMN IF NOT EXISTS variables JSONB NOT NULL DEFAULT '{}';

-- Add strategy column to rollouts (canary/all-at-once)
ALTER TABLE rollouts ADD COLUMN IF NOT EXISTS strategy JSONB NOT NULL DEFAULT '{"type":"all-at-once"}';

-- Add phase column to rollout_agents (canary/remainder)
ALTER TABLE rollout_agents ADD COLUMN IF NOT EXISTS phase TEXT NOT NULL DEFAULT 'all';

-- API keys table (alternative auth to JWT)
CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    key_hash    TEXT NOT NULL,      -- SHA256 hash of the key
    key_prefix  TEXT NOT NULL,      -- first 8 chars for identification
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ,
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_api_keys ON api_keys;
CREATE POLICY tenant_isolation_api_keys ON api_keys
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Agent config history table
CREATE TABLE IF NOT EXISTS agent_config_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    config_yaml TEXT NOT NULL,
    config_hash TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'rollout',  -- rollout, manual, etc.
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_config_history_agent ON agent_config_history(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_config_history_tenant ON agent_config_history(tenant_id);

ALTER TABLE agent_config_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE agent_config_history FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_agent_config_history ON agent_config_history;
CREATE POLICY tenant_isolation_agent_config_history ON agent_config_history
    USING (tenant_id::text = current_setting('app.tenant_id', true));
