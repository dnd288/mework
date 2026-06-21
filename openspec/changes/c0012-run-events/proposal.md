## Why

Once a run is dispatched it currently disappears into the runner: there is no
upstream telemetry, no way for a client to tail a run as it happens, no queryable
run status, and no way to stop a run that is misbehaving or no longer wanted. The
GitHub-Actions-style DX the product targets expects exactly this — live logs while
a job runs, a status you can poll, and a cancel button that actually reaches the
machine doing the work.

This change defines **live run telemetry + control**: the runner/agent emits
upstream events (progress/log/output/status), clients subscribe to a run's live
stream, run status is queryable at any time, and a run can be cancelled
(graceful then forced) with the cancellation propagating down to the sandbox.

## What Changes

- A new **run-events** capability built on the bus: the runner/agent **emits**
  upstream `RunEvent`s (progress|log|output|status) for a run; clients
  **subscribe** to a run's live event stream; run **status** is queryable; a run
  can be **cancelled** (graceful→forced), propagating teardown to the sandbox.
- A late subscriber receives a bounded recent tail and then the live stream, with
  per-run event ordering preserved.
- Streamed `output` events are assembled into the result that feeds the
  server-side write-back.
- Module homes: the `RunEvents` contract and `RunEvent` DTO land in
  `shared/transport`; the emit/subscribe/status/cancel orchestration and tail
  buffer land in `server/orchestrator`. Upstream emission and cancel teardown ride
  the bus from `c0002-message-bus`; cancel reaches into the sandbox owned by the
  runner from `c0005-agent-runner`.

## Capabilities

### New Capabilities
- `run-events`: live run telemetry (progress/log/output/status) over the bus,
  per-run subscription with tail-then-live for late subscribers, queryable run
  status and platform overview, and graceful→forced run cancellation that
  propagates to the sandbox.

## Impact

- **Depends on `c0002-message-bus`** (topics/subscriptions carry upstream events
  and cancel control) and **`c0005-agent-runner`** (the runner emits events and
  owns the sandbox that cancellation tears down).
- New code: `shared/transport` (`RunEvents`, `RunEvent`); `server/orchestrator`
  (emit/subscribe/status/cancel, recent-tail buffer, cancel propagation).
- Behaviors covered: STREAM-01..05 (upstream emission, live subscribe,
  tail-then-live late subscriber, per-run ordering, output→write-back),
  STATUS-01..03 (status transitions, runner presence/heartbeat detail, platform
  status overview), CANCEL-01..04 (graceful→forced cancel, sandbox teardown,
  pre-fire schedule cancel, idempotent/terminal cancel).
