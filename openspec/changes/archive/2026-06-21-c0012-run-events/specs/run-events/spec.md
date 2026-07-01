## ADDED Requirements

### Requirement: Upstream run event emission

The runner/agent SHALL emit upstream run events of kind `progress`, `log`, or
`output` for an in-progress run, and the hub MUST accept them for that run.

#### Scenario: Agent emits progress, log, and output upstream

- **WHEN** a running agent emits `progress`, `log`, and `output` events for a run
- **THEN** the hub accepts the upstream events for that run without error

### Requirement: Live run subscription

A client SHALL be able to subscribe to a run and receive that run's events live as
they are emitted.

#### Scenario: Client subscribes to a run's live events

- **WHEN** a client subscribes to a run and the runner then emits a log line
- **THEN** the subscribed client receives that event live on its subscription

### Requirement: Tail-then-live for late subscribers

A subscriber that joins after events have already been emitted SHALL first receive a
bounded recent tail of the run's events and then the live stream, with no gap or
duplication at the boundary.

#### Scenario: A late subscriber gets recent tail then live

- **WHEN** a client subscribes to a run that has already emitted several events
- **THEN** it receives a bounded recent tail of those events followed by live events in order

### Requirement: Per-run event ordering

Events for a single run SHALL be delivered to subscribers in emission order.

#### Scenario: Per-run order is preserved

- **WHEN** progress events 1, 2, 3 are emitted in order for a run
- **THEN** a client tailing that run receives them in emission order, 1 then 2 then 3

### Requirement: Streamed output feeds the write-back

Streamed `output` events for a run SHALL be assembled, in order, into the run
result that the server-side write-back posts.

#### Scenario: Streamed output is captured for the result

- **WHEN** a run streams output chunks and then reaches a terminal status
- **THEN** the assembled output is available as the run result for the server-side write-back

### Requirement: Queryable run status

The current status of a run SHALL be queryable at any time and reflect the run's
lifecycle transitions to a terminal state.

#### Scenario: Run status transitions are observable

- **WHEN** a dispatched run progresses to a terminal state and its status is queried
- **THEN** the query returns the run's status (e.g. done or failed)

### Requirement: Runner presence and heartbeat detail

The system SHALL report a runner's presence and recent heartbeat detail based on
its live channel.

#### Scenario: Presence reflects the live channel

- **WHEN** the hub is queried for the presence of a runner holding its live channel
- **THEN** it reports the runner online with recent heartbeat detail

### Requirement: Platform status overview

The system SHALL provide an operator-facing overview of a tenant's active runners,
sessions, and in-flight runs.

#### Scenario: Operators see active runs and sessions at a glance

- **WHEN** an operator queries platform status for a tenant via CLI or API
- **THEN** the response reports runner presence, active sessions, and in-flight run statuses

### Requirement: Graceful then forced run cancellation

A running run SHALL be cancellable with a graceful stop first and a forced
termination if it does not stop, after which the run reaches a terminal
canceled/failed state.

#### Scenario: Cancel a running run graceful then forced

- **WHEN** an operator cancels a running run gracefully and then forces it because it did not stop
- **THEN** the run stops and is reported as failed or canceled

### Requirement: Cancellation propagates to the sandbox

Cancelling a run SHALL propagate to the runner and stop or destroy the sandbox
executing the run, releasing its resources.

#### Scenario: Cancellation tears down the sandbox

- **WHEN** a run executing inside a sandbox is canceled
- **THEN** the sandbox is stopped or destroyed and its resources are released

### Requirement: Cancel a pending run before it fires

A run that has been scheduled but not yet fired SHALL be cancellable before its
fire time so that it never dispatches.

#### Scenario: Canceling a pending schedule prevents the dispatch

- **WHEN** a one-shot scheduled run is canceled before its fire time
- **THEN** the run never dispatches

### Requirement: Cancel is idempotent and terminal

Cancelling a run that is already canceled SHALL be a no-op success, and a canceled
run MUST NOT resume.

#### Scenario: Repeated cancel is an idempotent no-op

- **WHEN** cancel is issued again on a run that has already been canceled
- **THEN** it succeeds as a no-op and the run cannot resume
