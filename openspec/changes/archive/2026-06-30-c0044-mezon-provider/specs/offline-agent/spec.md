# Offline Agent

## Purpose

Defines the capability to run a self-contained, zero-infrastructure local agent
that operates without a hub server, Postgres, or any network-accessible service.
All state is in-memory or on the local filesystem.

## ADDED Requirements

### Requirement: Offline agent can host a Mezon bot listener

The offline-mode agent SHALL optionally start a Mezon bot listener as an additional input channel alongside the existing Unix socket listener. The Mezon bot SHALL be started only when Mezon credentials are configured in the workspace's `mework.yml` or via environment variables (`MEZON_APP_ID`, `MEZON_API_KEY`). The offline agent SHALL NOT fail if Mezon credentials are absent — it simply does not start the bot.

#### Scenario: Start offline agent with Mezon credentials

- **WHEN** user runs `mework start --workspace /tmp/ws --offline` and `mework.yml` contains `mezon.app_id: "myapp"` and `mezon.api_key: "secret123"`
- **THEN** the offline agent starts both the Unix socket listener and the Mezon bot WebSocket listener
- **THEN** messages from both sources are accepted and dispatched to the sandbox

#### Scenario: Start offline agent without Mezon credentials

- **WHEN** user runs `mework start --workspace /tmp/ws --offline` with no Mezon credentials
- **THEN** the offline agent starts with only the Unix socket listener (existing behavior)
- **THEN** no Mezon connection is attempted

### Requirement: Mezon messages in offline mode use policy enforcement

Messages arriving from the Mezon bot in offline mode SHALL be processed through the same `policy.Policy` engine as Unix socket messages. The policy attributes SHALL include `"channel": "mezon:<channel_id>"` instead of `"channel": "local"`, enabling distinct policy rules for Mezon-sourced messages.

#### Scenario: Policy enforcement for Mezon message

- **WHEN** a Mezon message arrives at the offline agent with `channel_id = "ch_abc"`
- **THEN** the policy is evaluated with `"channel": "mezon:ch_abc"` in the attributes

#### Scenario: Policy blocks Mezon message

- **WHEN** a Mezon message is rejected by the policy engine
- **THEN** the Mezon bot sends an error reply to the originating channel (when configured)
- **THEN** the message is not dispatched to the sandbox

### Requirement: Offline Mezon bot sends replies to channel

When the offline agent processes a Mezon message, the agent's response SHALL be sent back to the originating Mezon channel via the bot's `SendMessage()` method. The bot client SHALL be started in a mode that supports both receiving and sending.

#### Scenario: Reply sent to originating channel

- **WHEN** the offline agent finishes processing a Mezon-sourced message
- **THEN** the agent's output is sent as a message to the same Mezon channel the request came from

### Requirement: Offline mode credentials from mework.yml

The workspace `mework.yml` SHALL support optional `mezon` configuration fields: `app_id`, `api_key`, and optionally `base_url`. These SHALL be read at agent startup and passed to the bot client. When present, `api_key` SHALL be held in memory only and never written to disk in plaintext (the user is responsible for securing `mework.yml`).

#### Scenario: Read Mezon config from mework.yml

- **WHEN** `mework.yml` contains `mezon: {app_id: "myapp", api_key: "secret123"}`
- **THEN** the offline agent starts the Mezon bot with those credentials

### Requirement: Offline Mezon bot does not depend on Postgres or hub

The offline mode Mezon bot SHALL operate without Postgres, a hub server, or any provider adapter registration. It SHALL use the bot client directly (not through the `provider.Registry` or channel router). Messages are dispatched directly to the local sandbox, and replies are sent directly via the bot client's `SendMessage()`.

#### Scenario: No database dependency

- **WHEN** the offline agent starts a Mezon bot with no `DATABASE_URL` set
- **THEN** the bot connects to Mezon and processes messages without any database access
