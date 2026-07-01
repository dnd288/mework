-- +goose Up
-- +goose StatementBegin
CREATE TABLE mezon_bots (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id   UUID NOT NULL,
    name         VARCHAR(255) NOT NULL DEFAULT '',
    app_id       VARCHAR(255) NOT NULL,
    api_key_enc  TEXT NOT NULL,                                             -- AES-256-GCM sealed
    base_url     VARCHAR(255) NOT NULL DEFAULT 'https://api.mezon.vn',
    status       VARCHAR(20) NOT NULL DEFAULT 'active'
        CONSTRAINT mezon_bots_status_check CHECK (status IN ('active', 'inactive')),
    plan         VARCHAR(50) NOT NULL DEFAULT 'starter'
        CONSTRAINT mezon_bots_plan_check CHECK (plan IN ('starter', 'pro', 'enterprise')),
    workspace_id VARCHAR(255) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_mezon_bots_tenant_app ON mezon_bots (tenant_id, account_id, app_id);
CREATE INDEX idx_mezon_bots_tenant_id ON mezon_bots (tenant_id);
CREATE INDEX idx_mezon_bots_account_id ON mezon_bots (account_id);
CREATE INDEX idx_mezon_bots_status ON mezon_bots (status);

-- Auto-update updated_at on row modification.
CREATE OR REPLACE FUNCTION update_mezon_bots_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_mezon_bots_updated_at
    BEFORE UPDATE ON mezon_bots
    FOR EACH ROW EXECUTE FUNCTION update_mezon_bots_updated_at();
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS trg_mezon_bots_updated_at ON mezon_bots;
DROP FUNCTION IF EXISTS update_mezon_bots_updated_at;
DROP TABLE IF EXISTS mezon_bots;
