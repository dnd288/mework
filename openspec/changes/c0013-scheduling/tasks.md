## 1. Schedule model & persistence

- [ ] 1.1 Define `ScheduleSpec` (`Kind`, `Cron`, `Every`, `At`, `TZ`, `Agent`, `Target`, `Grant`, `Missed`), `ScheduleKind` (`cron`|`interval`|`at`), and `MissedPolicy` (`skip`|`catch_up`) in `shared`
- [ ] 1.2 Add a migration for `schedules` (id, tenant, spec fields, `state` active|paused|canceled, next-fire bookkeeping)
- [ ] 1.3 Persist schedules so next-fire state survives a server restart

## 2. Scheduler interface

- [ ] 2.1 Implement `Scheduler.Schedule` in `server/scheduler` — validate the spec per kind, persist it active, and arm its first fire
- [ ] 2.2 Implement `Pause` / `Resume` (suppress vs re-arm) and `Cancel` (terminal removal)
- [ ] 2.3 Implement `List(tenant)` — return only the calling tenant's schedule ids

## 3. Fire engine

- [ ] 3.1 Evaluate cron expressions against the schedule's IANA `TZ` (timezone-aware, DST-correct)
- [ ] 3.2 Compute the next fire for `cron`/`interval` and re-arm after each fire; complete `at` after its single fire
- [ ] 3.3 At each fire, dispatch the configured agent version + grant to `Target` via the catalog/orchestrator (`c0004-agent-catalog`)
- [ ] 3.4 Make fire bookkeeping idempotent per fire instant (no double-dispatch on re-arm/recovery)

## 4. Missed-fire policy

- [ ] 4.1 Detect fire times that elapse while no runner is online for `Target`
- [ ] 4.2 `skip` drops the missed fire; `catch_up` dispatches once on next availability, coalescing multiple missed fires into one run

## 5. Tenant isolation & authorization

- [ ] 5.1 Scope every schedule to its creating tenant; reject cross-tenant access on list and lifecycle operations
- [ ] 5.2 Authorize `Schedule`/`Pause`/`Resume`/`Cancel`/`List` against the caller's tenant identity

## 6. Validate

- [ ] 6.1 Tests: cron fire/TZ, interval re-arm, one-shot at, pause/resume/cancel, missed-fire skip vs catch_up, per-tenant listing
- [ ] 6.2 `openspec validate c0012-scheduling --type change --strict`
