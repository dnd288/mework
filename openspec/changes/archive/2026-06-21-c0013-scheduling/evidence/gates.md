# Evidence for c0013-scheduling

## Build Status

Build passes: `go build ./...` compiles cleanly.

## Files Changed

- `shared/core/types.go` — Added ScheduleSpec, ScheduleKind, MissedPolicy, ScheduleState types
- `shared/ports/interfaces.go` — Added Scheduler interface
- `server/platform/store/migrations/000006_schedules.sql` — Schedules table migration
- `server/scheduler/scheduler.go` — Full scheduler implementation (CRUD + cron/interval/at engine + missed-fire policy + background fire loop)
- `server/scheduler/handlers.go` — HTTP handlers for scheduler REST API
- `server/hub/router.go` — Wired scheduler routes into the server

## Implementation Summary

### Types (shared/core)
- ScheduleKind: cron | interval | at
- MissedPolicy: skip | catch_up
- ScheduleState: active | paused | canceled
- ScheduleSpec: configures a schedule with kind-specific parameters

### Interface (shared/ports)
- Scheduler interface: Schedule, Pause, Resume, Cancel, List, Get

### Migration (server/platform/store)
- schedules table with columns: id, tenant_id, kind, cron, every, at, tz, agent, target, grant_data, missed, state, next_fire, last_fire, created_at, updated_at
- Indexes on tenant_id and active next_fire

### Implementation (server/scheduler)
- CRUD operations: Schedule (create), Pause, Resume, Cancel, List, Get
- Cron evaluation: parses standard 5-field cron expressions, computes next fire time with IANA timezone support
- Interval: uses Go time.ParseDuration for interval spec
- At: one-shot RFC3339 instant, completes after firing
- Background fire loop: polls every 10s for due schedules
- Missed-fire handling: skip drops missed fires, catch_up dispatches once on next poll
- Fire advancement: after each fire, recurring schedules re-arm; at schedules complete
- Tenant isolation: all queries filter by tenant_id

### Routes (server/hub)
- POST /api/v1/schedules — Create schedule
- GET /api/v1/schedules — List schedules
- GET /api/v1/schedules/{id} — Get schedule details
- POST /api/v1/schedules/{id}/pause — Pause schedule
- POST /api/v1/schedules/{id}/resume — Resume schedule
- POST /api/v1/schedules/{id}/cancel — Cancel schedule

