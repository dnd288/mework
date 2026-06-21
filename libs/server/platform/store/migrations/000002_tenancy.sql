-- +goose Up
-- Tenancy: introduce the tenant isolation boundary. Every tenant-scoped resource
-- table gains an indexed, NOT NULL tenant_id keyed to a tenant. Existing rows are
-- backfilled to a single default tenant so the migration is safe on live data.

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A single default tenant that every pre-existing row maps to. Its id is fixed so
-- scoped tables can default their tenant_id to it until callers thread an explicit
-- tenant through every write (a later unit).
INSERT INTO tenants (id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'default');

-- provider_connections
ALTER TABLE provider_connections ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE provider_connections SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE provider_connections ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_provider_connections_tenant_id ON provider_connections (tenant_id);

-- account_identities
ALTER TABLE account_identities ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE account_identities SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE account_identities ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_account_identities_tenant_id ON account_identities (tenant_id);

-- watched_containers
ALTER TABLE watched_containers ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE watched_containers SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE watched_containers ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_watched_containers_tenant_id ON watched_containers (tenant_id);

-- runtimes
ALTER TABLE runtimes ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE runtimes SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE runtimes ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_runtimes_tenant_id ON runtimes (tenant_id);

-- profiles
ALTER TABLE profiles ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE profiles SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE profiles ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_profiles_tenant_id ON profiles (tenant_id);

-- jobs
ALTER TABLE jobs ADD COLUMN tenant_id UUID REFERENCES tenants(id) ON DELETE CASCADE
    DEFAULT '00000000-0000-0000-0000-000000000001';
UPDATE jobs SET tenant_id = '00000000-0000-0000-0000-000000000001';
ALTER TABLE jobs ALTER COLUMN tenant_id SET NOT NULL;
CREATE INDEX idx_jobs_tenant_id ON jobs (tenant_id);

-- registration_tokens: one-time(-ish) enrollment tokens bound to an owning tenant.
-- Only the HMAC lookup of the raw token is stored (never the plaintext), so the
-- tenant binding is tamper-resistant: a runner enrolled with the token inherits the
-- token's tenant by construction and can never be steered into another tenant.
CREATE TABLE registration_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    token_lookup VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_registration_tokens_tenant_id ON registration_tokens (tenant_id);

-- +goose Down
DROP TABLE IF EXISTS registration_tokens;

DROP INDEX IF EXISTS idx_jobs_tenant_id;
ALTER TABLE jobs DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_profiles_tenant_id;
ALTER TABLE profiles DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_runtimes_tenant_id;
ALTER TABLE runtimes DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_watched_containers_tenant_id;
ALTER TABLE watched_containers DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_account_identities_tenant_id;
ALTER TABLE account_identities DROP COLUMN IF EXISTS tenant_id;

DROP INDEX IF EXISTS idx_provider_connections_tenant_id;
ALTER TABLE provider_connections DROP COLUMN IF EXISTS tenant_id;

DROP TABLE IF EXISTS tenants;
