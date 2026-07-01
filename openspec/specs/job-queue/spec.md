# Job Queue Specification

## Purpose

Define the durable, Postgres-backed job lifecycle that connects the webhook
pipeline to the daemon: enqueue, long-poll claim, ack, heartbeat, a transactional
state machine, and a background sweeper that recovers leases. Owned by
`internal/server/jobs`.

## Requirements

### Requirement: Transactional state machine

The system SHALL enforce in-flight work-item status transitions inside a
transaction with row locking, with terminal states (`done`, `failed`) immutable
and same-status transitions idempotent. Under the message bus this state machine
governs the **durable backing store behind the bus** (the record of a dispatched
unit of work), **not** a client-facing claim queue. Entering `running` MUST set
`started_at`; entering `done`/`failed` MUST set `finished_at`.

#### Scenario: Reject a transition out of a terminal state

- **WHEN** a work item is `done` and a transition to `running` is attempted
- **THEN** the system returns an invalid-transition error and leaves the item unchanged

#### Scenario: Idempotent re-ack

- **WHEN** an ack sets a job to a status it already holds
- **THEN** the system treats it as a no-op and succeeds

#### Scenario: State is tracked independently of transport

- **WHEN** a work item's status changes
- **THEN** the change is recorded in the backing store regardless of how the originating message was delivered to the client

### Requirement: Lease sweeper

The system SHALL run a background sweeper that returns jobs whose lease has
expired (runtime crashed or stalled) back to `queued` so another runtime can
claim them, and that drives pending write-backs.

#### Scenario: Reclaim an abandoned job

- **WHEN** a claimed/running job's lease expires with no heartbeat
- **THEN** the sweeper transitions it back to `queued` for re-claim
