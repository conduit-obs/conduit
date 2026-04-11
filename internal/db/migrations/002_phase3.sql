-- Conduit Phase 3: Fleets, Rollouts, and Config Push

-- Fleets table (groups of agents by label selectors)
CREATE TABLE IF NOT EXISTS fleets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    selector    JSONB NOT NULL DEFAULT '{}',  -- label key/value pairs to match agents
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_fleets_tenant ON fleets(tenant_id);

ALTER TABLE fleets ENABLE ROW LEVEL SECURITY;
ALTER TABLE fleets FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_fleets ON fleets;
CREATE POLICY tenant_isolation_fleets ON fleets
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Rollouts table (config intent applied to a fleet)
CREATE TABLE IF NOT EXISTS rollouts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    fleet_id        UUID NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
    intent_id       UUID NOT NULL REFERENCES config_intents(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, in_progress, completed, failed
    target_count    INTEGER NOT NULL DEFAULT 0,
    completed_count INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rollouts_tenant ON rollouts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_rollouts_fleet ON rollouts(fleet_id);

ALTER TABLE rollouts ENABLE ROW LEVEL SECURITY;
ALTER TABLE rollouts FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_rollouts ON rollouts;
CREATE POLICY tenant_isolation_rollouts ON rollouts
    USING (tenant_id::text = current_setting('app.tenant_id', true));
