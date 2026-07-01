## ADDED Requirements

### Requirement: Install-once enrollment

A runner SHALL be installed and **enrolled once** against the hub (hub URL plus a
registration token), producing a **durable runner identity** persisted locally.
After enrollment the runner MUST operate **unattended** — no further manual
operation on the host is required to receive and run work.

#### Scenario: Enroll a new runner

- **WHEN** an operator runs the enrollment command with the hub URL and a valid registration token
- **THEN** the runner obtains and persists a durable runner identity and is ready to receive dispatches

#### Scenario: Unattended after enrollment

- **WHEN** an enrolled runner restarts
- **THEN** it resumes receiving and running dispatched work using its persisted identity, with no manual re-configuration

#### Scenario: Invalid registration token is rejected

- **WHEN** enrollment is attempted with an invalid or expired registration token
- **THEN** enrollment fails and no runner identity is persisted

### Requirement: SSE subscription and presence

An enrolled runner SHALL subscribe to its topics over SSE (per the `message-bus`
capability) and SHALL maintain **presence** so the hub knows it is online. The
runner MUST receive dispatches by push, not by polling.

#### Scenario: Runner comes online

- **WHEN** an enrolled runner starts and opens its SSE subscription
- **THEN** the hub marks the runner present/online, making it eligible to receive dispatches

#### Scenario: Receive a dispatch by push

- **WHEN** the hub dispatches an agent to the runner's topic
- **THEN** the runner receives it over the open SSE stream without issuing a poll request

### Requirement: Pull-run-report loop

On receiving a dispatch, the runner SHALL **pull** the referenced agent version
from the catalog, **run** it (via the sandbox runtime), and **report** the terminal
result back to the hub over POST. The loop MUST acknowledge the dispatch message so
it is not redelivered after successful terminal handling.

#### Scenario: Successful dispatch lifecycle

- **WHEN** the runner receives a dispatch for `code-fixer@1.2.0`
- **THEN** it pulls that version, runs it, posts the result to the hub, and acknowledges the dispatch

#### Scenario: Failed run is reported, not dropped

- **WHEN** a dispatched run fails during execution
- **THEN** the runner reports a `failed` result with a summary and acknowledges the dispatch

### Requirement: Grant enforcement on the client

The runner SHALL enforce the **permission grant** carried by a dispatch: operations
not covered by the grant MUST be refused locally, independent of any server-side
check. The runner MUST NOT widen its own scope beyond the grant.

#### Scenario: Operation within grant proceeds

- **WHEN** a dispatched run requests an operation covered by its grant
- **THEN** the runner permits the operation

#### Scenario: Operation outside grant is refused

- **WHEN** a dispatched run requests an operation not covered by its grant
- **THEN** the runner refuses the operation locally and reports it

### Requirement: Resilient runner lifecycle

The runner SHALL survive transient connection loss and process restarts without losing or
duplicating work: it MUST reconnect with jittered backoff and resume from its
`Last-Event-ID`, and on restart it MUST recover in-flight bookkeeping so an unacknowledged
dispatch is redelivered rather than dropped or double-run.

#### Scenario: Reconnect and resume after a dropped connection

- **WHEN** the runner's SSE connection drops after processing a dispatch and it reconnects with its `Last-Event-ID`
- **THEN** it resumes the stream and misses no dispatch, without redelivering already-processed events

#### Scenario: Crash recovery redelivers unacknowledged work

- **WHEN** the runner crashes mid-run with a dispatch still unacknowledged and then restarts with its persisted identity
- **THEN** the unacknowledged dispatch is redelivered and the runner resumes the pull → run → report loop without duplicating a completed run

### Requirement: One active run per runner under concurrency

The runner SHALL run at most one agent at a time: when multiple dispatches arrive
concurrently they MUST all be delivered, but the runner MUST serialize execution (one
active run) rather than run them in parallel on the same host.

#### Scenario: Concurrent dispatches are all delivered

- **WHEN** the hub dispatches several agents to one runner in quick succession
- **THEN** every dispatch is delivered to the runner (none lost under concurrency)

#### Scenario: A runner runs one agent at a time

- **WHEN** a second dispatch arrives while the runner is already executing one
- **THEN** the second dispatch is serialized (queued) and not run concurrently on the same runner
