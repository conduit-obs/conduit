-- Conduit: Auth0 integration

ALTER TABLE users ADD COLUMN IF NOT EXISTS auth0_sub TEXT;
CREATE INDEX IF NOT EXISTS idx_users_auth0_sub ON users(auth0_sub) WHERE auth0_sub IS NOT NULL;
