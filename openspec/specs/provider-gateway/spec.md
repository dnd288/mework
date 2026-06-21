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
