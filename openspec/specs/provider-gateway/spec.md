# Provider Gateway Specification

## Purpose

Define how `mework-server` stays **provider-agnostic**: a registry of provider
adapters and per-account provider connections lets the same pipeline serve any
issue/task tracker (Mello today; Jira, Linear, GitHub Issues by design) without
changing the daemon or the job queue. This capability owns the adapter
abstraction (`internal/server/provider`) and connection storage
(`internal/server/connection`).
## Requirements
### Requirement: Provider adapter registry

The system SHALL expose a registry that maps a provider code (e.g. `mello`) to
an adapter implementing a common interface (signature verification, event
parsing, and write-back). Adapters MUST be registered at server startup and
looked up by code when handling inbound webhooks and outbound write-backs.

#### Scenario: Resolve a registered adapter

- **WHEN** a request targets provider code `mello`
- **THEN** the registry returns the Mello adapter and the request is processed with it

#### Scenario: Reject an unknown provider

- **WHEN** a request targets a provider code with no registered adapter
- **THEN** the system MUST reject the request rather than guess a provider

### Requirement: Provider connections

The system SHALL store one provider connection per `(account, provider)` pair,
holding the credentials needed to write back to that provider. Credentials MUST
be sealed at rest (see `auth-and-secrets`) and MUST be unsealed only server-side
when a write-back is performed.

#### Scenario: Connect a provider account

- **WHEN** an authenticated user connects a provider with a valid token
- **THEN** the system stores a sealed credential and enforces uniqueness on `(account, provider)`

#### Scenario: Reuse the connection for write-back

- **WHEN** a job finishes and a write-back is required
- **THEN** the server unseals the matching provider connection's credential to call the provider API

### Requirement: Provider-agnostic data model

The system SHALL keep persisted job and account data free of
provider-specific columns, identifying external entities by
`(provider_code, external_*_id)` so new providers require no schema change.

#### Scenario: Add a new provider without migration

- **WHEN** a new provider adapter is registered
- **THEN** existing tables (`jobs`, `provider_connections`, `account_identities`) accommodate it without a schema migration

### Requirement: Adapter exposes channel key method

The provider adapter interface SHALL be extended with a method `ChannelKey(event payload) -> (providerCode, resourceID)` that extracts the normalized channel tuple from a raw event, enabling the channel router to compute the channel key without provider-specific knowledge.

#### Scenario: Mello adapter returns channel key

- **WHEN** the Mello adapter's `ChannelKey` is called with a webhook payload containing `ticket_id = "TICKET-99"`
- **THEN** it returns `("mello", "TICKET-99")`

### Requirement: Provider connection resolved from channel session

The provider connection lookup SHALL be extended to support resolution from a channel session context (account ID + provider code), enabling the write-back flow to find the correct credentials from the channel binding without the caller needing to know the account ID explicitly.

#### Scenario: Write-back resolves connection from session

- **WHEN** a write-back is triggered for channel `"mello:TICKET-99"` with a bound session containing `account_id = "acct_1"`
- **THEN** the system looks up the provider connection using `(account_id="acct_1", provider_code="mello")` and unseals the credential

### Requirement: Server HTTP hardening

The server SHALL bound the size of inbound request bodies and SHALL bound the time allowed to
read request headers, to mitigate memory-exhaustion and slow-client (slowloris) denial of
service. These limits MUST NOT apply to the long-lived Server-Sent-Events response streams
(the runtime subscribe stream and the session event stream), which remain open for the
lifetime of the subscription.

#### Scenario: Oversized request body is rejected

- **WHEN** a client sends a request whose body exceeds the configured maximum
- **THEN** the server rejects it rather than buffering an unbounded body

#### Scenario: SSE streams are not severed by the limits

- **WHEN** a subscriber holds an SSE stream open beyond the header-read timeout
- **THEN** the stream continues to receive events (the body/header limits do not close it)

### Requirement: Liveness and readiness probes

The server SHALL expose distinct **liveness** and **readiness** probes. Liveness SHALL report
process health independently of the database, so a transient database outage does not flap
liveness. Readiness SHALL report whether the server can serve traffic (the database is
reachable). Probe responses that depend on the database MUST return a generic status body and
MUST NOT leak the underlying error to the caller (the error is logged server-side). A
backward-compatible health endpoint MAY be retained with readiness semantics.

#### Scenario: Liveness independent of the database

- **WHEN** the database is unreachable but the process is running
- **THEN** the liveness probe still returns success

#### Scenario: Readiness reflects database reachability without leaking errors

- **WHEN** the database is unreachable
- **THEN** the readiness probe returns a not-ready status with a generic body, and the
  underlying database error is logged rather than returned to the caller

### Requirement: Chat-type provider support

The provider adapter registry SHALL support two categories of providers: **webhook-type** (Mello, GitHub, Jira) where events arrive via incoming HTTP webhooks, and **chat-type** (Mezon) where events arrive via the bot's outbound WebSocket connection. Adapters for chat-type providers MAY return empty or no-op implementations for webhook-specific methods (`VerifyWebhook`, `WebhookHeaders`, `ExtractContainerID`).

#### Scenario: Register a chat-type provider

- **WHEN** the Mezon provider adapter is registered
- **THEN** it SHALL be usable via the channel router for message dispatch, even though `VerifyWebhook` and `WebhookHeaders` return no-op values

#### Scenario: Chat provider does not use webhook endpoint

- **WHEN** a request targets `POST /webhooks/mezon`
- **THEN** the system MAY reject it or return 404 (Mezon is not a webhook source)

### Requirement: Connection config supports chat credentials

The provider connection SHALL support storing chat-type credentials (appID, apiKey) in its `config` JSONB column alongside the existing token-based credentials. The `mcp_auth_enc` column SHALL hold the sealed apiKey. A new `config.mezon_app_id` field SHALL hold the app ID as plaintext (it is not a secret -- only the apiKey is sealed).

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

