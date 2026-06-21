-- +goose Up
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    topic VARCHAR(255) NOT NULL,
    content_type VARCHAR(255) NOT NULL DEFAULT '',
    payload BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivery_lease_until TIMESTAMPTZ,
    acked_at TIMESTAMPTZ,
    subscriber_id VARCHAR(255) NOT NULL DEFAULT ''
);

CREATE INDEX idx_messages_topic ON messages (topic);
CREATE INDEX idx_messages_undelivered ON messages (topic, id) WHERE acked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_messages_undelivered;
DROP INDEX IF EXISTS idx_messages_topic;
DROP TABLE IF EXISTS messages;
