## MODIFIED Requirements

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

#### Scenario: State is tracked independently of transport

- **WHEN** a work item's status changes
- **THEN** the change is recorded in the backing store regardless of how the originating message was delivered to the client

## REMOVED Requirements

### Requirement: Exactly-once claim

**Reason**: The client no longer pulls work by claiming queued rows; work is
**pushed** to subscribers over the SSE message bus (see the `message-bus`
capability). The one-active-per-runtime and `FOR UPDATE SKIP LOCKED` claim
mechanics are replaced by topic subscription plus delivery acknowledgement.

**Migration**: Routing a unit of work to a specific runtime is expressed as
publishing to that runtime's/agent's topic; "exactly-once" handling is provided by
the bus's delivery acknowledgement and lease/redelivery semantics rather than a
claim transaction.

### Requirement: Heartbeat and lease

**Reason**: Per-job claim leases and heartbeats are superseded by the message
bus's delivery lease/redelivery (see `message-bus` "Delivery acknowledgement") and
by runner presence over the SSE channel (defined in the `agent-runner` capability).

**Migration**: Liveness moves to SSE-channel presence; in-flight redelivery moves
to the bus's unacked-message lease.
