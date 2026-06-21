## Context

An automation hub needs to run agents on a clock, not only in response to a
ticket comment or a manual dispatch. The dispatch path already exists
(`c0004-agent-catalog` publishes a dispatch — agent version + grant — to a target
runner's topic). What is missing is a durable, time-driven producer of those
dispatches. Because the platform is hybrid (runners come and go, may be offline at
any moment), the scheduler must also be explicit about what happens when a fire
time elapses with no runner online. This design introduces a `Scheduler` over a
small `ScheduleSpec`, with its module home in `server/scheduler`.

## Goals / Non-Goals

**Goals:**
- One interface, three kinds: cron (recurring on a cron expression), interval
  (recurring every `Every`), and at (one-shot at `At`).
- Timezone-aware cron evaluation against the schedule's IANA `TZ`.
- A full lifecycle: create, pause, resume, cancel, and per-tenant list.
- An explicit missed-fire policy for offline runners (`skip` vs `catch_up`).
- Firing a schedule **publishes a dispatch** through the existing catalog path —
  the scheduler never runs an agent itself.

**Non-Goals:**
- Running, sandboxing, or reporting on the agent — that is the runner/sandbox.
- Resolving agent versions or building grants — that is the catalog (`c0003`); the
  scheduler reuses it.
- The transport/topic substrate — that is the message bus.
- A general workflow/DAG engine; each schedule fires a single dispatch.

## Decisions

- **`ScheduleSpec` carries one kind plus its parameters.** `Kind` selects
  `cron` | `interval` | `at`. `Cron` (a cron expression) and `TZ` (IANA zone) apply
  to `cron`; `Every` (a duration) applies to `interval`; `At` (a fixed instant)
  applies to `at`. The spec also names the work: `Agent` (the `AgentRef` to
  dispatch), `Target` (the runner to dispatch to), and `Grant` (the scoped grant
  to carry), plus `Missed` (the missed-fire policy).
- **Firing publishes a dispatch.** At each fire time the scheduler resolves the
  schedule's `Agent` and publishes a dispatch (agent version + `Grant`) to
  `Target`'s topic via the catalog/orchestrator — identical to a manual dispatch.
  The scheduler is a dispatch producer, not a runner.
- **Recurring kinds re-arm; `at` completes.** `cron` and `interval` compute their
  next fire after each fire and re-arm; a one-shot `at` schedule fires exactly once
  and then completes (no re-arm).
- **Cron is timezone-aware.** A cron expression is evaluated against the schedule's
  `TZ`, so `0 9 * * *` fires at 09:00 local wall-clock time in that zone; two
  schedules with the same expression but different `TZ` fire at different absolute
  instants.
- **Lifecycle is explicit.** A schedule is `active`, `paused`, or `canceled`.
  `Pause` suppresses fires without discarding the schedule; `Resume` re-arms it;
  `Cancel` removes it permanently (terminal). A paused schedule never fires.
- **`MissedPolicy` governs offline fire times.** When a fire time elapses while no
  runner is online for `Target`, `skip` drops that missed fire; `catch_up` retains
  it and dispatches once on the next runner availability. Catch-up coalesces to a
  single run regardless of how many fire times were missed (no thundering herd).
- **Schedules are tenant-scoped.** Each schedule belongs to the tenant that created
  it. `List(tenant)` returns only that tenant's schedule ids; there is no
  cross-tenant visibility, and lifecycle operations are authorized within the
  owning tenant.

## Risks / Trade-offs

- **Clock skew and DST.** Timezone-aware cron must handle daylight-saving
  transitions (skipped/duplicated local hours); evaluate against the zone's rules
  rather than a fixed offset.
- **Catch-up storms.** A long outage could queue many missed fires; mitigated by
  coalescing `catch_up` to a single run on recovery.
- **Missed `at` while offline.** A one-shot `at` whose instant passed during an
  outage still follows `Missed`: `skip` drops it, `catch_up` runs it once on
  recovery; either way the schedule then completes.
- **Durability.** Schedules and their next-fire state must survive a server
  restart, or recurring/`catch_up` semantics break; persist them (out of scope to
  detail here, but assumed).
- **Misfire vs duplicate.** Re-arm and catch-up must not double-dispatch a single
  fire time; fire bookkeeping is per-schedule and idempotent on the fire instant.
