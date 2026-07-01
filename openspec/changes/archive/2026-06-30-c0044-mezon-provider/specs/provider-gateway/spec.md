# Provider Gateway Specification

## Purpose

Define how `mework-server` stays **provider-agnostic**: a registry of provider
adapters and per-account provider connections lets the same pipeline serve any
issue/task tracker (Mello today; Jira, Linear, GitHub Issues by design) and chat
platform (Mezon) without changing the daemon or the job queue. This capability
owns the adapter abstraction (`internal/server/provider`) and connection storage
(`internal/server/connection`).

## ADDED Requirements

### Requirement: Chat-type provider support

The provider adapter registry SHALL support two categories of providers: **webhook-type** (Mello, GitHub, Jira) where events arrive via incoming HTTP webhooks, and **chat-type** (Mezon) where events arrive via the bot's outbound WebSocket connection. Adapters for chat-type providers MAY return empty or no-op implementations for webhook-specific methods (`VerifyWebhook`, `WebhookHeaders`, `ExtractContainerID`).

#### Scenario: Register a chat-type provider

- **WHEN** the Mezon provider adapter is registered
- **THEN** it SHALL be usable via the channel router for message dispatch, even though `VerifyWebhook` and `WebhookHeaders` return no-op values

#### Scenario: Chat provider does not use webhook endpoint

- **WHEN** a request targets `POST /webhooks/mezon`
- **THEN** the system MAY reject it or return 404 (Mezon is not a webhook source)

### Requirement: Connection config supports chat credentials

The provider connection SHALL support storing chat-type credentials (appID, apiKey) in its `config` JSONB column alongside the existing token-based credentials. The `mcp_auth_enc` column SHALL hold the sealed apiKey. A new `config.mezon_app_id` field SHALL hold the app ID as plaintext (it is not a secret — only the apiKey is sealed).

#### Scenario: Store Mezon bot credentials

- **WHEN** an authenticated user creates a Mezon connection with `app_id = "myapp"` and `api_key = "secret123"`
- **THEN** the apiKey is sealed into `mcp_auth_enc` and `config` is set to `{"mezon_app_id": "myapp"}`

#### Scenario: Retrieve Mezon bot credentials for bot startup

- **WHEN** the Mezon bot service starts
- **THEN** it retrieves and unseals the Mezon connection, obtaining both appID and apiKey

### Requirement: Provider connection with optional base URL

The provider connection SHALL support an optional `base_url` field in its config for self-hosted or custom Mezon installations. When empty, the system SHALL use the default Mezon API/WebSocket base URL.

#### Scenario: Default Mezon URL used when empty

- **WHEN** a Mezon connection has no `base_url` in config
- **THEN** the system uses the default `https://api.mezon.vn` for REST and `wss://gateway.mezon.vn` for WebSocket

#### Scenario: Custom base URL respected

- **WHEN** a Mezon connection has `base_url: "https://self-hosted.mezon.example"` in config
- **THEN** the system uses that URL for all Mezon API and WebSocket connections
