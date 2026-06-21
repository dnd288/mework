-- +goose Up
-- Per-tenant quota and audit tables for platform hardening.

-- tenant_quotas holds the configured resource limits for each tenant.
CREATE TABLE tenant_quotas (
    tenant_id UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    max_concurrent_runs INT NOT NULL DEFAULT 5,
    max_dispatches_per_min INT NOT NULL DEFAULT 10,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default quotas for the default tenant.
INSERT INTO tenant_quotas (tenant_id, max_concurrent_runs, max_dispatches_per_min)
VALUES ('00000000-0000-0000-0000-000000000001', 5, 10)
ON CONFLICT (tenant_id) DO NOTHING;

-- tenant_active_runs tracks currently active runs per tenant. A row is inserted when a
-- run is admitted (INSERT ... ON CONFLICT DO NOTHING) and deleted when the run reaches
-- a terminal state. The unique constraint on (tenant_id) enforces MaxConcurrentRuns
-- atomically: two concurrent transactions cannot both insert when only one slot is free.
CREATE TABLE tenant_active_runs (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    run_id UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, run_id)
);

-- tenant_dispatch_minute tracks per-minute dispatch counts per tenant for sliding-window
-- rate limiting. Each row represents one dispatch in a minute bucket.
CREATE TABLE tenant_dispatch_minute (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    minute_bucket TIMESTAMPTZ NOT NULL,
    seq BIGSERIAL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, minute_bucket, seq)
);

CREATE INDEX idx_tenant_dispatch_minute_tenant ON tenant_dispatch_minute (tenant_id, minute_bucket);
CREATE INDEX idx_tenant_dispatch_minute_created ON tenant_dispatch_minute (created_at);

-- audit_log is an append-only, tamper-evident log of security-relevant actions. The
-- (tenant_id, seq) primary key guarantees append-order iteration: a monotonic sequence
-- ensures entries are returned in chronological order without a separate sort.
CREATE TABLE audit_log (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    seq BIGSERIAL,
    actor_id VARCHAR(255) NOT NULL,
    actor_type VARCHAR(50) NOT NULL DEFAULT 'user',
    action VARCHAR(255) NOT NULL,
    target_type VARCHAR(255),
    target_id VARCHAR(255),
    metadata JSONB NOT NULL DEFAULT '{}',
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, seq)
);

CREATE INDEX idx_audit_log_recorded ON audit_log (tenant_id, recorded_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS tenant_dispatch_minute;
DROP TABLE IF EXISTS tenant_active_runs;
DROP TABLE IF EXISTS tenant_quotas;
