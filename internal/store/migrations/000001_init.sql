-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE provider_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_code VARCHAR(255) NOT NULL,
    webhook_secret TEXT,
    mcp_url TEXT,
    mcp_auth_enc TEXT,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, provider_code)
);

CREATE TABLE account_identities (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_code VARCHAR(255) NOT NULL,
    external_user_id VARCHAR(255) NOT NULL,
    PRIMARY KEY (account_id, provider_code),
    UNIQUE (provider_code, external_user_id)
);

CREATE TABLE watched_containers (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider_code VARCHAR(255) NOT NULL,
    external_container_id VARCHAR(255) NOT NULL,
    PRIMARY KEY (account_id, provider_code, external_container_id),
    UNIQUE (provider_code, external_container_id)
);

CREATE TABLE runtimes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    code VARCHAR(255) NOT NULL,
    label VARCHAR(255) NOT NULL,
    token_lookup VARCHAR(255) NOT NULL UNIQUE,
    last_seen_at TIMESTAMPTZ,
    status VARCHAR(50) NOT NULL DEFAULT 'offline',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, code)
);

CREATE TABLE profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    body TEXT NOT NULL,
    backend_hint VARCHAR(255),
    harness VARCHAR(255),
    workflow_config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, name)
);

CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    runtime_id UUID NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
    external_task_id VARCHAR(255) NOT NULL,
    external_event_id VARCHAR(255) NOT NULL,
    provider_code VARCHAR(255) NOT NULL,
    external_actor_id VARCHAR(255),
    writeback_status VARCHAR(50) NOT NULL DEFAULT 'pending',
    writeback_attempts INT NOT NULL DEFAULT 0,
    writeback_last_error TEXT,
    task_title VARCHAR(255) NOT NULL,
    task_description TEXT NOT NULL,
    profile_body_snapshot TEXT,
    workflow VARCHAR(255),
    instructions TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    claim_lease_until TIMESTAMPTZ,
    ttl_expires_at TIMESTAMPTZ NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    result_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (provider_code, external_event_id)
);

-- Index for supporting the claim query:
-- WHERE runtime_id = $1 AND status = 'queued' ORDER BY created_at
CREATE INDEX idx_jobs_claim ON jobs (runtime_id, status, created_at);

-- Partial unique index as a hard backstop for one-job-per-runtime invariant:
CREATE UNIQUE INDEX idx_jobs_one_active_per_runtime ON jobs (runtime_id)
WHERE status IN ('claimed', 'running');

-- Index for supporting the writeback query:
CREATE INDEX idx_jobs_writeback ON jobs (writeback_status)
WHERE writeback_status = 'pending';

-- Index for cascading deletes and account job queries:
CREATE INDEX idx_jobs_account_id ON jobs (account_id);

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_account_id;
DROP INDEX IF EXISTS idx_jobs_writeback;
DROP INDEX IF EXISTS idx_jobs_one_active_per_runtime;
DROP INDEX IF EXISTS idx_jobs_claim;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS runtimes;
DROP TABLE IF EXISTS watched_containers;
DROP TABLE IF EXISTS account_identities;
DROP TABLE IF EXISTS provider_connections;
DROP TABLE IF EXISTS accounts;
