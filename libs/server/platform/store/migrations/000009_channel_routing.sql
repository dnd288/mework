-- +goose Up
ALTER TABLE runtimes ADD COLUMN specs TEXT[];

CREATE TABLE channel_sessions (
    channel_key   TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    provider_code TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    runner_id     TEXT NOT NULL,
    spec          TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active'
        CONSTRAINT channel_sessions_status_check CHECK (status IN ('active', 'draining', 'closed')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at     TIMESTAMPTZ
);

CREATE INDEX idx_channel_sessions_runner_id ON channel_sessions(runner_id);
CREATE INDEX idx_channel_sessions_provider_resource ON channel_sessions(provider_code, resource_id);

-- +goose Down
DROP TABLE IF EXISTS channel_sessions;
ALTER TABLE runtimes DROP COLUMN IF EXISTS specs;
