-- +goose Up
-- Notifications & artifacts: outbound notification targets and delivery tracking.

CREATE TABLE notification_targets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    signing_secret TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id)
);
CREATE INDEX idx_notification_targets_tenant ON notification_targets (tenant_id);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    run_id VARCHAR(255) NOT NULL,
    event_kind VARCHAR(50) NOT NULL,
    target_url TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 4,
    last_status_code INT,
    last_error TEXT,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);
CREATE INDEX idx_notification_deliveries_next_retry ON notification_deliveries (next_retry_at) WHERE status = 'pending';
CREATE INDEX idx_notification_deliveries_tenant ON notification_deliveries (tenant_id);
CREATE INDEX idx_notification_deliveries_run ON notification_deliveries (run_id);

-- +goose Down
DROP TABLE IF EXISTS notification_deliveries;
DROP TABLE IF EXISTS notification_targets;
