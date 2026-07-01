## 1. Transport contract

- [ ] 1.1 Define `RunEvent` (kind: progress|log|output|status; data) in `shared/transport`
- [ ] 1.2 Define the `RunEvents` interface (Emit/Subscribe/Status/Cancel) in `shared/transport`

## 2. Upstream emission

- [ ] 2.1 `Emit(runID, RunEvent)`: accept runner/agent upstream events and publish on the run's bus topic
- [ ] 2.2 Assemble in-order `output` events into the run's result output for the server-side write-back

## 3. Subscription & tail

- [ ] 3.1 `Subscribe(runID)`: return a live subscription to a run's events in `server/orchestrator`
- [ ] 3.2 Maintain a bounded recent-tail buffer per run; replay tail then splice into live (no gaps/dupes)
- [ ] 3.3 Preserve per-run emission ordering over the stream

## 4. Status & overview

- [ ] 4.1 `Status(runID)`: return the run's current status, queryable at any time
- [ ] 4.2 Surface runner presence/heartbeat detail from the live channel
- [ ] 4.3 Operator overview (CLI/API): runner presence, active sessions, in-flight run statuses

## 5. Cancellation

- [ ] 5.1 `Cancel(runID, force)`: graceful stop, then forced termination if it does not stop
- [ ] 5.2 Propagate cancellation to the runner; stop/destroy the sandbox and release resources
- [ ] 5.3 Cancel a scheduled/pending run before it fires (never dispatches)
- [ ] 5.4 Make cancel idempotent and terminal (repeat is a no-op success; canceled cannot resume)

## 6. Validate

- [ ] 6.1 openspec validate c0011-run-events --type change --strict
- [ ] 6.2 e2e pointer: flip `tests/e2e/18_status_streaming_test.go` from Skip to Green for STREAM-01..05 (agent→hub progress/log/output streaming, client tails a run, late-subscriber tail, per-run ordering, output→write-back) and STATUS-01..03 (run status transitions, presence detail, status overview); flip `tests/e2e/19_cancellation_test.go` from Skip to Green for CANCEL-01..04 (cancel running run, propagate to sandbox, cancel scheduled, idempotent/terminal). Cross-references: `tests/e2e/17_chat_test.go` CHAT-* streams through this bus; `tests/e2e/13_journeys_test.go` E2E-02/03 target resilience must remain green.
