-- Conduit Phase 8: Rate Limiting, Tags, Scheduled Rollouts, Topology, Export/Import, Request Tracing

-- Rate limiting: add rate_limit column to tenants (requests per second, 0 = unlimited)
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS rate_limit INTEGER NOT NULL DEFAULT 0;

-- Config intent tags
ALTER TABLE config_intents ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- Scheduled rollouts: add scheduled_at to rollouts
ALTER TABLE rollouts ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ;

-- Agent topology metadata (region/zone/cluster)
ALTER TABLE agents ADD COLUMN IF NOT EXISTS topology JSONB NOT NULL DEFAULT '{}';

-- Add request_id to events for correlation
ALTER TABLE events ADD COLUMN IF NOT EXISTS request_id TEXT;
CREATE INDEX IF NOT EXISTS idx_events_request_id ON events(request_id) WHERE request_id IS NOT NULL;
