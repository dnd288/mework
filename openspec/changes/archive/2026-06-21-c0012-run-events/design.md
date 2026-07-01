## Context

A dispatched run today is opaque after it leaves the hub: the runner pulls and
runs an agent but emits nothing upstream, so no client can watch progress, no
operator can see in-flight runs, and a stuck run can only be stopped by killing the
host. `c0002-message-bus` gives us topics/subscriptions to carry upstream events
and control messages; `c0005-agent-runner` gives us the enrolled runner that emits
events and owns the local sandbox. This change adds the live telemetry + control
layer on top of those two.

## Goals / Non-Goals

**Goals:**
- Upstream `RunEvent` emission from the runner/agent (progress|log|output|status).
- Per-run live subscription clients can tail.
- A bounded recent tail for late subscribers, then live, in order.
- Assemble streamed `output` into the result used for the write-back.
- Queryable run status and an operator-facing overview of active runs/sessions.
- Graceful→forced run cancellation that propagates teardown to the sandbox and is
  idempotent and terminal.

**Non-Goals:**
- The SSE/bus transport internals (`c0002-message-bus`).
- The runner loop and sandbox driver internals (`c0005-agent-runner`,
  `sandbox-runtime`) — this change *drives* them.
- Interactive multi-turn chat (separate conversation surface).
- Long-term artifact retention beyond the bounded tail (artifact store is separate).

## Decisions

- **`RunEvents` interface (`shared/transport`).** Four methods:
  `Emit(ctx, runID, RunEvent)` (runner→hub), `Subscribe(ctx, runID) →
  Subscription` (client tails one run), `Status(ctx, runID) → RunStatus`, and
  `Cancel(ctx, runID, force)`. The orchestration (`server/orchestrator`) backs
  `Emit`/`Subscribe` with bus topics keyed per run, keeps a recent-tail buffer, and
  drives status + cancel.
- **`RunEvent` kinds = `progress|log|output|status`.** `progress` is coarse
  completion, `log` is human-readable lines, `output` is result content chunks,
  `status` marks lifecycle transitions. The kind set is closed and enumerable.
- **Late subscriber = tail + live.** A new subscriber first receives a *bounded*
  recent tail (replay of the last N buffered events for the run) and is then
  spliced into the live stream with no gap and no duplication at the boundary.
- **Per-run ordering.** Events for a single run are delivered in emission order;
  ordering across different runs is not constrained. The bus event ID is monotonic
  per stream so a resuming subscriber preserves order via `Last-Event-ID`.
- **Output feeds the write-back.** `output` events are assembled (in order) into
  the run's `Result.Output`, which is what the server-side REST write-back posts —
  there is one source of truth for run output.
- **Cancel is graceful then forced.** `Cancel(..., force=false)` requests a
  graceful stop (signal the agent, allow teardown); if the run does not stop,
  `Cancel(..., force=true)` forces termination. Either way the run reaches a
  terminal canceled/failed state.
- **Cancel propagates to the sandbox.** Cancellation flows down to the runner over
  the bus control channel; the runner stops/destroys the sandbox (guaranteed
  teardown) and releases resources — the run does not leak a live sandbox.
- **Cancel of a pending/scheduled run.** A run that has been scheduled but not yet
  fired is cancelled at the schedule level so it never dispatches.
- **Cancel is idempotent and terminal.** Re-issuing cancel on an already-canceled
  run is a no-op success; a canceled run cannot resume or transition back to
  running.
- **Status + overview.** `Status` returns the run's current `RunStatus`
  (done/failed/…); presence/heartbeat detail for runners comes from the registry's
  live SSE channel; an operator overview (CLI/API) aggregates runner presence,
  active sessions, and in-flight run statuses for a tenant.

## Risks / Trade-offs

- **Tail buffer bound.** A larger recent tail helps late subscribers but costs
  memory per run; the bound is a trade-off and very chatty runs may lose the
  earliest lines from the replay (live stream is unaffected).
- **Boundary correctness.** Splicing tail→live without gaps or duplicates is the
  subtle part; we order by the monotonic per-run event ID and de-dup at the seam.
- **Cancel races.** A run completing while a cancel is in flight must resolve to a
  single terminal state; the state machine treats canceled/done/failed as mutually
  exclusive terminals and cancel-after-terminal as an idempotent no-op.
- **Forced teardown vs. data loss.** A forced cancel may interrupt the agent
  mid-write; the guarantee is sandbox teardown and resource release, not a clean
  flush — graceful is attempted first to give the agent a chance to finish.
- **Backpressure.** A slow subscriber must not stall emission for others; per-run
  subscriptions are independent and a slow consumer is dropped/lagged rather than
  blocking the run.
