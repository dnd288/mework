-- +goose Up
CREATE TABLE schedules (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('cron', 'interval', 'at')),
    cron        TEXT,
    every       TEXT,
    at          TEXT,
    tz          TEXT NOT NULL DEFAULT 'UTC',
    agent       TEXT NOT NULL,
    target      TEXT NOT NULL,
    grant_data  BYTEA,
    missed      TEXT NOT NULL DEFAULT 'skip' CHECK (missed IN ('skip', 'catch_up')),
    state       TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'paused', 'canceled')),
    next_fire   TIMESTAMPTZ,
    last_fire   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_schedules_tenant ON schedules (tenant_id);
CREATE INDEX idx_schedules_next_fire ON schedules (next_fire) WHERE state = 'active';

-- +goose Down
DROP TABLE IF EXISTS schedules;
