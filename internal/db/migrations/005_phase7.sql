-- Conduit Phase 7: Config Cache, Health Scoring, Webhooks, Soft-Delete

-- Config compilation cache (content-addressable)
CREATE TABLE IF NOT EXISTS config_cache (
    intent_hash TEXT PRIMARY KEY,          -- SHA256 of intent JSON
    compiled_yaml TEXT NOT NULL,
    hits        BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent health score
ALTER TABLE agents ADD COLUMN IF NOT EXISTS health_score INTEGER NOT NULL DEFAULT 100;
ALTER TABLE agents ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Webhooks table
CREATE TABLE IF NOT EXISTS webhooks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    url         TEXT NOT NULL,
    events      TEXT[] NOT NULL DEFAULT '{}',  -- event types to subscribe to, empty = all
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_webhooks_tenant ON webhooks(tenant_id);

ALTER TABLE webhooks ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhooks FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_webhooks ON webhooks;
CREATE POLICY tenant_isolation_webhooks ON webhooks
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Add base_intent field to config_intents for inheritance
ALTER TABLE config_intents ADD COLUMN IF NOT EXISTS base_intent TEXT;
