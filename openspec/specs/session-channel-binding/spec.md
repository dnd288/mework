# Session-Channel Binding Specification

## Purpose

Define the durable binding between channel keys and sessions. Each active channel has exactly one bound session. The binding is persisted in Postgres for crash recovery and cached in memory for fast event routing. Owned by `libs/server/channel/` and `libs/server/session/`.

## Requirements

### Requirement: Durable channel-to-session binding

The system SHALL persist each channel→session binding in a `channel_sessions` table recording the channel key, session ID, provider code, resource ID, runner ID, status, and timestamps. The binding SHALL survive a server restart.

#### Scenario: Binding persists across restart

- **WHEN** a channel `"mello:TICKET-99"` is bound to session `"s_abc123"` and the server restarts
- **THEN** the binding is loaded from the DB into the in-memory cache on startup
- **AND** when the next event arrives, it is routed to the existing session

### Requirement: Channel lifecycle management

A channel binding SHALL have three states: `active`, `draining`, and `closed`. An `active` channel accepts events. A `draining` channel accepts no new events but completes in-flight processing. A `closed` channel is unbound.

#### Scenario: Channel transitions through lifecycle

- **WHEN** a sandbox signals completion for channel `"mello:TICKET-99"`
- **THEN** the channel transitions `active → draining → closed`
- **AND** the session is closed and the runner's active binding count is decremented

### Requirement: Sweeper for orphaned bindings

The system SHALL run a background sweeper that closes channel bindings where the bound session or runner is no longer active. The sweeper SHALL run every 30 seconds.

#### Scenario: Sweeper closes orphaned channel

- **WHEN** the runner bound to channel `"mello:TICKET-99"` goes `offline` unexpectedly
- **THEN** the sweeper transitions the channel to `closed` and frees the binding
