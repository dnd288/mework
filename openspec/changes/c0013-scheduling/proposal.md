## Why

Today an agent only runs when something dispatches it: a user comments on a
ticket, or an operator hits the dispatch endpoint by hand. There is no way to say
"run the dependency-bump agent every weekday at 09:00", "sweep stale branches
once an hour", or "kick off the release agent at this exact instant next Friday".
Recurring and time-triggered work — the bread and butter of any automation
platform — has no home. And the model is hybrid: runners come and go, so a
scheduler must also answer what happens when a fire time elapses while no runner
is online. This change adds first-class **scheduling**: cron, interval, and
one-shot run-at dispatches, with pause/resume/cancel, timezone-aware evaluation,
a missed-fire policy, and per-tenant listing.

## What Changes

- A new **scheduling** capability, with its module home in `server/scheduler`,
  that fires dispatches on a schedule **via the catalog/orchestrator** (it does
  not run agents itself — at fire time it publishes a dispatch for the configured
  agent to the target runner, exactly as a manual dispatch would).
- A `Scheduler` interface (`Schedule`/`Pause`/`Resume`/`Cancel`/`List`) defined in
  `shared`, alongside the `ScheduleSpec`, `ScheduleKind` (`cron`|`interval`|`at`),
  and `MissedPolicy` (`skip`|`catch_up`) types.
- Three schedule kinds: **cron** (recurring on a cron expression, evaluated in the
  schedule's IANA timezone), **interval** (recurring every `Every`), and **at**
  (a one-shot dispatch at a fixed future instant that completes after it fires).
- **Lifecycle controls**: a schedule can be paused (suppresses fires without
  losing it), resumed (re-arms it), and canceled (removed permanently).
- **Missed-fire policy**: when a fire time elapses while no runner is online,
  `skip` drops the missed fire and `catch_up` runs it once on next availability.
- **Timezone-aware cron**: cron expressions are evaluated against the schedule's
  `TZ`, so the same expression fires at the local wall-clock time of its zone.
- **Per-tenant listing and isolation**: `List` enumerates only the calling
  tenant's schedules; there is no cross-tenant visibility.
- Depends on `c0004-agent-catalog`: firing a schedule **dispatches** the
  configured agent version to the target runner with its grant, reusing the
  catalog's dispatch path.

## Capabilities

### New Capabilities

- `scheduling`: dispatch agents on schedules — cron, interval, and one-shot
  run-at; recurring schedules that re-arm each interval; pause/resume/cancel;
  a missed-fire policy (`skip` vs `catch_up`) for when the runner is offline;
  timezone-aware cron evaluation; and per-tenant schedule listing with isolation.

## Impact

- **Depends on `c0004-agent-catalog`**: a schedule fires by dispatching the
  configured agent version (with its grant) to the target runner via the catalog's
  dispatch path; the scheduler is a producer of dispatches, not a new runner.
- Module home is `server/scheduler`; the `Scheduler`, `ScheduleSpec`,
  `ScheduleKind`, and `MissedPolicy` contract lives in `shared`.
- Behaviors are pinned by the e2e scenarios `SCHED-01` through `SCHED-07`
  (`tests/e2e/15_scheduling_test.go`), driving the `Scheduler`/`ScheduleSpec`
  surface in `tests/e2e/api_test.go`: cron fires at the scheduled time (SCHED-01),
  a recurring schedule re-arms each interval (SCHED-02), a one-shot run-at fires
  once then completes (SCHED-03), pause/resume/cancel (SCHED-04), missed-fire
  policy `skip` vs `catch_up` while offline (SCHED-05), timezone-aware cron
  (SCHED-06), and per-tenant schedule listing/isolation (SCHED-07).
