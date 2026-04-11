-- Conduit Phase 10: Pipeline Templates and Policy Packs

-- Pipeline templates table
CREATE TABLE IF NOT EXISTS pipeline_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    version     TEXT NOT NULL DEFAULT '1.0.0',
    description TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL DEFAULT '',
    parameters  JSONB NOT NULL DEFAULT '[]',
    intent_json JSONB NOT NULL DEFAULT '{}',
    deprecated  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name, version)
);

CREATE INDEX IF NOT EXISTS idx_pipeline_templates_tenant ON pipeline_templates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_pipeline_templates_name ON pipeline_templates(tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_pipeline_templates_category ON pipeline_templates(tenant_id, category);

ALTER TABLE pipeline_templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE pipeline_templates FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_pipeline_templates ON pipeline_templates;
CREATE POLICY tenant_isolation_pipeline_templates ON pipeline_templates
    USING (tenant_id::text = current_setting('app.tenant_id', true));

-- Policy packs table
CREATE TABLE IF NOT EXISTS policy_packs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    version     TEXT NOT NULL DEFAULT '1.0.0',
    description TEXT NOT NULL DEFAULT '',
    pack_json   JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name, version)
);

CREATE INDEX IF NOT EXISTS idx_policy_packs_tenant ON policy_packs(tenant_id);

ALTER TABLE policy_packs ENABLE ROW LEVEL SECURITY;
ALTER TABLE policy_packs FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_policy_packs ON policy_packs;
CREATE POLICY tenant_isolation_policy_packs ON policy_packs
    USING (tenant_id::text = current_setting('app.tenant_id', true));
