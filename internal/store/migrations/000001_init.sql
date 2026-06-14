-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mello_user_id VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE account_boards (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    mello_board_id VARCHAR(255) NOT NULL UNIQUE,
    PRIMARY KEY (account_id, mello_board_id)
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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, name)
);

CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    runtime_id UUID NOT NULL REFERENCES runtimes(id) ON DELETE CASCADE,
    mello_ticket_id VARCHAR(255) NOT NULL,
    mello_comment_id VARCHAR(255) NOT NULL UNIQUE,
    profile_body_snapshot TEXT,
    instructions TEXT NOT NULL,
    ticket_title VARCHAR(255) NOT NULL,
    ticket_description TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'queued',
    claim_lease_until TIMESTAMPTZ,
    ttl_expires_at TIMESTAMPTZ NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    result_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

-- Index for supporting the claim query:
-- WHERE runtime_id = $1 AND status = 'queued' ORDER BY created_at
CREATE INDEX idx_jobs_claim ON jobs (runtime_id, status, created_at);

-- Partial unique index as a hard backstop for one-job-per-runtime invariant:
CREATE UNIQUE INDEX idx_jobs_one_active_per_runtime ON jobs (runtime_id)
WHERE status IN ('claimed', 'running');

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_one_active_per_runtime;
DROP INDEX IF EXISTS idx_jobs_claim;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS profiles;
DROP TABLE IF EXISTS runtimes;
DROP TABLE IF EXISTS account_boards;
DROP TABLE IF EXISTS accounts;
