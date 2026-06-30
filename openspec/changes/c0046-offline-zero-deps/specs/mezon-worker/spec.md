# Mezon Worker — Delta

## Changes

The worker previously **required** an external Redis server for the turbo
engine's state store (message dedup, channel cursors, activity tracking).
It now falls back to an **embedded in-memory Redis** (`miniredis`) when
no `REDIS_URL` is configured. This allows the worker to run on machines
without Redis installed, at the cost of losing state on restart.

## Modified Requirements

### Requirement: Worker is configured via environment (modified)

The worker SHALL read its configuration from environment variables. The
`REDIS_URL` variable becomes **optional** (was: required). When unset,
the worker SHALL start an embedded in-memory Redis server and connect
to it for the turbo engine's state store.

#### Scenario: Worker starts without Redis (new)

- **WHEN** the worker starts without `REDIS_URL` set
- **THEN** it starts an embedded miniredis server
- **THEN** it logs a warning: `WARNING: using embedded in-memory state, lost on restart`
- **THEN** the turbo engine operates normally with in-memory state

#### Scenario: Worker starts with Redis (unchanged)

- **WHEN** the worker starts with `REDIS_URL` set to a valid Redis URL
- **THEN** it connects to the external Redis server
- **THEN** state is persistent across restarts

#### Scenario: Worker restarts without Redis (new)

- **WHEN** the worker running with miniredis is restarted
- **THEN** all state (dedup cursors, channel tracking, activity) is lost
- **THEN** the worker re-learns channels from inbound messages
- **THEN** duplicate message delivery may occur for messages seen before
  the restart
