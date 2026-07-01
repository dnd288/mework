# Session-Channel Binding Specification

## Purpose

Define the durable binding between channel keys and sessions. Each active channel has exactly one bound session. The binding is persisted in Postgres for crash recovery and cached in memory for fast event routing. Owned by `libs/server/channel/` and `libs/server/session/`.

## ADDED Requirements

### Requirement: Channel sessions for Mezon channel IDs

The channel→session binding SHALL support Mezon channel IDs as valid resource IDs alongside existing Mello ticket IDs. A Mezon channel key has the format `"mezon:<channel_id>"`. The binding SHALL be stored in the same `channel_sessions` table with `provider_code = "mezon"`.

#### Scenario: Bind Mezon channel

- **WHEN** a session is auto-provisioned for Mezon channel `"ch_abc"`
- **THEN** a binding is created with `channel_key = "mezon:ch_abc"`, `provider_code = "mezon"`, and `resource_id = "ch_abc"`

#### Scenario: Lookup bound Mezon channel

- **WHEN** a Mezon message arrives for channel `"ch_abc"` and a binding exists
- **THEN** the registry returns the bound session ID

### Requirement: Channel lifecycle for chat channels

A Mezon channel session SHALL follow the same lifecycle as other channel sessions: `active`, `draining`, and `closed`. A Mezon channel transitions to `closed` when the bot disconnects or the session is explicitly closed.

#### Scenario: Mezon channel closes on bot disconnect

- **WHEN** the Mezon bot disconnects (graceful shutdown or unexpected)
- **THEN** all active Mezon channel sessions are transitioned to `closed`
- **THEN** the sweeper cleans up any orphaned bindings

### Requirement: No DB required for offline Mezon binding

In offline mode, Mezon channel→session binding SHALL NOT require Postgres. Bindings SHALL be maintained in-memory only, keyed by channel ID. The offline agent's Mezon bot SHALL dispatch messages directly to the local sandbox without a channel registry lookup.

#### Scenario: In-memory binding in offline mode

- **WHEN** the offline agent processes a Mezon message
- **THEN** the channel is bound to the single local sandbox session in memory
- **THEN** no database lookup is performed
