-- +goose Up
ALTER TABLE registration_tokens
    ADD COLUMN account_id UUID,
    ADD COLUMN expires_at TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '15 minutes',
    ADD COLUMN consumed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE registration_tokens
    DROP COLUMN IF EXISTS consumed_at,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS account_id;
