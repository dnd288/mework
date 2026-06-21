-- +goose Up
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE agent_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    version VARCHAR(255) NOT NULL,
    form VARCHAR(255) NOT NULL,
    payload BYTEA,
    reference TEXT NOT NULL DEFAULT '',
    checksum VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, version)
);

CREATE INDEX idx_agent_versions_agent_id ON agent_versions (agent_id);
CREATE INDEX idx_agent_versions_lookup ON agent_versions (agent_id, version);

CREATE TABLE agent_pointers (
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    channel VARCHAR(255) NOT NULL DEFAULT 'latest',
    version_id UUID NOT NULL REFERENCES agent_versions(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, channel)
);

CREATE INDEX idx_agent_pointers_lookup ON agent_pointers (agent_id, channel);

-- Data migration: copy existing profiles into definition-form agents so the
-- profile→agent migration is transparent.  Each profile row becomes an agent with
-- a single version 1.0.0.
INSERT INTO agents (id, name, description, created_at)
SELECT gen_random_uuid(), p.name, '', NOW()
FROM profiles p
WHERE NOT EXISTS (SELECT 1 FROM agents a WHERE a.name = p.name);

INSERT INTO agent_versions (id, agent_id, version, form, payload, checksum, created_at)
SELECT gen_random_uuid(), a.id, '1.0.0', 'definition', p.body::bytea, md5(p.body), NOW()
FROM profiles p
JOIN agents a ON a.name = p.name
WHERE NOT EXISTS (
    SELECT 1 FROM agent_versions av WHERE av.agent_id = a.id AND av.version = '1.0.0'
);

INSERT INTO agent_pointers (agent_id, channel, version_id)
SELECT a.id, 'latest', av.id
FROM agents a
JOIN agent_versions av ON av.agent_id = a.id AND av.version = '1.0.0'
WHERE NOT EXISTS (
    SELECT 1 FROM agent_pointers ap WHERE ap.agent_id = a.id AND ap.channel = 'latest'
);

-- +goose Down
DROP TABLE IF EXISTS agent_pointers;
DROP TABLE IF EXISTS agent_versions;
DROP TABLE IF EXISTS agents;
