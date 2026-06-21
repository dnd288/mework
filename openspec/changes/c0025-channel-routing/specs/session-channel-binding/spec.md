# Session-Channel Binding Specification

## Purpose

Define the durable binding between channel keys and sessions. Each active channel has exactly one bound session; each session may have one or more bound channels (e.g., a sandbox handling multiple resources). The binding is persisted in Postgres for crash recovery and cached in memory for fast event routing. Owned by `libs/server/channel/` and `libs/server/session/`.

## ADDED Requirements

### Requirement: Durable channel-to-session binding

The system SHALL persist each channel→session binding in a `channel_sessions` table. The binding SHALL record the channel key, session ID, provider code, resource ID, runner ID, status, and timestamps. The binding SHALL survive a server restart.

#### Scenario: Binding persists across restart

- **WHEN** a channel `"mello:TICKET-99"` is bound to session `"s_abc123"` and the server restarts
- **THEN** the binding is loaded from the DB into the in-memory cache on startup
- **AND** when the next event for that channel arrives, it is routed to the existing session

#### Scenario: Binding recovered for disconnected session

- **WHEN** the session `"s_abc123"` is still active but its in-memory cache entry was evicted
- **THEN** the next lookup reads from the DB and repopulates the cache

### Requirement: Channel lifecycle management

A channel binding SHALL have a lifecycle with three states: `active`, `draining`, and `closed`. An `active` channel accepts and delivers events. A `draining` channel accepts no new events but completes in-flight processing. A `closed` channel is unbound and its resources (bus subscription, session slot) are released.

#### Scenario: Channel transitions through lifecycle

- **WHEN** a sandbox signals completion for channel `"mello:TICKET-99"`
- **THEN** the channel transitions `active → draining → closed`
- **AND** the session is closed
- **AND** the runner's active binding count is decremented

#### Scenario: New event rejected on draining channel

- **WHEN** an event arrives for channel `"mello:TICKET-99"` while it is `draining`
- **THEN** the event is not delivered and the system returns a 200 to the provider (silent ignore)

### Requirement: Sweeper for orphaned bindings

The system SHALL run a background sweeper that closes channel bindings where the bound session or runner is no longer active. The sweeper SHALL run every 30 seconds and SHALL log each orphaned binding it closes.

#### Scenario: Sweeper closes orphaned channel

- **WHEN** the runner bound to channel `"mello:TICKET-99"` goes `offline` unexpectedly
- **THEN** the sweeper transitions the channel to `closed` and frees the binding
- **AND** any buffered events for that channel are returned to the provider (silent ignore)

### Requirement: Per-runner binding count

The system SHALL track the number of active channel bindings per runner, in memory and in the DB. This count SHALL be used by the spec-aware runner selector for load-balanced assignment. The count SHALL be decremented when a channel transitions to `closed`.

#### Scenario: Binding count tracks active channels

- **WHEN** a runner has 2 active channel bindings and a 3rd channel is assigned
- **THEN** the binding count becomes 3
- **WHEN** one channel is closed
- **THEN** the binding count becomes 2
